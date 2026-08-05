package api

import (
	"context"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hydrascale/internal/config"
	"hydrascale/internal/reconciler"
)

// testAuthKeyValue is the credential that the routes must never return. The tests match
// the whole response body against this value, so a new route that encodes a tailnet
// fails here as well.
const testAuthKeyValue = "tskey-auth-kTESTSECRET123"

// configWithAuthKey writes a configuration file that holds one tailnet with an auth key,
// and it returns the path of that file.
func configWithAuthKey(t *testing.T) string {
	t.Helper()
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	cfg := config.DefaultConfig()
	cfg.Tailnets = append(cfg.Tailnets, config.Tailnet{ID: "corp", AuthKey: testAuthKeyValue})
	if err := config.SaveConfig(cfgPath, cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	return cfgPath
}

// getRawBody sends a GET request to path over the socket of srv, and it returns the
// response body without any decode step.
func getRawBody(t *testing.T, srv *Server, path string) string {
	t.Helper()
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", srv.socketPath)
		},
	}
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	resp, err := client.Get("http://localhost" + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body of %s: %v", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s returned status %d, want 200", path, resp.StatusCode)
	}
	return string(body)
}

func TestGetStatus_returns_no_auth_key_value(t *testing.T) {
	cfgPath := configWithAuthKey(t)
	r := reconciler.New(cfgPath, newMockNS(), newMockDaemon(), &mockRouting{}, 1*time.Second, nil, "10.200.0.0/16")
	srv, _, cleanup := startTestServer(t, r)
	defer cleanup()

	body := getRawBody(t, srv, "/api/status")
	if strings.Contains(body, testAuthKeyValue) {
		t.Errorf("GET /api/status returned the auth key value, body: %s", body)
	}
	if !strings.Contains(body, "corp") {
		t.Errorf("GET /api/status returned no tailnet, body: %s", body)
	}
}

func TestGetConfig_returns_no_auth_key_value(t *testing.T) {
	cfgPath := configWithAuthKey(t)
	r := reconciler.New(cfgPath, newMockNS(), newMockDaemon(), &mockRouting{}, 1*time.Second, nil, "10.200.0.0/16")
	srv, _, cleanup := startTestServer(t, r)
	defer cleanup()

	body := getRawBody(t, srv, "/api/config")
	if strings.Contains(body, testAuthKeyValue) {
		t.Errorf("GET /api/config returned the auth key value, body: %s", body)
	}
}
