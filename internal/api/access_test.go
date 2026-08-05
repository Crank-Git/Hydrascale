package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hydrascale/internal/access"
	"hydrascale/internal/config"
	"hydrascale/internal/namespaces"
	"hydrascale/internal/reconciler"
)

// The test file declares the response shape rather than reading it from the package, so
// that a change to a Go type never hides a change to the wire format that the console
// reads. docs/specs/features/05-reachability-model.md quotes the shape.

type wireRule struct {
	From  string   `json:"from"`
	To    string   `json:"to"`
	Ports []string `json:"ports"`
}

type wireNode struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Peers int    `json:"peers"`
	Veth  string `json:"veth"`
}

type wireAccess struct {
	Mode  string     `json:"mode"`
	Rules []wireRule `json:"rules"`
	Nodes []wireNode `json:"nodes"`
}

// writeAccessConfig writes a configuration file that declares the named tailnets and the
// rule set, and it returns the path of that file. A nil rule set writes no access block.
func writeAccessConfig(t *testing.T, set *access.RuleSet, tailnets ...string) string {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := config.DefaultConfig()
	for _, id := range tailnets {
		cfg.Tailnets = append(cfg.Tailnets, config.Tailnet{ID: id})
	}
	cfg.Access = set
	if err := config.SaveConfig(cfgPath, cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	return cfgPath
}

// callAccess sends one request to the control socket and returns the status code and the
// whole body. body is the request body, and an empty string sends no body.
func callAccess(t *testing.T, c *Client, method, target, body string) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = bytes.NewReader([]byte(body))
	}
	req, err := http.NewRequest(method, "http://localhost"+target, reader)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, target, err)
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read the body of %s %s: %v", method, target, err)
	}
	return resp.StatusCode, payload
}

// loadAccessBlock returns the access block that the configuration file holds.
func loadAccessBlock(t *testing.T, cfgPath string) *access.RuleSet {
	t.Helper()
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	return cfg.Access
}

func TestGetAccessReturnsTheModeTheRuleListAndTheNodeList(t *testing.T) {
	set := &access.RuleSet{
		Mode: access.ModeObserve,
		Rules: []access.Rule{
			{From: "alpha", To: "beta", Ports: []string{"tcp/22"}},
			{From: "alpha", To: access.Internet},
		},
	}
	cfgPath := writeAccessConfig(t, set, "alpha", "beta")
	_, client, cleanup := startTestServer(t, newTestReconciler(cfgPath))
	defer cleanup()

	code, payload := callAccess(t, client, http.MethodGet, "/api/access", "")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusOK, payload)
	}

	var got wireAccess
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("decode the body %s: %v", payload, err)
	}
	if got.Mode != access.ModeObserve {
		t.Errorf("mode = %q, want %q", got.Mode, access.ModeObserve)
	}
	if len(got.Rules) != 2 {
		t.Fatalf("len(rules) = %d, want 2", len(got.Rules))
	}
	if got.Rules[0].From != "alpha" || got.Rules[0].To != "beta" {
		t.Errorf("rules[0] = %+v, want alpha -> beta", got.Rules[0])
	}
	if len(got.Rules[0].Ports) != 1 || got.Rules[0].Ports[0] != "tcp/22" {
		t.Errorf("rules[0].ports = %v, want [tcp/22]", got.Rules[0].Ports)
	}
	// FR-access-9 states that an empty port list allows every port, therefore the field
	// is an empty list rather than a null.
	if got.Rules[1].Ports == nil {
		t.Error("rules[1].ports = null, want an empty list")
	}

	kinds := map[string]string{}
	nodes := map[string]wireNode{}
	for _, n := range got.Nodes {
		kinds[n.ID] = n.Kind
		nodes[n.ID] = n
	}
	for id, want := range map[string]string{
		"alpha":         "tailnet",
		"beta":          "tailnet",
		access.Host:     "host",
		access.Internet: "internet",
	} {
		if kinds[id] != want {
			t.Errorf("node %q has kind %q, want %q", id, kinds[id], want)
		}
	}
	if len(got.Nodes) != 4 {
		t.Errorf("len(nodes) = %d, want 4", len(got.Nodes))
	}

	// The veth address of a node is the namespace side of the pair, because that is the
	// address the operator reaches from inside the namespace.
	_, _, hostGW, nsGW, err := namespaces.VethIPs("10.200.0.0/16", namespaces.VethIndex(namespaces.GetNamespaceName("alpha")))
	if err != nil {
		t.Fatalf("VethIPs: %v", err)
	}
	if nodes["alpha"].Veth != nsGW {
		t.Errorf("node alpha veth = %q, want %q", nodes["alpha"].Veth, nsGW)
	}
	if nodes["alpha"].Veth == hostGW {
		t.Errorf("node alpha veth = %q, which is the host side of the pair", nodes["alpha"].Veth)
	}
	if nodes[access.Host].Veth != "" {
		t.Errorf("node host veth = %q, want an empty value", nodes[access.Host].Veth)
	}
}

func TestPutAccessWithAnInvalidRuleReturnsHTTPBadRequestAndChangesNothing(t *testing.T) {
	set := &access.RuleSet{Rules: []access.Rule{{From: "alpha", To: access.Internet}}}
	cfgPath := writeAccessConfig(t, set, "alpha", "beta")
	_, client, cleanup := startTestServer(t, newTestReconciler(cfgPath))
	defer cleanup()

	body := `{"mode":"enforce","rules":[{"from":"alpha","to":"beta","ports":["tcp/70000"]}]}`
	code, payload := callAccess(t, client, http.MethodPut, "/api/access", body)
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusBadRequest, payload)
	}

	var refusal map[string]any
	if err := json.Unmarshal(payload, &refusal); err != nil {
		t.Fatalf("decode the body %s: %v", payload, err)
	}
	message, ok := refusal["error"].(string)
	if !ok {
		t.Fatalf("body %s holds no error field of type string", payload)
	}
	if !strings.Contains(message, "tcp/70000") {
		t.Errorf("error = %q, want a message that names the invalid port", message)
	}
	if len(refusal) != 1 {
		t.Errorf("body %s holds %d fields, want the single field error", payload, len(refusal))
	}

	live := loadAccessBlock(t, cfgPath)
	if live == nil || len(live.Rules) != 1 || live.Rules[0].To != access.Internet {
		t.Errorf("the configuration file holds %+v, want the rule set that the test wrote", live)
	}
}

func TestPutAccessValidatesEveryRuleBeforeItWritesTheConfigurationFile(t *testing.T) {
	set := &access.RuleSet{Rules: []access.Rule{{From: "alpha", To: access.Internet}}}
	cfgPath := writeAccessConfig(t, set, "alpha", "beta")
	_, client, cleanup := startTestServer(t, newTestReconciler(cfgPath))
	defer cleanup()

	before, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// The first rule is valid, the second names an undeclared tailnet, and the third
	// holds an invalid port. A route that writes before it validates keeps the first
	// rule, and a route that stops at the first failure names one failure only.
	body := `{"mode":"enforce","rules":[` +
		`{"from":"alpha","to":"beta"},` +
		`{"from":"alpha","to":"gamma"},` +
		`{"from":"beta","to":"alpha","ports":["tcp/22-21"]}]}`
	code, payload := callAccess(t, client, http.MethodPut, "/api/access", body)
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusBadRequest, payload)
	}

	var refusal map[string]any
	if err := json.Unmarshal(payload, &refusal); err != nil {
		t.Fatalf("decode the body %s: %v", payload, err)
	}
	message, _ := refusal["error"].(string)
	if !strings.Contains(message, "gamma") {
		t.Errorf("error = %q, want a message that names the undeclared tailnet gamma", message)
	}
	if !strings.Contains(message, "tcp/22-21") {
		t.Errorf("error = %q, want a message that names the invalid port range", message)
	}

	after, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("the route changed the configuration file:\nbefore %s\nafter  %s", before, after)
	}
}

func TestPutAccessWithDryRunReturnsTheCompiledResultAndChangesNothing(t *testing.T) {
	set := &access.RuleSet{Rules: []access.Rule{{From: "alpha", To: access.Internet}}}
	cfgPath := writeAccessConfig(t, set, "alpha", "beta")
	_, client, cleanup := startTestServer(t, newTestReconciler(cfgPath))
	defer cleanup()

	before, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	body := `{"mode":"observe","rules":[{"from":"beta","to":"alpha","ports":["tcp/22"]}]}`
	code, payload := callAccess(t, client, http.MethodPut, "/api/access?dry_run=true", body)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusOK, payload)
	}

	var got wireAccess
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("decode the body %s: %v", payload, err)
	}
	if got.Mode != access.ModeObserve {
		t.Errorf("mode = %q, want %q", got.Mode, access.ModeObserve)
	}
	if len(got.Rules) != 1 || got.Rules[0].From != "beta" {
		t.Errorf("rules = %+v, want the rule set that the request holds", got.Rules)
	}
	if len(got.Nodes) != 4 {
		t.Errorf("len(nodes) = %d, want 4", len(got.Nodes))
	}

	after, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("the dry run changed the configuration file:\nbefore %s\nafter  %s", before, after)
	}
}

func TestPutAccessWritesTheRuleSetAndTheReconcilerAppliesItOnTheNextTick(t *testing.T) {
	cfgPath := writeAccessConfig(t, &access.RuleSet{}, "alpha", "beta")
	r := newTestReconciler(cfgPath)
	_, client, cleanup := startTestServer(t, r)
	defer cleanup()

	body := `{"mode":"enforce","rules":[` +
		`{"from":"alpha","to":"beta","ports":["tcp/22","tcp/443"]},` +
		`{"from":"beta","to":"internet"}]}`
	code, payload := callAccess(t, client, http.MethodPut, "/api/access", body)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusOK, payload)
	}

	live := loadAccessBlock(t, cfgPath)
	if live == nil {
		t.Fatal("the configuration file holds no access block")
	}
	if live.EffectiveMode() != access.ModeEnforce {
		t.Errorf("mode = %q, want %q", live.EffectiveMode(), access.ModeEnforce)
	}
	if len(live.Rules) != 2 {
		t.Fatalf("len(rules) = %d, want 2", len(live.Rules))
	}
	if live.Rules[0].From != "alpha" || live.Rules[0].To != "beta" {
		t.Errorf("rules[0] = %+v, want alpha -> beta", live.Rules[0])
	}
	if len(live.Rules[0].Ports) != 2 {
		t.Errorf("rules[0].ports = %v, want two entries", live.Rules[0].Ports)
	}
	if live.Rules[1].To != access.Internet {
		t.Errorf("rules[1] = %+v, want beta -> internet", live.Rules[1])
	}

	// The reconciler reads the rule set that the route wrote, therefore a further tick
	// runs against the new rule set and it reports no failure.
	if err := r.Reconcile(); err != nil {
		t.Errorf("Reconcile after the write: %v", err)
	}

	code, payload = callAccess(t, client, http.MethodGet, "/api/access", "")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusOK, payload)
	}
	var got wireAccess
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("decode the body %s: %v", payload, err)
	}
	if len(got.Rules) != 2 {
		t.Errorf("GET returns %d rules, want the 2 rules that PUT wrote", len(got.Rules))
	}
}

func TestPutAccessRecordsTheEventAccessAppliedWithTheCountOfRules(t *testing.T) {
	cfgPath := writeAccessConfig(t, &access.RuleSet{}, "alpha", "beta")
	r := newTestReconciler(cfgPath)
	_, client, cleanup := startTestServer(t, r)
	defer cleanup()

	body := `{"mode":"enforce","rules":[` +
		`{"from":"alpha","to":"beta"},{"from":"beta","to":"internet"},` +
		`{"from":"alpha","to":"host","ports":["tcp/22"]}]}`
	code, payload := callAccess(t, client, http.MethodPut, "/api/access", body)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusOK, payload)
	}

	var applied *reconciler.Event
	for i, event := range r.Events() {
		if event.Type == "access.applied" {
			applied = &r.Events()[i]
		}
	}
	if applied == nil {
		t.Fatalf("the daemon recorded no access.applied event; it recorded %+v", r.Events())
	}
	if !strings.Contains(applied.Message, "3") {
		t.Errorf("access.applied message = %q, want the count of rules", applied.Message)
	}
	if applied.Time.IsZero() || time.Since(applied.Time) > time.Minute {
		t.Errorf("access.applied time = %v, want the time of the request", applied.Time)
	}
}

func TestTheStatusResponseCarriesTheAccessModeAndTheRuleCount(t *testing.T) {
	set := &access.RuleSet{
		Mode: access.ModeObserve,
		Rules: []access.Rule{
			{From: "alpha", To: access.Internet},
			{From: "beta", To: access.Internet},
		},
	}
	cfgPath := writeAccessConfig(t, set, "alpha", "beta")
	_, client, cleanup := startTestServer(t, newTestReconciler(cfgPath))
	defer cleanup()

	code, payload := callAccess(t, client, http.MethodGet, "/api/status", "")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusOK, payload)
	}

	var got struct {
		Access *struct {
			Mode  string `json:"mode"`
			Rules int    `json:"rules"`
		} `json:"access"`
	}
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("decode the body %s: %v", payload, err)
	}
	if got.Access == nil {
		t.Fatalf("the status body %s holds no access field", payload)
	}
	if got.Access.Mode != access.ModeObserve {
		t.Errorf("access.mode = %q, want %q", got.Access.Mode, access.ModeObserve)
	}
	if got.Access.Rules != 2 {
		t.Errorf("access.rules = %d, want 2", got.Access.Rules)
	}
}

func TestNoAccessResponseBodyHoldsAnAuthKey(t *testing.T) {
	const authKey = "tskey-auth-kNotARealKey-000000000000"

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := config.DefaultConfig()
	cfg.Tailnets = append(cfg.Tailnets,
		config.Tailnet{ID: "alpha", AuthKey: authKey},
		config.Tailnet{ID: "beta"})
	cfg.Access = &access.RuleSet{Rules: []access.Rule{{From: "alpha", To: access.Internet}}}
	if err := config.SaveConfig(cfgPath, cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	_, client, cleanup := startTestServer(t, newTestReconciler(cfgPath))
	defer cleanup()

	// The configuration file must really hold the key, so that the assertion tests the
	// response rather than an empty field. See SA-1.
	loaded, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if loaded.Tailnets[0].AuthKey != authKey {
		t.Fatalf("the configuration file holds the key %q, want %q", loaded.Tailnets[0].AuthKey, authKey)
	}

	calls := []struct {
		method string
		target string
		body   string
	}{
		{http.MethodGet, "/api/access", ""},
		{http.MethodPut, "/api/access?dry_run=true", `{"mode":"enforce","rules":[{"from":"beta","to":"internet"}]}`},
		{http.MethodPut, "/api/access", `{"mode":"enforce","rules":[{"from":"beta","to":"internet"}]}`},
		{http.MethodGet, "/api/status", ""},
	}
	for _, call := range calls {
		_, payload := callAccess(t, client, call.method, call.target, call.body)
		if strings.Contains(string(payload), authKey) {
			t.Errorf("%s %s returns the auth key in the body %s", call.method, call.target, payload)
		}
		if strings.Contains(string(payload), "tskey-") {
			t.Errorf("%s %s returns a value that starts with tskey- in the body %s", call.method, call.target, payload)
		}
	}
}

func TestAccessRejectsAMethodThatIsNotGetAndNotPut(t *testing.T) {
	cfgPath := writeAccessConfig(t, &access.RuleSet{}, "alpha")
	_, client, cleanup := startTestServer(t, newTestReconciler(cfgPath))
	defer cleanup()

	code, payload := callAccess(t, client, http.MethodPost, "/api/access", `{"mode":"enforce"}`)
	if code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d; body %s", code, http.StatusMethodNotAllowed, payload)
	}
}
