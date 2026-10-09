package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hydrascale/internal/access"
	"hydrascale/internal/config"
)

// writePublishConfig writes a configuration file that declares the tailnets alpha and
// beta, publishes tcp/22 on alpha with host access on, and holds a rule set that covers
// the published port. It returns the path of the file.
func writePublishConfig(t *testing.T) string {
	t.Helper()
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	on := true
	cfg := config.DefaultConfig()
	cfg.Tailnets = []config.Tailnet{
		{ID: "alpha", HostAccess: &on, Publish: []string{"tcp/22"}},
		{ID: "beta"},
	}
	cfg.Access = &access.RuleSet{Rules: []access.Rule{{From: "alpha", To: access.Host, Ports: []string{"tcp/22"}}}}
	if err := config.SaveConfig(cfgPath, cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	return cfgPath
}

func TestPutAccessRefusesARuleSetThatUncoversAPublishedPort(t *testing.T) {
	cfgPath := writePublishConfig(t)
	_, client, cleanup := startTestServer(t, newTestReconciler(cfgPath))
	defer cleanup()

	before, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	body := `{"mode":"enforce","rules":[{"from":"alpha","to":"host","ports":["tcp/443"]}]}`
	code, payload := callAccess(t, client, http.MethodPut, "/api/access", body)
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusBadRequest, payload)
	}

	var refusal map[string]any
	if err := json.Unmarshal(payload, &refusal); err != nil {
		t.Fatalf("decode the body %s: %v", payload, err)
	}
	message, _ := refusal["error"].(string)
	for _, want := range []string{`"alpha"`, `"tcp/22"`, "from: alpha, to: host"} {
		if !strings.Contains(message, want) {
			t.Errorf("error = %q, want a message that holds %s", message, want)
		}
	}

	after, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("the route changed the configuration file:\nbefore %s\nafter  %s", before, after)
	}
}

func TestTheStatusResponseCarriesThePublishListOfEachTailnet(t *testing.T) {
	cfgPath := writePublishConfig(t)
	_, client, cleanup := startTestServer(t, newTestReconciler(cfgPath))
	defer cleanup()

	code, payload := callAccess(t, client, http.MethodGet, "/api/status", "")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusOK, payload)
	}

	var got struct {
		Desired map[string]map[string]any `json:"desired"`
	}
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("decode the body %s: %v", payload, err)
	}
	list, ok := got.Desired["alpha"]["publish"].([]any)
	if !ok || len(list) != 1 || list[0] != "tcp/22" {
		t.Errorf("desired.alpha.publish = %v, want [tcp/22]; body %s", got.Desired["alpha"]["publish"], payload)
	}
}
