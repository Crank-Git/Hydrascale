package api

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hydrascale/internal/config"
	"hydrascale/internal/daemon"
	"hydrascale/internal/namespaces"
	"hydrascale/internal/reconciler"
)

// The test file declares the response shape rather than reading it from the package, so
// that a change to a Go type never hides a change to the wire format that the console
// reads.

type wireRemovalPlan struct {
	ID        string   `json:"id"`
	Namespace string   `json:"namespace"`
	HostVeth  string   `json:"host_veth"`
	StateDir  string   `json:"state_dir"`
	Commands  []string `json:"commands"`
	RuleCount int      `json:"rule_count"`
}

type wireDetail struct {
	TailscaleIPs []string `json:"tailscale_ips"`
	MagicDNSName string   `json:"magic_dns_name"`
	PeerCount    int      `json:"peer_count"`
	BackendState string   `json:"backend_state"`
	LoginURL     string   `json:"login_url"`
}

// reconcilerWithDaemon returns a reconciler over cfgPath and the mock daemon that it
// holds, so that a test states the tailscale status that the routes read.
func reconcilerWithDaemon(cfgPath string) (*reconciler.Reconciler, *mockDaemon) {
	dm := newMockDaemon()
	return reconciler.New(cfgPath, newMockNS(), dm, &mockRouting{}, 1*time.Second, nil, "10.200.0.0/16"), dm
}

func TestTheRemovalPlanRouteNamesEveryCommandThatTheRemovalRuns(t *testing.T) {
	// FR-console-29. The console dialog names every command, therefore the daemon states
	// them. The console repeats no rule of the daemon.
	cfgPath := writeTestConfig(t, "alpha")
	_, client, cleanup := startTestServer(t, newTestReconciler(cfgPath))
	defer cleanup()

	status, payload := callAccess(t, client, http.MethodGet, "/api/tailnet/alpha/removal-plan", "")
	if status != http.StatusOK {
		t.Fatalf("GET /api/tailnet/alpha/removal-plan returns %d and the body %s, want %d", status, payload, http.StatusOK)
	}

	var plan wireRemovalPlan
	if err := json.Unmarshal(payload, &plan); err != nil {
		t.Fatalf("parse the plan %s: %v", payload, err)
	}

	nsName := namespaces.GetNamespaceName("alpha")
	hostVeth, _ := namespaces.VethNames(nsName)

	if plan.ID != "alpha" {
		t.Errorf("the plan names the tailnet %q, want alpha", plan.ID)
	}
	if plan.Namespace != nsName {
		t.Errorf("the plan names the namespace %q, want %q", plan.Namespace, nsName)
	}
	if plan.HostVeth != hostVeth {
		t.Errorf("the plan names the veth device %q, want %q", plan.HostVeth, hostVeth)
	}
	if !strings.HasSuffix(plan.StateDir, "/alpha") {
		t.Errorf("the plan names the state directory %q, and it holds no tailnet identifier", plan.StateDir)
	}
	if plan.RuleCount < 1 {
		t.Errorf("the plan states %d iptables rules, want 1 or more", plan.RuleCount)
	}

	joined := strings.Join(plan.Commands, "\n")
	for _, want := range []string{nsName, hostVeth, plan.StateDir, "iptables"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the commands of the plan name no %s:\n%s", want, joined)
		}
	}
}

func TestTheRemovalPlanRouteRefusesAnUnknownTailnet(t *testing.T) {
	cfgPath := writeTestConfig(t, "alpha")
	_, client, cleanup := startTestServer(t, newTestReconciler(cfgPath))
	defer cleanup()

	status, payload := callAccess(t, client, http.MethodGet, "/api/tailnet/beta/removal-plan", "")
	if status != http.StatusNotFound {
		t.Errorf("GET /api/tailnet/beta/removal-plan returns %d and the body %s, want %d", status, payload, http.StatusNotFound)
	}
}

func TestTheDetailRouteStatesTheBackendStateAndTheLoginURL(t *testing.T) {
	// The namespace view shows a warning dot for a tailnet that is not authenticated, and
	// the panel shows the login URL. The daemon reads both from tailscale status --json,
	// therefore the console invents neither.
	const loginURL = "https://controlplane.example.net/register/nodekey:0000"

	cfgPath := writeTestConfig(t, "alpha")
	r, dm := reconcilerWithDaemon(cfgPath)
	dm.statusResult = &daemon.TailscaleStatus{
		BackendState: "NeedsLogin",
		AuthURL:      loginURL,
	}

	_, client, cleanup := startTestServer(t, r)
	defer cleanup()

	status, payload := callAccess(t, client, http.MethodGet, "/api/tailnet/alpha/detail", "")
	if status != http.StatusOK {
		t.Fatalf("GET /api/tailnet/alpha/detail returns %d and the body %s, want %d", status, payload, http.StatusOK)
	}

	var detail wireDetail
	if err := json.Unmarshal(payload, &detail); err != nil {
		t.Fatalf("parse the detail %s: %v", payload, err)
	}
	if detail.BackendState != "NeedsLogin" {
		t.Errorf("the detail states the backend state %q, want NeedsLogin", detail.BackendState)
	}
	if detail.LoginURL != loginURL {
		t.Errorf("the detail states the login URL %q, want %q", detail.LoginURL, loginURL)
	}
}

func TestNoTailnetAddResponseBodyHoldsAnAuthKey(t *testing.T) {
	// FR-console-33. The add flow sends the auth key once, and no route returns it. SA-1
	// was an auth key in the body of GET /api/status.
	const authKey = "tskey-auth-kNotARealKey-000000000000"

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := config.DefaultConfig()
	if err := config.SaveConfig(cfgPath, cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	_, client, cleanup := startTestServer(t, newTestReconciler(cfgPath))
	defer cleanup()

	addBody := `{"id":"alpha","auth_key":"` + authKey + `"}`
	calls := []struct {
		method string
		target string
		body   string
	}{
		{http.MethodPost, "/api/tailnet/add", addBody},
		{http.MethodGet, "/api/status", ""},
		{http.MethodGet, "/api/config", ""},
		{http.MethodGet, "/api/tailnet/alpha/detail", ""},
		{http.MethodGet, "/api/tailnet/alpha/removal-plan", ""},
	}
	for _, call := range calls {
		_, payload := callAccess(t, client, call.method, call.target, call.body)
		if strings.Contains(string(payload), authKey) {
			t.Errorf("%s %s returns the auth key in the body %s", call.method, call.target, payload)
		}
	}

	// The add must really have written the key, so that the assertions above test the
	// responses rather than an empty field.
	loaded, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(loaded.Tailnets) != 1 {
		t.Fatalf("the configuration file holds %d tailnets after the add, want 1", len(loaded.Tailnets))
	}
	if loaded.Tailnets[0].AuthKey != authKey {
		t.Fatalf("the configuration file holds the key %q, want %q", loaded.Tailnets[0].AuthKey, authKey)
	}
}
