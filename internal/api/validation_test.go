package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"hydrascale/internal/config"
	"hydrascale/internal/reconciler"
)

// startValidationServer starts a server on a temporary socket, and it returns the
// configuration path and a function that sends a POST request to a route.
// The post function returns the status code and the response body.
// startValidationServer fails the test when the server does not start.
func startValidationServer(t *testing.T, tailnets ...string) (cfgPath string, post func(path, body string) (int, string)) {
	t.Helper()
	cfgPath = writeTestConfig(t, tailnets...)
	r := reconciler.New(cfgPath, newMockNS(), newMockDaemon(), &mockRouting{}, time.Second, nil, "10.200.0.0/16")
	socketPath := tempSocketPath(t, "val-api.sock")
	srv := NewServer(socketPath, r)
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})

	httpClient := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
			},
		},
	}
	post = func(path, body string) (int, string) {
		t.Helper()
		resp, err := httpClient.Post("http://localhost"+path, "application/json", bytes.NewReader([]byte(body)))
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read body of %s: %v", path, err)
		}
		return resp.StatusCode, string(data)
	}
	return cfgPath, post
}

// assertJSONError fails the test when status is not 400, and when body is not the object
// {"error": "<message>"} with a message that is not empty.
func assertJSONError(t *testing.T, status int, body string) {
	t.Helper()
	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body %s", status, http.StatusBadRequest, body)
	}
	var decoded struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("body %q does not parse as JSON: %v", body, err)
	}
	if decoded.Error == "" {
		t.Errorf("body %q holds no error message", body)
	}
}

// readFile returns the content of path, and it fails the test when the read fails.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", path, err)
	}
	return string(data)
}

func TestAddTailnet_rejects_a_tailnet_identifier_that_holds_a_space(t *testing.T) {
	cfgPath, post := startValidationServer(t)
	before := readFile(t, cfgPath)

	status, body := post("/api/tailnet/add", `{"id":"My Net"}`)
	assertJSONError(t, status, body)

	if after := readFile(t, cfgPath); after != before {
		t.Errorf("the configuration file changed on a failed validation:\nbefore %s\nafter %s", before, after)
	}
}

func TestAddTailnet_rejects_a_control_url_that_uses_the_http_scheme(t *testing.T) {
	cfgPath, post := startValidationServer(t)
	before := readFile(t, cfgPath)

	status, body := post("/api/tailnet/add", `{"id":"gamma","control_url":"http://example.com"}`)
	assertJSONError(t, status, body)

	if after := readFile(t, cfgPath); after != before {
		t.Errorf("the configuration file changed on a failed validation:\nbefore %s\nafter %s", before, after)
	}
}

func TestAddTailnet_accepts_a_control_url_on_a_loopback_address(t *testing.T) {
	cfgPath, post := startValidationServer(t)

	status, body := post("/api/tailnet/add", `{"id":"gamma","control_url":"http://127.0.0.1:8080"}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %s", status, http.StatusOK, body)
	}

	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	found := false
	for _, tn := range cfg.Tailnets {
		if tn.ID == "gamma" {
			found = true
			if tn.ControlURL != "http://127.0.0.1:8080" {
				t.Errorf("control_url = %q, want %q", tn.ControlURL, "http://127.0.0.1:8080")
			}
		}
	}
	if !found {
		t.Error("the route did not write the tailnet gamma")
	}
}

func TestAddTailnet_rejects_an_empty_tailnet_identifier(t *testing.T) {
	_, post := startValidationServer(t)

	status, body := post("/api/tailnet/add", `{"id":""}`)
	assertJSONError(t, status, body)
}

func TestRemoveTailnet_rejects_a_tailnet_identifier_that_holds_a_path_separator(t *testing.T) {
	cfgPath, post := startValidationServer(t, "alpha")
	before := readFile(t, cfgPath)

	status, body := post("/api/tailnet/remove", `{"id":"../../tmp/x"}`)
	assertJSONError(t, status, body)

	if after := readFile(t, cfgPath); after != before {
		t.Errorf("the configuration file changed on a failed validation:\nbefore %s\nafter %s", before, after)
	}
}

func TestConnectTailnet_rejects_a_tailnet_identifier_that_holds_a_space(t *testing.T) {
	_, post := startValidationServer(t, "alpha")

	status, body := post("/api/tailnet/connect", `{"id":"My Net"}`)
	assertJSONError(t, status, body)
}

func TestConnectTailnet_rejects_an_identifier_that_names_no_tailnet(t *testing.T) {
	_, post := startValidationServer(t, "alpha")

	status, body := post("/api/tailnet/connect", `{"id":"beta"}`)
	assertJSONError(t, status, body)
}

func TestDisconnectTailnet_rejects_an_identifier_that_holds_a_path_separator(t *testing.T) {
	_, post := startValidationServer(t, "alpha")

	status, body := post("/api/tailnet/disconnect", `{"id":"../../tmp/x"}`)
	assertJSONError(t, status, body)
}

func TestDisconnectTailnet_rejects_an_identifier_that_names_no_tailnet(t *testing.T) {
	_, post := startValidationServer(t, "alpha")

	status, body := post("/api/tailnet/disconnect", `{"id":"beta"}`)
	assertJSONError(t, status, body)
}

func TestConfigDNS_rejects_an_unknown_resolver_mode(t *testing.T) {
	cfgPath, post := startValidationServer(t)
	before := readFile(t, cfgPath)

	status, body := post("/api/config/dns", `{"mode":"split"}`)
	assertJSONError(t, status, body)

	if after := readFile(t, cfgPath); after != before {
		t.Errorf("the configuration file changed on a failed validation:\nbefore %s\nafter %s", before, after)
	}
}

func TestConfigDNS_rejects_a_bind_address_that_is_not_loopback(t *testing.T) {
	cfgPath, post := startValidationServer(t)
	before := readFile(t, cfgPath)

	status, body := post("/api/config/dns", `{"mode":"unified","bind_address":"0.0.0.0:53"}`)
	assertJSONError(t, status, body)

	if after := readFile(t, cfgPath); after != before {
		t.Errorf("the configuration file changed on a failed validation:\nbefore %s\nafter %s", before, after)
	}
}

func TestMutatingRoutes_reject_a_body_that_does_not_parse_as_json(t *testing.T) {
	_, post := startValidationServer(t, "alpha")

	for _, path := range []string{
		"/api/tailnet/add",
		"/api/tailnet/remove",
		"/api/tailnet/connect",
		"/api/tailnet/disconnect",
		"/api/config/dns",
	} {
		status, body := post(path, `{`)
		assertJSONError(t, status, body)
	}
}
