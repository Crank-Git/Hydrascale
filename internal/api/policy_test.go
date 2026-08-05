package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"hydrascale/internal/config"
	"hydrascale/internal/reconciler"
	"hydrascale/internal/secrets"
)

// The test file declares the response shape rather than reading it from the package, so
// that a change to a Go type never hides a change to the wire format that the console
// reads.

type wirePolicyTailnet struct {
	ID                string `json:"id"`
	Kind              string `json:"kind"`
	CredentialPresent bool   `json:"credential_present"`
	WriteAvailable    bool   `json:"write_available"`
	Reason            string `json:"reason"`
}

type wirePolicyList struct {
	Tailnets []wirePolicyTailnet `json:"tailnets"`
}

type wirePolicy struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	Document       string `json:"document"`
	ETag           string `json:"etag"`
	WriteAvailable bool   `json:"write_available"`
}

type wireValidate struct {
	Passed bool   `json:"passed"`
	Result string `json:"result"`
}

type wireError struct {
	Error string `json:"error"`
}

// The credential values that every test uses. No test reaches a real control server, so
// these values are the only credential in the test suite.
const (
	testClientID     = "k-test-client-id"
	testClientSecret = "tskey-client-secret-value"
	testAPIKey       = "hskey-api-key-value"
	testAccessToken  = "tskey-access-token-value"
)

// fakeTailscale answers the four Tailscale routes that the daemon calls.
// The test sets document, etag, validateResult, and writeStatus before the call, and it
// reads ifMatch and paths after the call.
type fakeTailscale struct {
	server *httptest.Server

	mu             sync.Mutex
	document       string
	etag           string
	writtenETag    string
	validateResult string
	writeStatus    int
	ifMatch        string
	paths          []string
}

// newFakeTailscale returns a local Tailscale control server that holds document.
func newFakeTailscale(t *testing.T, document, etag string) *fakeTailscale {
	t.Helper()
	f := &fakeTailscale{document: document, etag: etag, writtenETag: etag + "-written"}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeTailscale) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.paths = append(f.paths, r.Method+" "+r.URL.Path)

	switch r.URL.Path {
	case "/oauth/token":
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"access_token":%q,"token_type":"Bearer","expires_in":3600}`, testAccessToken)
	case "/tailnet/-/acl/validate":
		if f.validateResult != "" {
			fmt.Fprint(w, f.validateResult)
		}
	case "/tailnet/-/acl":
		if r.Method == http.MethodGet {
			w.Header().Set("ETag", f.etag)
			fmt.Fprint(w, f.document)
			return
		}
		f.ifMatch = r.Header.Get("If-Match")
		if f.writeStatus != 0 {
			w.WriteHeader(f.writeStatus)
			fmt.Fprint(w, `{"message":"the policy changed"}`)
			return
		}
		body := make([]byte, 1<<16)
		n, _ := r.Body.Read(body)
		f.document = string(body[:n])
		w.Header().Set("ETag", f.writtenETag)
		fmt.Fprint(w, f.document)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *fakeTailscale) sent(path string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range f.paths {
		if p == path {
			return true
		}
	}
	return false
}

// fakeHeadscale answers the three Headscale routes that the daemon calls.
type fakeHeadscale struct {
	server *httptest.Server

	mu          sync.Mutex
	document    string
	checkError  string
	writeError  string
	writeCalled bool
}

// newFakeHeadscale returns a local Headscale control server that holds document.
func newFakeHeadscale(t *testing.T, document string) *fakeHeadscale {
	t.Helper()
	f := &fakeHeadscale{document: document}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeHeadscale) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")

	switch r.URL.Path {
	case "/api/v1/policy/check":
		if f.checkError != "" {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, `{"code":2,"message":%q,"details":[]}`, f.checkError)
			return
		}
		fmt.Fprint(w, `{}`)
	case "/api/v1/policy":
		if r.Method == http.MethodGet {
			body, _ := json.Marshal(map[string]string{"policy": f.document})
			w.Write(body)
			return
		}
		f.writeCalled = true
		if f.writeError != "" {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, `{"code":2,"message":%q,"details":[]}`, f.writeError)
			return
		}
		var request struct {
			Policy string `json:"policy"`
		}
		json.NewDecoder(r.Body).Decode(&request)
		f.document = request.Policy
		body, _ := json.Marshal(map[string]string{"policy": f.document})
		w.Write(body)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// policyFixture holds the paths and the client of one policy test.
type policyFixture struct {
	client      *Client
	reconciler  *reconciler.Reconciler
	cfgPath     string
	secretsPath string
	dir         string
}

// startPolicyServer writes a configuration file that declares tailnets and credentials,
// and it starts a control API server that reaches tailscaleBaseURL rather than the real
// Tailscale API.
// tailnets names the declared tailnets, and creds holds the credential of each tailnet
// that has one.
func startPolicyServer(t *testing.T, tailscaleBaseURL string, tailnets []config.Tailnet, creds map[string]secrets.Tailnet) *policyFixture {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	secretsPath := filepath.Join(dir, "secrets.yaml")

	cfg := config.DefaultConfig()
	cfg.Tailnets = tailnets
	cfg.SecretsFile = secretsPath
	if err := config.SaveConfig(cfgPath, cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	if len(creds) > 0 {
		if err := secrets.Save(secretsPath, &secrets.Store{Tailnets: creds}); err != nil {
			t.Fatalf("secrets.Save: %v", err)
		}
	}

	r := newTestReconciler(cfgPath)
	socketPath := tempSocketPath(t, "policy-api.sock")
	srv := NewServer(socketPath, r)
	srv.tailscaleBaseURL = tailscaleBaseURL
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	})
	return &policyFixture{client: NewClient(socketPath), reconciler: r, cfgPath: cfgPath, secretsPath: secretsPath, dir: dir}
}

// tailscaleCredential returns a complete Tailscale credential.
func tailscaleCredential() secrets.Tailnet {
	return secrets.Tailnet{TailscaleOAuthClientID: testClientID, TailscaleOAuthClientSecret: testClientSecret}
}

// headscaleCredential returns a complete Headscale credential for address.
func headscaleCredential(address string) secrets.Tailnet {
	return secrets.Tailnet{HeadscaleAPIKey: testAPIKey, HeadscaleAddress: address}
}

// decodePolicy decodes one JSON body and it fails the test when the body is not JSON.
func decodePolicy(t *testing.T, payload []byte, into interface{}) {
	t.Helper()
	if err := json.Unmarshal(payload, into); err != nil {
		t.Fatalf("decode the body %s: %v", payload, err)
	}
}

// --- GET /api/policy ---

func TestGetPolicyListsEveryTailnetWithItsKindAndItsCredentialState(t *testing.T) {
	headscale := newFakeHeadscale(t, "{}")
	fixture := startPolicyServer(t, "http://127.0.0.1:1",
		[]config.Tailnet{{ID: "alpha"}, {ID: "beta", ControlURL: headscale.server.URL}},
		map[string]secrets.Tailnet{"alpha": tailscaleCredential()})

	code, payload := callAccess(t, fixture.client, http.MethodGet, "/api/policy", "")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusOK, payload)
	}

	var got wirePolicyList
	decodePolicy(t, payload, &got)
	if len(got.Tailnets) != 2 {
		t.Fatalf("len(tailnets) = %d, want 2; body %s", len(got.Tailnets), payload)
	}
	alpha, beta := got.Tailnets[0], got.Tailnets[1]
	if alpha.ID != "alpha" || alpha.Kind != KindTailscale {
		t.Errorf("alpha = %+v, want the kind %q", alpha, KindTailscale)
	}
	if !alpha.CredentialPresent || !alpha.WriteAvailable {
		t.Errorf("alpha = %+v, want the credential present and the write available", alpha)
	}
	if beta.ID != "beta" || beta.Kind != KindHeadscale {
		t.Errorf("beta = %+v, want the kind %q", beta, KindHeadscale)
	}
	if beta.CredentialPresent || beta.WriteAvailable {
		t.Errorf("beta = %+v, want no credential and no write availability", beta)
	}
	if !strings.Contains(beta.Reason, "headscale_api_key") {
		t.Errorf("the reason of beta = %q, want the name of the credential", beta.Reason)
	}
	assertNoCredentialValue(t, payload)
}

// --- GET /api/policy/{id} ---

func TestGetOnePolicyReturnsHTTP409WhenTheTailnetHasNoCredential(t *testing.T) {
	fixture := startPolicyServer(t, "http://127.0.0.1:1", []config.Tailnet{{ID: "alpha"}}, nil)

	code, payload := callAccess(t, fixture.client, http.MethodGet, "/api/policy/alpha", "")
	if code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusConflict, payload)
	}
	var got wireError
	decodePolicy(t, payload, &got)
	for _, want := range []string{"tailscale_oauth_client_id", "tailscale_oauth_client_secret", "HYDRASCALE_TS_CLIENT_ID_ALPHA"} {
		if !strings.Contains(got.Error, want) {
			t.Errorf("the message %q does not name %q", got.Error, want)
		}
	}
}

func TestGetOnePolicyReturnsTheDocumentTheKindTheWriteAvailabilityAndTheETagValue(t *testing.T) {
	tailscale := newFakeTailscale(t, `{"acls":[]} // a comment`, `W/"abc123"`)
	fixture := startPolicyServer(t, tailscale.server.URL,
		[]config.Tailnet{{ID: "alpha"}},
		map[string]secrets.Tailnet{"alpha": tailscaleCredential()})

	code, payload := callAccess(t, fixture.client, http.MethodGet, "/api/policy/alpha", "")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusOK, payload)
	}
	var got wirePolicy
	decodePolicy(t, payload, &got)
	if got.Document != `{"acls":[]} // a comment` {
		t.Errorf("document = %q, want the document of the control server", got.Document)
	}
	if got.Kind != KindTailscale {
		t.Errorf("kind = %q, want %q", got.Kind, KindTailscale)
	}
	if !got.WriteAvailable {
		t.Error("write_available = false, want true")
	}
	if got.ETag != `W/"abc123"` {
		t.Errorf("etag = %q, want %q", got.ETag, `W/"abc123"`)
	}
	assertNoCredentialValue(t, payload)
}

func TestGetOnePolicyReturnsTheHeadscaleDocument(t *testing.T) {
	headscale := newFakeHeadscale(t, `{"acls":[]}`)
	fixture := startPolicyServer(t, "http://127.0.0.1:1",
		[]config.Tailnet{{ID: "beta", ControlURL: headscale.server.URL}},
		map[string]secrets.Tailnet{"beta": headscaleCredential(headscale.server.URL)})

	code, payload := callAccess(t, fixture.client, http.MethodGet, "/api/policy/beta", "")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusOK, payload)
	}
	var got wirePolicy
	decodePolicy(t, payload, &got)
	if got.Document != `{"acls":[]}` {
		t.Errorf("document = %q, want the document of the control server", got.Document)
	}
	if got.Kind != KindHeadscale {
		t.Errorf("kind = %q, want %q", got.Kind, KindHeadscale)
	}
	if got.ETag != "" {
		t.Errorf("etag = %q, want an empty value for a Headscale tailnet", got.ETag)
	}
}

// --- POST /api/policy/{id}/validate ---

func TestValidateReturnsTheResultOfTheControlServer(t *testing.T) {
	tailscale := newFakeTailscale(t, "{}", `W/"1"`)
	fixture := startPolicyServer(t, tailscale.server.URL,
		[]config.Tailnet{{ID: "alpha"}},
		map[string]secrets.Tailnet{"alpha": tailscaleCredential()})

	code, payload := callAccess(t, fixture.client, http.MethodPost, "/api/policy/alpha/validate", `{"document":"{}"}`)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusOK, payload)
	}
	var got wireValidate
	decodePolicy(t, payload, &got)
	if !got.Passed {
		t.Errorf("passed = false, want true; body %s", payload)
	}

	tailscale.mu.Lock()
	tailscale.validateResult = `{"message":"line 3: unknown field \"acl\""}`
	tailscale.mu.Unlock()

	code, payload = callAccess(t, fixture.client, http.MethodPost, "/api/policy/alpha/validate", `{"document":"{}"}`)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusOK, payload)
	}
	decodePolicy(t, payload, &got)
	if got.Passed {
		t.Error("passed = true, want false")
	}
	if !strings.Contains(got.Result, "line 3") {
		t.Errorf("result = %q, want the answer of the control server verbatim", got.Result)
	}
}

// --- PUT /api/policy/{id} ---

func TestPutOnePolicySendsIfMatchOnATailscaleWrite(t *testing.T) {
	tailscale := newFakeTailscale(t, "{}", `W/"abc123"`)
	fixture := startPolicyServer(t, tailscale.server.URL,
		[]config.Tailnet{{ID: "alpha"}},
		map[string]secrets.Tailnet{"alpha": tailscaleCredential()})

	code, payload := callAccess(t, fixture.client, http.MethodPut, "/api/policy/alpha",
		`{"document":"{\"acls\":[]}","etag":"W/\"abc123\""}`)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusOK, payload)
	}

	tailscale.mu.Lock()
	sent := tailscale.ifMatch
	tailscale.mu.Unlock()
	if sent != `W/"abc123"` {
		t.Errorf("If-Match = %q, want %q", sent, `W/"abc123"`)
	}

	var got wirePolicy
	decodePolicy(t, payload, &got)
	if got.ETag != `W/"abc123"-written` {
		t.Errorf("etag = %q, want the value of the write answer", got.ETag)
	}
}

func TestPutOnePolicyReturnsHTTP409WhenTheETagValueIsStale(t *testing.T) {
	tailscale := newFakeTailscale(t, "{}", `W/"abc123"`)
	tailscale.mu.Lock()
	tailscale.writeStatus = http.StatusPreconditionFailed
	tailscale.mu.Unlock()
	fixture := startPolicyServer(t, tailscale.server.URL,
		[]config.Tailnet{{ID: "alpha"}},
		map[string]secrets.Tailnet{"alpha": tailscaleCredential()})

	code, payload := callAccess(t, fixture.client, http.MethodPut, "/api/policy/alpha",
		`{"document":"{\"acls\":[]}","etag":"W/\"stale\""}`)
	if code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusConflict, payload)
	}
	var got wireError
	decodePolicy(t, payload, &got)
	if !strings.Contains(got.Error, "the policy changed since the read") {
		t.Errorf("the message %q does not state that the policy changed", got.Error)
	}
}

func TestPutOnePolicyRefusesADocumentThatValidateRejected(t *testing.T) {
	tailscale := newFakeTailscale(t, "{}", `W/"1"`)
	tailscale.mu.Lock()
	tailscale.validateResult = `{"message":"line 3: unknown field \"acl\""}`
	tailscale.mu.Unlock()
	fixture := startPolicyServer(t, tailscale.server.URL,
		[]config.Tailnet{{ID: "alpha"}},
		map[string]secrets.Tailnet{"alpha": tailscaleCredential()})

	code, payload := callAccess(t, fixture.client, http.MethodPut, "/api/policy/alpha", `{"document":"{\"acl\":[]}"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusBadRequest, payload)
	}
	var got wireError
	decodePolicy(t, payload, &got)
	if !strings.Contains(got.Error, "line 3") {
		t.Errorf("the message %q does not hold the answer of the control server", got.Error)
	}
	if tailscale.sent("POST /tailnet/-/acl") {
		t.Error("the daemon wrote the document that validate rejected")
	}
}

func TestPutOnePolicyRefusesAHeadscaleDocumentThatCheckRejected(t *testing.T) {
	headscale := newFakeHeadscale(t, "{}")
	headscale.mu.Lock()
	headscale.checkError = "policy is invalid: line 7"
	headscale.mu.Unlock()
	fixture := startPolicyServer(t, "http://127.0.0.1:1",
		[]config.Tailnet{{ID: "beta", ControlURL: headscale.server.URL}},
		map[string]secrets.Tailnet{"beta": headscaleCredential(headscale.server.URL)})

	code, payload := callAccess(t, fixture.client, http.MethodPut, "/api/policy/beta", `{"document":"{}"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusBadRequest, payload)
	}
	headscale.mu.Lock()
	written := headscale.writeCalled
	headscale.mu.Unlock()
	if written {
		t.Error("the daemon wrote the document that the check route rejected")
	}
}

func TestPutOnePolicyRecordsThePolicyPushedEvent(t *testing.T) {
	tailscale := newFakeTailscale(t, "{}", `W/"1"`)
	fixture := startPolicyServer(t, tailscale.server.URL,
		[]config.Tailnet{{ID: "alpha"}},
		map[string]secrets.Tailnet{"alpha": tailscaleCredential()})

	code, payload := callAccess(t, fixture.client, http.MethodPut, "/api/policy/alpha", `{"document":"{\"acls\":[]}"}`)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusOK, payload)
	}

	var found *reconciler.Event
	for _, event := range fixture.reconciler.Events() {
		if event.Type == EventPolicyPushed {
			e := event
			found = &e
		}
	}
	if found == nil {
		t.Fatalf("no %s event; events %+v", EventPolicyPushed, fixture.reconciler.Events())
	}
	if found.TailnetID != "alpha" {
		t.Errorf("the tailnet of the event = %q, want %q", found.TailnetID, "alpha")
	}
	if !strings.Contains(found.Message, KindTailscale) {
		t.Errorf("the message of the event = %q, want the control server kind", found.Message)
	}
}

func TestPutOnePolicyReturnsHTTP409WhenTheHeadscaleControlServerRunsTheFilePolicyMode(t *testing.T) {
	headscale := newFakeHeadscale(t, "{}")
	headscale.mu.Lock()
	headscale.writeError = "update is disabled for modes other than 'database'"
	headscale.mu.Unlock()
	fixture := startPolicyServer(t, "http://127.0.0.1:1",
		[]config.Tailnet{{ID: "beta", ControlURL: headscale.server.URL}},
		map[string]secrets.Tailnet{"beta": headscaleCredential(headscale.server.URL)})

	code, payload := callAccess(t, fixture.client, http.MethodPut, "/api/policy/beta", `{"document":"{}"}`)
	if code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusConflict, payload)
	}
	var got wireError
	decodePolicy(t, payload, &got)
	if !strings.Contains(got.Error, `policy.mode: "db"`) {
		t.Errorf("the message %q does not state the reason", got.Error)
	}
}

// --- PUT /api/policy/{id}/credentials ---

func TestPutCredentialsWritesTheSecretsFileAndReturnsNoCredentialValue(t *testing.T) {
	fixture := startPolicyServer(t, "http://127.0.0.1:1", []config.Tailnet{{ID: "alpha"}}, nil)

	body := fmt.Sprintf(`{"tailscale_oauth_client_id":%q,"tailscale_oauth_client_secret":%q}`, testClientID, testClientSecret)
	code, payload := callAccess(t, fixture.client, http.MethodPut, "/api/policy/alpha/credentials", body)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusOK, payload)
	}

	var got wirePolicyTailnet
	decodePolicy(t, payload, &got)
	if !got.CredentialPresent {
		t.Errorf("credential_present = false, want true; body %s", payload)
	}
	assertNoCredentialValue(t, payload)

	info, err := os.Stat(fixture.secretsPath)
	if err != nil {
		t.Fatalf("stat the secrets file: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("the mode of the secrets file = %04o, want 0600", info.Mode().Perm())
	}
	stored, err := secrets.Load(fixture.secretsPath)
	if err != nil {
		t.Fatalf("secrets.Load: %v", err)
	}
	if stored.Tailnets["alpha"].TailscaleOAuthClientSecret != testClientSecret {
		t.Errorf("the stored secret = %q, want the value of the request", stored.Tailnets["alpha"].TailscaleOAuthClientSecret)
	}
}

func TestPutCredentialsRefusesACredentialOfTheOtherControlServerKind(t *testing.T) {
	headscale := newFakeHeadscale(t, "{}")
	fixture := startPolicyServer(t, "http://127.0.0.1:1",
		[]config.Tailnet{{ID: "alpha"}, {ID: "beta", ControlURL: headscale.server.URL}}, nil)

	cases := []struct {
		name string
		id   string
		body string
	}{
		{"a Tailscale tailnet takes no Headscale API key", "alpha", `{"headscale_api_key":"x","headscale_address":"https://h.example.com"}`},
		{"a Headscale tailnet takes no OAuth client", "beta", `{"tailscale_oauth_client_id":"x","tailscale_oauth_client_secret":"y"}`},
		{"a Tailscale tailnet needs the client secret", "alpha", `{"tailscale_oauth_client_id":"x"}`},
		{"a Headscale tailnet needs the address", "beta", `{"headscale_api_key":"x"}`},
		{"a Headscale address is a URL", "beta", `{"headscale_api_key":"x","headscale_address":"not-a-url"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, payload := callAccess(t, fixture.client, http.MethodPut, "/api/policy/"+tc.id+"/credentials", tc.body)
			if code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body %s", code, http.StatusBadRequest, payload)
			}
			var got wireError
			decodePolicy(t, payload, &got)
			if got.Error == "" {
				t.Errorf("the body %s holds no error message", payload)
			}
			if _, err := os.Stat(fixture.secretsPath); !os.IsNotExist(err) {
				t.Errorf("the route wrote the secrets file for a request that it refused")
			}
		})
	}
}

// --- The rules that every route keeps ---

func TestEveryPolicyRouteRefusesABadRequestBodyBeforeItActs(t *testing.T) {
	tailscale := newFakeTailscale(t, "{}", `W/"1"`)
	headscale := newFakeHeadscale(t, "{}")
	fixture := startPolicyServer(t, tailscale.server.URL,
		[]config.Tailnet{{ID: "alpha"}, {ID: "beta", ControlURL: headscale.server.URL}},
		map[string]secrets.Tailnet{
			"alpha": tailscaleCredential(),
			"beta":  headscaleCredential(headscale.server.URL),
		})

	cases := []struct {
		name   string
		method string
		target string
		body   string
	}{
		{"the write route rejects a body that is not JSON", http.MethodPut, "/api/policy/alpha", "not json"},
		{"the write route rejects an empty document", http.MethodPut, "/api/policy/alpha", `{"document":""}`},
		{"the write route rejects an ETag value for a Headscale tailnet", http.MethodPut, "/api/policy/beta", `{"document":"{}","etag":"W/\"1\""}`},
		{"the validate route rejects a body that is not JSON", http.MethodPost, "/api/policy/alpha/validate", "not json"},
		{"the validate route rejects an empty document", http.MethodPost, "/api/policy/alpha/validate", `{"document":""}`},
		{"the credentials route rejects a body that is not JSON", http.MethodPut, "/api/policy/alpha/credentials", "not json"},
		{"the credentials route rejects an empty body", http.MethodPut, "/api/policy/alpha/credentials", `{}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, payload := callAccess(t, fixture.client, tc.method, tc.target, tc.body)
			if code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body %s", code, http.StatusBadRequest, payload)
			}
			var got wireError
			decodePolicy(t, payload, &got)
			if got.Error == "" {
				t.Errorf("the body %s holds no error message", payload)
			}
		})
	}

	if tailscale.sent("POST /tailnet/-/acl") {
		t.Error("a refused request reached the write route of the control server")
	}
	headscale.mu.Lock()
	written := headscale.writeCalled
	headscale.mu.Unlock()
	if written {
		t.Error("a refused request reached the write route of the Headscale control server")
	}
}

func TestEveryPolicyRouteRejectsARequestBodyLargerThanOneMegabyte(t *testing.T) {
	tailscale := newFakeTailscale(t, "{}", `W/"1"`)
	fixture := startPolicyServer(t, tailscale.server.URL,
		[]config.Tailnet{{ID: "alpha"}},
		map[string]secrets.Tailnet{"alpha": tailscaleCredential()})

	document := strings.Repeat("a", (1<<20)+1024)
	cases := []struct {
		name   string
		method string
		target string
		body   string
	}{
		{"the write route", http.MethodPut, "/api/policy/alpha", `{"document":"` + document + `"}`},
		{"the validate route", http.MethodPost, "/api/policy/alpha/validate", `{"document":"` + document + `"}`},
		{"the credentials route", http.MethodPut, "/api/policy/alpha/credentials", `{"tailscale_oauth_client_id":"` + document + `"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, payload := callAccess(t, fixture.client, tc.method, tc.target, tc.body)
			if code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body %s", code, http.StatusBadRequest, payload)
			}
		})
	}
}

func TestEveryPolicyRouteRefusesATailnetIdentifierThatTheDaemonDoesNotAccept(t *testing.T) {
	fixture := startPolicyServer(t, "http://127.0.0.1:1", []config.Tailnet{{ID: "alpha"}}, nil)

	// The identifier holds an escaped path separator, so the route reads one path segment
	// that names a parent directory. A route that writes a file per tailnet must refuse
	// it. See SA-15.
	cases := []struct {
		method string
		target string
		body   string
	}{
		{http.MethodGet, "/api/policy/..%2F..%2Fetc%2Fshadow", ""},
		{http.MethodPut, "/api/policy/..%2F..%2Fetc%2Fshadow", `{"document":"{}"}`},
		{http.MethodPost, "/api/policy/..%2F..%2Fetc%2Fshadow/validate", `{"document":"{}"}`},
		{http.MethodPut, "/api/policy/..%2F..%2Fetc%2Fshadow/credentials", `{"tailscale_oauth_client_id":"x","tailscale_oauth_client_secret":"y"}`},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.target, func(t *testing.T) {
			code, payload := callAccess(t, fixture.client, tc.method, tc.target, tc.body)
			if code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body %s", code, http.StatusBadRequest, payload)
			}
		})
	}

	entries, err := os.ReadDir(fixture.dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "config.yaml" {
		t.Errorf("the directory holds %v, want the configuration file alone", entries)
	}
}

func TestNoPolicyResponseAndNoLogLineHoldsACredentialValue(t *testing.T) {
	var logged bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&logged)
	defer log.SetOutput(previous)

	tailscale := newFakeTailscale(t, `{"acls":[]}`, `W/"1"`)
	fixture := startPolicyServer(t, tailscale.server.URL, []config.Tailnet{{ID: "alpha"}}, nil)

	body := fmt.Sprintf(`{"tailscale_oauth_client_id":%q,"tailscale_oauth_client_secret":%q}`, testClientID, testClientSecret)
	calls := []struct {
		method string
		target string
		body   string
	}{
		{http.MethodPut, "/api/policy/alpha/credentials", body},
		{http.MethodGet, "/api/policy", ""},
		{http.MethodGet, "/api/policy/alpha", ""},
		{http.MethodPost, "/api/policy/alpha/validate", `{"document":"{}"}`},
		{http.MethodPut, "/api/policy/alpha", `{"document":"{\"acls\":[]}"}`},
	}
	for _, call := range calls {
		code, payload := callAccess(t, fixture.client, call.method, call.target, call.body)
		if code != http.StatusOK {
			t.Fatalf("%s %s: status = %d, want %d; body %s", call.method, call.target, code, http.StatusOK, payload)
		}
		assertNoCredentialValue(t, payload)
	}

	for _, value := range []string{testClientSecret, testAccessToken, testAPIKey} {
		if strings.Contains(logged.String(), value) {
			t.Errorf("the log holds the credential value %q", value)
		}
	}
}

// assertNoCredentialValue fails the test when payload holds a credential value.
func assertNoCredentialValue(t *testing.T, payload []byte) {
	t.Helper()
	for _, value := range []string{testClientID, testClientSecret, testAPIKey, testAccessToken} {
		if bytes.Contains(payload, []byte(value)) {
			t.Errorf("the response body %s holds the credential value %q", payload, value)
		}
	}
}
