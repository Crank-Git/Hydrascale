package api

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hydrascale/internal/config"
	"hydrascale/internal/reconciler"
	"hydrascale/internal/ui"
)

// captureLog sends the standard logger to a buffer for the length of the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	flags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(io.Discard)
		log.SetFlags(flags)
	})
	return &buf
}

// startTestConsole starts a console listener on a free loopback port and returns the
// server and the console origin, such as http://127.0.0.1:41234.
func startTestConsole(t *testing.T, r *reconciler.Reconciler) (*Server, string) {
	t.Helper()
	srv := NewServer(tempSocketPath(t, "console-api.sock"), r)
	if err := srv.StartConsole(true, "127.0.0.1:0"); err != nil {
		t.Fatalf("StartConsole: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})
	return srv, "http://" + srv.ConsoleAddress()
}

// consoleCall sends one request to the console listener and returns the response.
func consoleCall(t *testing.T, method, url, body string, headers map[string]string) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// consoleHeader is the header set that a console request carries.
var consoleHeader = map[string]string{ConsoleRequestHeader: "1"}

func TestTheDaemonRefusesANonLoopbackConsoleBindAddress(t *testing.T) {
	logs := captureLog(t)
	srv := NewServer(tempSocketPath(t, "refuse-api.sock"), newTestReconciler(writeTestConfig(t, "alpha")))

	err := srv.StartConsole(true, "0.0.0.0:9443")
	if err == nil {
		t.Fatal("StartConsole accepted 0.0.0.0:9443, want an error")
	}
	if !strings.Contains(err.Error(), "loopback") {
		t.Errorf("the error %q does not state that the console must bind a loopback address", err)
	}
	if !strings.Contains(err.Error(), "console.bind_address") {
		t.Errorf("the error %q does not name the configuration key console.bind_address", err)
	}
	if !strings.Contains(logs.String(), "loopback") {
		t.Errorf("the log %q does not hold the refusal", logs.String())
	}
	if srv.ConsoleAddress() != "" {
		t.Errorf("ConsoleAddress() = %q, want the empty string after a refusal", srv.ConsoleAddress())
	}
}

func TestTheDaemonStartsTheConsoleOnEachLoopbackAddress(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:0", "[::1]:0"} {
		srv := NewServer(tempSocketPath(t, "loop-api.sock"), newTestReconciler(writeTestConfig(t, "alpha")))
		if err := srv.StartConsole(true, addr); err != nil {
			t.Errorf("StartConsole(%q) = %v, want nil", addr, err)
			continue
		}
		if srv.ConsoleAddress() == "" {
			t.Errorf("StartConsole(%q) opened no listener", addr)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		if err := srv.Shutdown(ctx); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
		cancel()
	}
}

func TestConsoleEnabledFalseOpensNoListener(t *testing.T) {
	srv := NewServer(tempSocketPath(t, "off-api.sock"), newTestReconciler(writeTestConfig(t, "alpha")))

	if err := srv.StartConsole(false, "127.0.0.1:0"); err != nil {
		t.Fatalf("StartConsole: %v", err)
	}
	if srv.ConsoleAddress() != "" {
		t.Errorf("ConsoleAddress() = %q, want the empty string when console.enabled is false", srv.ConsoleAddress())
	}
}

func TestAConsolePortInUseNamesThePortAndTheConfigurationKey(t *testing.T) {
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer taken.Close()
	_, port, err := net.SplitHostPort(taken.Addr().String())
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}

	srv := NewServer(tempSocketPath(t, "busy-api.sock"), newTestReconciler(writeTestConfig(t, "alpha")))
	err = srv.StartConsole(true, taken.Addr().String())
	if err == nil {
		t.Fatal("StartConsole accepted an address that is in use, want an error")
	}
	if !strings.Contains(err.Error(), port) {
		t.Errorf("the error %q does not name the port %s", err, port)
	}
	if !strings.Contains(err.Error(), "console.bind_address") {
		t.Errorf("the error %q does not name the configuration key console.bind_address", err)
	}
}

func TestTheConsoleStartLogNamesTheAddressAndTheNoAuthenticationStatement(t *testing.T) {
	logs := captureLog(t)
	srv, _ := startTestConsole(t, newTestReconciler(writeTestConfig(t, "alpha")))

	line := logs.String()
	if !strings.Contains(line, srv.ConsoleAddress()) {
		t.Errorf("the log %q does not name the console address %s", line, srv.ConsoleAddress())
	}
	if !strings.Contains(line, "no authentication") {
		t.Errorf("the log %q does not state that the console has no authentication", line)
	}
}

func TestAMutatingConsoleRequestWithoutTheConsoleHeaderReturns403(t *testing.T) {
	// A browser cannot set a custom header on a cross-origin form post, so this control
	// stops a hostile web page. It stops no local account. See spec.md, control 2.
	_, origin := startTestConsole(t, newTestReconciler(writeTestConfig(t, "alpha")))

	resp := consoleCall(t, http.MethodPost, origin+"/api/tailnet/remove", `{"id":"alpha"}`, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("POST /api/tailnet/remove with no console header returns %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

func TestAMutatingConsoleRequestWithTheConsoleHeaderIsAccepted(t *testing.T) {
	_, origin := startTestConsole(t, newTestReconciler(writeTestConfig(t, "alpha")))

	resp := consoleCall(t, http.MethodPost, origin+"/api/tailnet/remove", `{"id":"alpha"}`, consoleHeader)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("POST /api/tailnet/remove with the console header returns %d and the body %s, want %d", resp.StatusCode, body, http.StatusOK)
	}
}

func TestAReadRouteNeedsNoConsoleHeader(t *testing.T) {
	// FR-console-8 names a mutating route only. The console reads GET /api/status on
	// every poll and a read route changes nothing.
	_, origin := startTestConsole(t, newTestReconciler(writeTestConfig(t, "alpha")))

	resp := consoleCall(t, http.MethodGet, origin+"/api/status", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /api/status with no console header returns %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestAForeignOriginReturns403(t *testing.T) {
	_, origin := startTestConsole(t, newTestReconciler(writeTestConfig(t, "alpha")))

	headers := map[string]string{ConsoleRequestHeader: "1", "Origin": "http://evil.example"}
	resp := consoleCall(t, http.MethodPost, origin+"/api/tailnet/remove", `{"id":"alpha"}`, headers)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("POST with Origin: http://evil.example returns %d, want %d", resp.StatusCode, http.StatusForbidden)
	}

	read := consoleCall(t, http.MethodGet, origin+"/api/status", "", map[string]string{"Origin": "http://evil.example"})
	if read.StatusCode != http.StatusForbidden {
		t.Errorf("GET with Origin: http://evil.example returns %d, want %d", read.StatusCode, http.StatusForbidden)
	}
}

func TestARequestWithNoOriginIsAccepted(t *testing.T) {
	// A browser omits Origin on a same-origin navigation and a command line client omits
	// it too, so an absent Origin is not evidence of a cross-origin request.
	_, origin := startTestConsole(t, newTestReconciler(writeTestConfig(t, "alpha")))

	resp := consoleCall(t, http.MethodPost, origin+"/api/tailnet/remove", `{"id":"alpha"}`, consoleHeader)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("POST with no Origin returns %d and the body %s, want %d", resp.StatusCode, body, http.StatusOK)
	}
}

func TestTheConsoleOriginIsAccepted(t *testing.T) {
	_, origin := startTestConsole(t, newTestReconciler(writeTestConfig(t, "alpha")))

	headers := map[string]string{ConsoleRequestHeader: "1", "Origin": origin}
	resp := consoleCall(t, http.MethodPost, origin+"/api/tailnet/remove", `{"id":"alpha"}`, headers)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("POST with the console origin returns %d and the body %s, want %d", resp.StatusCode, body, http.StatusOK)
	}
}

func TestEveryConsoleResponseCarriesTheContentSecurityPolicyHeader(t *testing.T) {
	_, origin := startTestConsole(t, newTestReconciler(writeTestConfig(t, "alpha")))

	targets := []struct {
		method string
		path   string
		body   string
		header map[string]string
	}{
		{http.MethodGet, "/", "", nil},
		{http.MethodGet, "/brand/tokens/colors.css", "", nil},
		{http.MethodGet, "/api/status", "", nil},
		{http.MethodGet, "/api/events", "", nil},
		{http.MethodPost, "/api/tailnet/remove", `{"id":"alpha"}`, consoleHeader},
		{http.MethodPost, "/api/tailnet/remove", `{"id":"alpha"}`, nil},
	}
	for _, target := range targets {
		resp := consoleCall(t, target.method, origin+target.path, target.body, target.header)
		if got := resp.Header.Get("Content-Security-Policy"); got != ConsoleContentSecurityPolicy {
			t.Errorf("%s %s carries the policy %q, want %q", target.method, target.path, got, ConsoleContentSecurityPolicy)
		}
	}
}

func TestAMutatingConsoleRequestRecordsTheConsoleRequestEvent(t *testing.T) {
	r := newTestReconciler(writeTestConfig(t, "alpha"))
	_, origin := startTestConsole(t, r)

	consoleCall(t, http.MethodPost, origin+"/api/tailnet/remove", `{"id":"alpha"}`, consoleHeader)

	found := false
	for _, event := range r.Events() {
		if event.Type != EventConsoleRequest {
			continue
		}
		found = true
		if !strings.Contains(event.Message, "/api/tailnet/remove") {
			t.Errorf("the event message %q does not name the route", event.Message)
		}
		if !strings.Contains(event.Message, http.MethodPost) {
			t.Errorf("the event message %q does not name the method", event.Message)
		}
	}
	if !found {
		t.Errorf("the reconciler holds no %s event after a mutating console request", EventConsoleRequest)
	}
}

func TestAReadConsoleRequestRecordsNoEvent(t *testing.T) {
	// FR-console-10 names a mutating request. A poll every 5 seconds would otherwise
	// fill the event list and hide every real action.
	r := newTestReconciler(writeTestConfig(t, "alpha"))
	_, origin := startTestConsole(t, r)

	consoleCall(t, http.MethodGet, origin+"/api/status", "", nil)

	for _, event := range r.Events() {
		if event.Type == EventConsoleRequest {
			t.Errorf("GET /api/status records the event %s with the message %q", event.Type, event.Message)
		}
	}
}

func TestTheSameJSONRouteAnswersOnTheControlSocketAndOnTheConsoleListener(t *testing.T) {
	cfgPath := writeTestConfig(t, "alpha", "beta")
	r := newTestReconciler(cfgPath)

	socketPath := tempSocketPath(t, "both-api.sock")
	srv := NewServer(socketPath, r)
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := srv.StartConsole(true, "127.0.0.1:0"); err != nil {
		t.Fatalf("StartConsole: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})

	socketStatus, err := NewClient(socketPath).Status()
	if err != nil {
		t.Fatalf("Status on the control socket: %v", err)
	}

	resp := consoleCall(t, http.MethodGet, "http://"+srv.ConsoleAddress()+"/api/status", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/status on the console listener returns %d, want %d", resp.StatusCode, http.StatusOK)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	for _, id := range []string{"alpha", "beta"} {
		if _, ok := socketStatus.Desired[id]; !ok {
			t.Errorf("the control socket answer holds no tailnet %s", id)
		}
		if !strings.Contains(string(body), id) {
			t.Errorf("the console answer holds no tailnet %s: %s", id, body)
		}
	}
}

func TestTheConsoleListenerServesTheEmbeddedIndexPage(t *testing.T) {
	_, origin := startTestConsole(t, newTestReconciler(writeTestConfig(t, "alpha")))

	resp := consoleCall(t, http.MethodGet, origin+"/", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / returns %d, want %d", resp.StatusCode, http.StatusOK)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !strings.Contains(string(body), "Hydrascale") {
		t.Errorf("GET / returns a body that does not name the product: %s", body)
	}
}

func TestNoConsoleResponseBodyHoldsAnAuthKey(t *testing.T) {
	const authKey = "tskey-auth-kNotARealKey-000000000000"

	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	cfg := config.DefaultConfig()
	cfg.Tailnets = append(cfg.Tailnets, config.Tailnet{ID: "alpha", AuthKey: authKey})
	if err := config.SaveConfig(cfgPath, cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	// The configuration file must really hold the key, so that the assertion tests the
	// response rather than an empty field. See SA-1.
	loaded, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if loaded.Tailnets[0].AuthKey != authKey {
		t.Fatalf("the configuration file holds the key %q, want %q", loaded.Tailnets[0].AuthKey, authKey)
	}

	_, origin := startTestConsole(t, newTestReconciler(cfgPath))

	targets := []struct {
		method string
		path   string
		body   string
		header map[string]string
	}{
		{http.MethodGet, "/api/status", "", nil},
		{http.MethodGet, "/api/config", "", nil},
		{http.MethodGet, "/api/events", "", nil},
		{http.MethodPost, "/api/tailnet/remove", `{"id":"alpha"}`, consoleHeader},
		{http.MethodGet, "/api/events", "", nil},
	}
	for _, target := range targets {
		resp := consoleCall(t, target.method, origin+target.path, target.body, target.header)
		payload, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		if strings.Contains(string(payload), authKey) {
			t.Errorf("%s %s returns the auth key in the body %s", target.method, target.path, payload)
		}
		if strings.Contains(string(payload), "tskey-") {
			t.Errorf("%s %s returns a value that starts with tskey- in the body %s", target.method, target.path, payload)
		}
	}
}

func TestTheConsoleNamesTheSocketPathThatTheDaemonOpens(t *testing.T) {
	// The error state of the overview names the socket path, and the console reads that
	// path from no route. This test holds the two values together, so a change to
	// DefaultSocketPath cannot leave the console with a path that the daemon does not open.
	source, err := fs.ReadFile(ui.Files(), "topology.js")
	if err != nil {
		t.Fatalf("read the console topology module: %v", err)
	}
	if !strings.Contains(string(source), `"`+DefaultSocketPath+`"`) {
		t.Errorf("the console names no socket path %q, so its error state names a path that the daemon does not open", DefaultSocketPath)
	}
}
