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
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"hydrascale/internal/config"
	"hydrascale/internal/policy"
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
	CredentialState   string `json:"credential_state"`
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

// The test file declares the response shape of the two sections routes rather than
// reading it from the package, matching the wire structs above.

type wireACLRule struct {
	Action string   `json:"action"`
	Src    []string `json:"src"`
	Dst    []string `json:"dst"`
}

type wireSections struct {
	Groups      map[string][]string `json:"groups"`
	Hosts       map[string]string   `json:"hosts"`
	TagOwners   map[string][]string `json:"tagOwners"`
	IPSets      map[string][]string `json:"ipsets"`
	ACLs        []wireACLRule       `json:"acls"`
	Grants      []json.RawMessage   `json:"grants"`
	SSH         []json.RawMessage   `json:"ssh"`
	NodeAttrs   []json.RawMessage   `json:"nodeAttrs"`
	Postures    map[string][]string `json:"postures"`
	Tests       []json.RawMessage   `json:"tests"`
	SSHTests    []json.RawMessage   `json:"sshTests"`
	OpaqueKeys  []string            `json:"opaque_keys"`
	SectionKeys []string            `json:"section_keys"`
}

type wireSectionsEdit struct {
	Document string `json:"document"`
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
	validateStatus int
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
		if f.validateStatus != 0 {
			w.WriteHeader(f.validateStatus)
			fmt.Fprint(w, `{"message":"API token invalid"}`)
			return
		}
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

// --- POST /api/policy/{id}/sections ---

func TestPolicySectionsReturnsEveryNamedSectionAndTheOpaqueKeys(t *testing.T) {
	fixture := startPolicyServer(t, "http://127.0.0.1:1", []config.Tailnet{{ID: "alpha"}}, nil)

	document := `{
  "groups": {"group:admins": ["alice@example.com"]},
  "hosts": {"server": "100.64.0.1"},
  "acls": [{"action": "accept", "src": ["group:admins"], "dst": ["*:*"]}],
  "randomizeClientPort": true,
}
`
	req, err := json.Marshal(map[string]string{"document": document})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	code, payload := callAccess(t, fixture.client, http.MethodPost, "/api/policy/alpha/sections", string(req))
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusOK, payload)
	}
	var got wireSections
	decodePolicy(t, payload, &got)
	if len(got.Groups["group:admins"]) != 1 {
		t.Errorf("Groups = %v, want one member of group:admins", got.Groups)
	}
	if got.Hosts["server"] != "100.64.0.1" {
		t.Errorf("Hosts = %v, want server = 100.64.0.1", got.Hosts)
	}
	if len(got.ACLs) != 1 || got.ACLs[0].Action != "accept" {
		t.Errorf("ACLs = %v, want one accept entry", got.ACLs)
	}
	if len(got.OpaqueKeys) != 1 || got.OpaqueKeys[0] != "randomizeClientPort" {
		t.Errorf("OpaqueKeys = %v, want [randomizeClientPort]", got.OpaqueKeys)
	}
}

func TestPolicySectionsReturnsHTTP400WithTheLineAndTheColumnOnAParseFailure(t *testing.T) {
	fixture := startPolicyServer(t, "http://127.0.0.1:1", []config.Tailnet{{ID: "alpha"}}, nil)

	req, err := json.Marshal(map[string]string{"document": "{\n  \"groups\": {\n"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	code, payload := callAccess(t, fixture.client, http.MethodPost, "/api/policy/alpha/sections", string(req))
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusBadRequest, payload)
	}
	var got wireError
	decodePolicy(t, payload, &got)
	if !strings.Contains(got.Error, "line") || !strings.Contains(got.Error, "column") {
		t.Errorf("Error = %q, want a message naming the line and the column", got.Error)
	}
}

func TestPolicySectionsAnAbsentSectionReturnsAnEmptyListNotNull(t *testing.T) {
	fixture := startPolicyServer(t, "http://127.0.0.1:1", []config.Tailnet{{ID: "alpha"}}, nil)

	code, payload := callAccess(t, fixture.client, http.MethodPost, "/api/policy/alpha/sections", `{"document":"{}"}`)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusOK, payload)
	}
	for _, want := range []string{`"groups":{}`, `"acls":[]`, `"opaque_keys":[]`} {
		if !strings.Contains(string(payload), want) {
			t.Errorf("body %s does not hold %s", payload, want)
		}
	}
}

// FR-vadv-11 disables Push while the document holds a postures key. The response must
// separate an empty postures key from an absent one. Both decode into the same empty map,
// therefore SectionKeys carries the signal.
func TestPolicySectionsNamesEveryNamedSectionKeyTheDocumentHolds(t *testing.T) {
	fixture := startPolicyServer(t, "http://127.0.0.1:1", []config.Tailnet{{ID: "alpha"}}, nil)

	cases := []struct {
		name     string
		document string
		want     bool
	}{
		{name: "an empty postures key", document: `{"postures": {}}`, want: true},
		{name: "a postures key with one entry", document: `{"postures": {"posture:latest": ["node:os == 'linux'"]}}`, want: true},
		{name: "no postures key", document: `{"groups": {}}`, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := json.Marshal(map[string]string{"document": tc.document})
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			code, payload := callAccess(t, fixture.client, http.MethodPost, "/api/policy/alpha/sections", string(req))
			if code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body %s", code, http.StatusOK, payload)
			}
			var got wireSections
			decodePolicy(t, payload, &got)
			if slices.Contains(got.SectionKeys, "postures") != tc.want {
				t.Errorf("SectionKeys = %v, want it to hold %q = %v", got.SectionKeys, "postures", tc.want)
			}
		})
	}
}

// --- POST /api/policy/{id}/sections/edit ---

func TestPolicySectionsEditAddsAnEntryAndKeepsEveryOtherByte(t *testing.T) {
	fixture := startPolicyServer(t, "http://127.0.0.1:1", []config.Tailnet{{ID: "alpha"}}, nil)

	document := `{
  "acls": [
    {"action": "accept", "src": ["group:admins"], "dst": ["*:*"]},
  ],
}
`
	req, err := json.Marshal(map[string]interface{}{
		"document": document,
		"section":  "acls",
		"op":       "add",
		"entry":    map[string]interface{}{"action": "accept", "src": []string{"tag:laptop"}, "dst": []string{"tag:server:*"}},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	code, payload := callAccess(t, fixture.client, http.MethodPost, "/api/policy/alpha/sections/edit", string(req))
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusOK, payload)
	}
	var got wireSectionsEdit
	decodePolicy(t, payload, &got)
	if !strings.Contains(got.Document, "tag:laptop") {
		t.Errorf("Document = %q, want it to hold the new entry", got.Document)
	}
	if !strings.Contains(got.Document, `{"action": "accept", "src": ["group:admins"], "dst": ["*:*"]},`) {
		t.Errorf("Document = %q, want the existing entry byte-for-byte unchanged", got.Document)
	}
}

func TestPolicySectionsEditReplacesAndRemovesAnEntryByIndex(t *testing.T) {
	fixture := startPolicyServer(t, "http://127.0.0.1:1", []config.Tailnet{{ID: "alpha"}}, nil)

	document := `{
  "acls": [
    {"action": "accept", "src": ["a"], "dst": ["b"]},
    {"action": "accept", "src": ["c"], "dst": ["d"]},
  ],
}
`
	index := 1
	req, err := json.Marshal(map[string]interface{}{
		"document": document,
		"section":  "acls",
		"op":       "replace",
		"index":    &index,
		"entry":    map[string]interface{}{"action": "accept", "src": []string{"e"}, "dst": []string{"f"}},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	code, payload := callAccess(t, fixture.client, http.MethodPost, "/api/policy/alpha/sections/edit", string(req))
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusOK, payload)
	}
	var replaced wireSectionsEdit
	decodePolicy(t, payload, &replaced)
	if !strings.Contains(replaced.Document, `"e"`) || strings.Contains(replaced.Document, `"src": ["c"]`) {
		t.Errorf("Document = %q, want the second entry replaced", replaced.Document)
	}
	if !strings.Contains(replaced.Document, `"src": ["a"]`) {
		t.Errorf("Document = %q, want the first entry unchanged", replaced.Document)
	}

	req, err = json.Marshal(map[string]interface{}{
		"document": replaced.Document,
		"section":  "acls",
		"op":       "remove",
		"index":    &index,
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	code, payload = callAccess(t, fixture.client, http.MethodPost, "/api/policy/alpha/sections/edit", string(req))
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusOK, payload)
	}
	var removed wireSectionsEdit
	decodePolicy(t, payload, &removed)
	if strings.Contains(removed.Document, `"e"`) {
		t.Errorf("Document = %q, want the replaced entry removed", removed.Document)
	}
	if !strings.Contains(removed.Document, `"src": ["a"]`) {
		t.Errorf("Document = %q, want the first entry unchanged", removed.Document)
	}
}

func TestPolicySectionsEditAddsRenamesAndRemovesAMapEntryByKey(t *testing.T) {
	fixture := startPolicyServer(t, "http://127.0.0.1:1", []config.Tailnet{{ID: "alpha"}}, nil)

	document := `{
  "groups": {
    "group:admins": ["alice@example.com"],
  },
}
`
	req, err := json.Marshal(map[string]interface{}{
		"document": document,
		"section":  "groups",
		"op":       "add",
		"key":      "group:eng",
		"entry":    []string{"carol@example.com"},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	code, payload := callAccess(t, fixture.client, http.MethodPost, "/api/policy/alpha/sections/edit", string(req))
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusOK, payload)
	}
	var added wireSectionsEdit
	decodePolicy(t, payload, &added)
	if !strings.Contains(added.Document, "group:eng") || !strings.Contains(added.Document, "carol@example.com") {
		t.Errorf("Document = %q, want it to hold the new key", added.Document)
	}
	if !strings.Contains(added.Document, "group:admins") {
		t.Errorf("Document = %q, want the existing key unchanged", added.Document)
	}

	req, err = json.Marshal(map[string]interface{}{
		"document": added.Document,
		"section":  "groups",
		"op":       "rename",
		"key":      "group:eng",
		"new_key":  "group:owners",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	code, payload = callAccess(t, fixture.client, http.MethodPost, "/api/policy/alpha/sections/edit", string(req))
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusOK, payload)
	}
	var renamed wireSectionsEdit
	decodePolicy(t, payload, &renamed)
	if strings.Contains(renamed.Document, "group:eng") || !strings.Contains(renamed.Document, "group:owners") {
		t.Errorf("Document = %q, want group:eng renamed to group:owners", renamed.Document)
	}
	if !strings.Contains(renamed.Document, "carol@example.com") {
		t.Errorf("Document = %q, want the renamed key's members unchanged", renamed.Document)
	}

	req, err = json.Marshal(map[string]interface{}{
		"document": renamed.Document,
		"section":  "groups",
		"op":       "remove",
		"key":      "group:owners",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	code, payload = callAccess(t, fixture.client, http.MethodPost, "/api/policy/alpha/sections/edit", string(req))
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusOK, payload)
	}
	var removed wireSectionsEdit
	decodePolicy(t, payload, &removed)
	if strings.Contains(removed.Document, "group:owners") {
		t.Errorf("Document = %q, want group:owners removed", removed.Document)
	}
	if !strings.Contains(removed.Document, "group:admins") {
		t.Errorf("Document = %q, want group:admins unchanged", removed.Document)
	}
}

func TestPolicySectionsEditReplacesAMapEntryValueByKey(t *testing.T) {
	fixture := startPolicyServer(t, "http://127.0.0.1:1", []config.Tailnet{{ID: "alpha"}}, nil)

	document := `{"groups": {"group:admins": ["alice@example.com"]}}`
	req, err := json.Marshal(map[string]interface{}{
		"document": document,
		"section":  "groups",
		"op":       "replace",
		"key":      "group:admins",
		"entry":    []string{"alice@example.com", "bob@example.com"},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	code, payload := callAccess(t, fixture.client, http.MethodPost, "/api/policy/alpha/sections/edit", string(req))
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusOK, payload)
	}
	var got wireSectionsEdit
	decodePolicy(t, payload, &got)
	if !strings.Contains(got.Document, "bob@example.com") {
		t.Errorf("Document = %q, want the new member added", got.Document)
	}
}

// The postures section is map-shaped, per features/13-visual-policy-advanced.md
// FR-vadv-10. Issue #345 records the defect: the route wrote an array, the key reached
// the document never, and the sections route then refused the result.
func TestPolicySectionsEditAddsAPostureAsAMapEntryAndReadsItBack(t *testing.T) {
	fixture := startPolicyServer(t, "http://127.0.0.1:1", []config.Tailnet{{ID: "alpha"}}, nil)

	req, err := json.Marshal(map[string]interface{}{
		"document": "{}",
		"section":  "postures",
		"op":       "add",
		"key":      "posture:test",
		"entry":    []string{"node:os == 'macos'"},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	code, payload := callAccess(t, fixture.client, http.MethodPost, "/api/policy/alpha/sections/edit", string(req))
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusOK, payload)
	}
	var added wireSectionsEdit
	decodePolicy(t, payload, &added)
	if !strings.Contains(added.Document, "posture:test") {
		t.Errorf("Document = %q, want it to hold the posture name", added.Document)
	}

	req, err = json.Marshal(map[string]interface{}{"document": added.Document})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	code, payload = callAccess(t, fixture.client, http.MethodPost, "/api/policy/alpha/sections", string(req))
	if code != http.StatusOK {
		t.Fatalf("sections status = %d, want %d; body %s", code, http.StatusOK, payload)
	}
	var sections wireSections
	decodePolicy(t, payload, &sections)
	got := sections.Postures["posture:test"]
	if len(got) != 1 || got[0] != "node:os == 'macos'" {
		t.Errorf("Postures = %#v, want the key %q with one expression", sections.Postures, "posture:test")
	}
}

func TestPolicySectionsEditReplacesRenamesAndRemovesAPostureByKey(t *testing.T) {
	fixture := startPolicyServer(t, "http://127.0.0.1:1", []config.Tailnet{{ID: "alpha"}}, nil)

	document := `{
  "postures": {
    "posture:latestMac": ["node:os == 'macos'"],
  },
}
`
	req, err := json.Marshal(map[string]interface{}{
		"document": document,
		"section":  "postures",
		"op":       "replace",
		"key":      "posture:latestMac",
		"entry":    []string{"node:os == 'macos'", "node:os == 'linux'"},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	code, payload := callAccess(t, fixture.client, http.MethodPost, "/api/policy/alpha/sections/edit", string(req))
	if code != http.StatusOK {
		t.Fatalf("replace status = %d, want %d; body %s", code, http.StatusOK, payload)
	}
	var replaced wireSectionsEdit
	decodePolicy(t, payload, &replaced)
	if !strings.Contains(replaced.Document, "node:os == 'linux'") {
		t.Errorf("Document = %q, want the new expression", replaced.Document)
	}

	req, err = json.Marshal(map[string]interface{}{
		"document": replaced.Document,
		"section":  "postures",
		"op":       "rename",
		"key":      "posture:latestMac",
		"new_key":  "posture:currentMac",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	code, payload = callAccess(t, fixture.client, http.MethodPost, "/api/policy/alpha/sections/edit", string(req))
	if code != http.StatusOK {
		t.Fatalf("rename status = %d, want %d; body %s", code, http.StatusOK, payload)
	}
	var renamed wireSectionsEdit
	decodePolicy(t, payload, &renamed)
	if strings.Contains(renamed.Document, "posture:latestMac") || !strings.Contains(renamed.Document, "posture:currentMac") {
		t.Errorf("Document = %q, want posture:latestMac renamed to posture:currentMac", renamed.Document)
	}

	// The console sends a remove with the key alone. Issue #345 records the refusal
	// "index is required for op \"remove\"" that the array-shaped branch returned.
	req, err = json.Marshal(map[string]interface{}{
		"document": renamed.Document,
		"section":  "postures",
		"op":       "remove",
		"key":      "posture:currentMac",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	code, payload = callAccess(t, fixture.client, http.MethodPost, "/api/policy/alpha/sections/edit", string(req))
	if code != http.StatusOK {
		t.Fatalf("remove status = %d, want %d; body %s", code, http.StatusOK, payload)
	}
	var removed wireSectionsEdit
	decodePolicy(t, payload, &removed)
	if strings.Contains(removed.Document, "posture:currentMac") {
		t.Errorf("Document = %q, want posture:currentMac removed", removed.Document)
	}
}

func TestPolicySectionsEditAddsReplacesAndRemovesAnAutoApproverRouteByCIDR(t *testing.T) {
	fixture := startPolicyServer(t, "http://127.0.0.1:1", []config.Tailnet{{ID: "alpha"}}, nil)

	document := `{
  "autoApprovers": {
    "routes": {
      "10.0.0.0/24": ["tag:router"],
    },
  },
}
`
	req, err := json.Marshal(map[string]interface{}{
		"document": document,
		"section":  "autoApprovers.routes",
		"op":       "add",
		"key":      "10.0.1.0/24",
		"entry":    []string{"tag:router"},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	code, payload := callAccess(t, fixture.client, http.MethodPost, "/api/policy/alpha/sections/edit", string(req))
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusOK, payload)
	}
	var added wireSectionsEdit
	decodePolicy(t, payload, &added)
	if !strings.Contains(added.Document, "10.0.1.0/24") {
		t.Errorf("Document = %q, want the new route added", added.Document)
	}
	if !strings.Contains(added.Document, "10.0.0.0/24") {
		t.Errorf("Document = %q, want the existing route unchanged", added.Document)
	}

	req, err = json.Marshal(map[string]interface{}{
		"document": added.Document,
		"section":  "autoApprovers.routes",
		"op":       "replace",
		"key":      "10.0.1.0/24",
		"entry":    []string{"tag:router", "tag:backup"},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	code, payload = callAccess(t, fixture.client, http.MethodPost, "/api/policy/alpha/sections/edit", string(req))
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusOK, payload)
	}
	var replaced wireSectionsEdit
	decodePolicy(t, payload, &replaced)
	if !strings.Contains(replaced.Document, "tag:backup") {
		t.Errorf("Document = %q, want tag:backup added to the route", replaced.Document)
	}

	req, err = json.Marshal(map[string]interface{}{
		"document": replaced.Document,
		"section":  "autoApprovers.routes",
		"op":       "remove",
		"key":      "10.0.0.0/24",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	code, payload = callAccess(t, fixture.client, http.MethodPost, "/api/policy/alpha/sections/edit", string(req))
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusOK, payload)
	}
	var removed wireSectionsEdit
	decodePolicy(t, payload, &removed)
	if strings.Contains(removed.Document, "10.0.0.0/24") {
		t.Errorf("Document = %q, want 10.0.0.0/24 removed", removed.Document)
	}
	if !strings.Contains(removed.Document, "10.0.1.0/24") {
		t.Errorf("Document = %q, want 10.0.1.0/24 unchanged", removed.Document)
	}
}

func TestPolicySectionsEditReplacesTheWholeAutoApproverExitNodeList(t *testing.T) {
	fixture := startPolicyServer(t, "http://127.0.0.1:1", []config.Tailnet{{ID: "alpha"}}, nil)

	document := `{"autoApprovers": {"exitNode": ["tag:exit"]}}`
	req, err := json.Marshal(map[string]interface{}{
		"document": document,
		"section":  "autoApprovers.exitNode",
		"op":       "replace",
		"entry":    []string{"tag:exit", "tag:backup-exit"},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	code, payload := callAccess(t, fixture.client, http.MethodPost, "/api/policy/alpha/sections/edit", string(req))
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusOK, payload)
	}
	var got wireSectionsEdit
	decodePolicy(t, payload, &got)
	if !strings.Contains(got.Document, "tag:backup-exit") {
		t.Errorf("Document = %q, want the new exit node approver added", got.Document)
	}
}

func TestPolicySectionsEditRejectsAnAutoApproverRouteAddWithNoKey(t *testing.T) {
	fixture := startPolicyServer(t, "http://127.0.0.1:1", []config.Tailnet{{ID: "alpha"}}, nil)

	code, payload := callAccess(t, fixture.client, http.MethodPost, "/api/policy/alpha/sections/edit",
		`{"document":"{}","section":"autoApprovers.routes","op":"add","entry":["tag:router"]}`)
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusBadRequest, payload)
	}
}

func TestPolicySectionsEditRejectsAnAutoApproverExitNodeAddOp(t *testing.T) {
	fixture := startPolicyServer(t, "http://127.0.0.1:1", []config.Tailnet{{ID: "alpha"}}, nil)

	code, payload := callAccess(t, fixture.client, http.MethodPost, "/api/policy/alpha/sections/edit",
		`{"document":"{}","section":"autoApprovers.exitNode","op":"add","entry":["tag:exit"]}`)
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusBadRequest, payload)
	}
}

func TestPolicySectionsEditRejectsABadOpBeforeAnyChange(t *testing.T) {
	fixture := startPolicyServer(t, "http://127.0.0.1:1", []config.Tailnet{{ID: "alpha"}}, nil)

	code, payload := callAccess(t, fixture.client, http.MethodPost, "/api/policy/alpha/sections/edit",
		`{"document":"{\"acls\":[]}","section":"acls","op":"bogus"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusBadRequest, payload)
	}
}

func TestPolicySectionsEditRejectsAReplaceWithNoIndex(t *testing.T) {
	fixture := startPolicyServer(t, "http://127.0.0.1:1", []config.Tailnet{{ID: "alpha"}}, nil)

	code, payload := callAccess(t, fixture.client, http.MethodPost, "/api/policy/alpha/sections/edit",
		`{"document":"{\"acls\":[]}","section":"acls","op":"replace","entry":{}}`)
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusBadRequest, payload)
	}
}

func TestPolicySectionsEditRejectsAnAddWithNoEntry(t *testing.T) {
	fixture := startPolicyServer(t, "http://127.0.0.1:1", []config.Tailnet{{ID: "alpha"}}, nil)

	code, payload := callAccess(t, fixture.client, http.MethodPost, "/api/policy/alpha/sections/edit",
		`{"document":"{\"acls\":[]}","section":"acls","op":"add"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusBadRequest, payload)
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
	if !strings.Contains(got.Error, `policy.mode: "database"`) {
		t.Errorf("the message %q does not state the reason", got.Error)
	}
}

func TestPutOnePolicyRecordsARejectionOnAValidateFailure(t *testing.T) {
	tailscale := newFakeTailscale(t, "{}", `W/"1"`)
	tailscale.mu.Lock()
	tailscale.validateStatus = http.StatusUnauthorized
	tailscale.mu.Unlock()
	fixture := startPolicyServer(t, tailscale.server.URL,
		[]config.Tailnet{{ID: "alpha"}},
		map[string]secrets.Tailnet{"alpha": tailscaleCredential()})

	code, _ := callAccess(t, fixture.client, http.MethodPut, "/api/policy/alpha", `{"document":"{\"acls\":[]}"}`)
	if code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", code, http.StatusBadGateway)
	}

	code, payload := callAccess(t, fixture.client, http.MethodGet, "/api/policy", "")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusOK, payload)
	}
	var got wirePolicyList
	decodePolicy(t, payload, &got)
	if got.Tailnets[0].CredentialState != CredentialRejected {
		t.Errorf("credential_state = %q, want %q; body %s", got.Tailnets[0].CredentialState, CredentialRejected, payload)
	}
}

func TestPutOnePolicyRecordsARejectionOnAWriteFailure(t *testing.T) {
	tailscale := newFakeTailscale(t, "{}", `W/"1"`)
	tailscale.mu.Lock()
	tailscale.writeStatus = http.StatusUnauthorized
	tailscale.mu.Unlock()
	fixture := startPolicyServer(t, tailscale.server.URL,
		[]config.Tailnet{{ID: "alpha"}},
		map[string]secrets.Tailnet{"alpha": tailscaleCredential()})

	code, _ := callAccess(t, fixture.client, http.MethodPut, "/api/policy/alpha", `{"document":"{\"acls\":[]}"}`)
	if code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", code, http.StatusBadGateway)
	}

	code, payload := callAccess(t, fixture.client, http.MethodGet, "/api/policy", "")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %s", code, http.StatusOK, payload)
	}
	var got wirePolicyList
	decodePolicy(t, payload, &got)
	if got.Tailnets[0].CredentialState != CredentialRejected {
		t.Errorf("credential_state = %q, want %q; body %s", got.Tailnets[0].CredentialState, CredentialRejected, payload)
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
		{"the sections route rejects a body that is not JSON", http.MethodPost, "/api/policy/alpha/sections", "not json"},
		{"the sections route rejects an empty document", http.MethodPost, "/api/policy/alpha/sections", `{"document":""}`},
		{"the sections route rejects a document that does not parse", http.MethodPost, "/api/policy/alpha/sections", `{"document":"{"}`},
		{"the sections edit route rejects a body that is not JSON", http.MethodPost, "/api/policy/alpha/sections/edit", "not json"},
		{"the sections edit route rejects an empty document", http.MethodPost, "/api/policy/alpha/sections/edit", `{"document":"","section":"acls","op":"add","entry":{}}`},
		{"the sections edit route rejects an empty section", http.MethodPost, "/api/policy/alpha/sections/edit", `{"document":"{}","section":"","op":"add","entry":{}}`},
		{"the sections edit route rejects an unknown op", http.MethodPost, "/api/policy/alpha/sections/edit", `{"document":"{}","section":"acls","op":"bogus"}`},
		{"the sections edit route rejects a map add with no key", http.MethodPost, "/api/policy/alpha/sections/edit", `{"document":"{}","section":"groups","op":"add","entry":[]}`},
		{"the sections edit route rejects a map remove with no key", http.MethodPost, "/api/policy/alpha/sections/edit", `{"document":"{}","section":"groups","op":"remove"}`},
		{"the sections edit route rejects a map rename with no new_key", http.MethodPost, "/api/policy/alpha/sections/edit", `{"document":"{}","section":"groups","op":"rename","key":"group:admins"}`},
		{"the sections edit route rejects a map section with an unknown op", http.MethodPost, "/api/policy/alpha/sections/edit", `{"document":"{}","section":"groups","op":"bogus"}`},
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
		{"the sections route", http.MethodPost, "/api/policy/alpha/sections", `{"document":"` + document + `"}`},
		{"the sections edit route", http.MethodPost, "/api/policy/alpha/sections/edit", `{"document":"` + document + `"}`},
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
		{http.MethodPost, "/api/policy/..%2F..%2Fetc%2Fshadow/sections", `{"document":"{}"}`},
		{http.MethodPost, "/api/policy/..%2F..%2Fetc%2Fshadow/sections/edit", `{"document":"{}","section":"acls","op":"add","entry":{}}`},
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

func TestThePolicyRowStatesARejectedCredentialForADeviceAuthenticationKey(t *testing.T) {
	// Issue #276. An operator wrote such a key on 2026-08-13. The row read `read and write`
	// and the control server answered the token request with HTTP 401.
	cred := secrets.Tailnet{
		TailscaleOAuthClientID:     "khjuWjCNTRL",
		TailscaleOAuthClientSecret: "tskey-auth-khjuWjCNTRL-abcdefghijklmnop",
	}

	row := policyRow("jbones", KindTailscale, cred, "")

	if row.CredentialState != CredentialRejected {
		t.Errorf("CredentialState = %q, want %q", row.CredentialState, CredentialRejected)
	}
	if !row.CredentialPresent {
		t.Error("CredentialPresent is false, and the tailnet holds a credential")
	}
	if row.WriteAvailable {
		t.Error("WriteAvailable is true, and the control server takes no request with this credential")
	}
	if !strings.Contains(row.Reason, "device authentication key") {
		t.Errorf("Reason = %q, and it names no device authentication key", row.Reason)
	}
}

func TestThePolicyRowStatesAUsableCredentialForAnOAuthClientSecret(t *testing.T) {
	cred := secrets.Tailnet{
		TailscaleOAuthClientID:     "khjuWjCNTRL",
		TailscaleOAuthClientSecret: "tskey-client-khjuWjCNTRL-abcdefghijklmnop",
	}

	row := policyRow("jbones", KindTailscale, cred, "")

	if row.CredentialState != CredentialUsable {
		t.Errorf("CredentialState = %q, want %q", row.CredentialState, CredentialUsable)
	}
	if !row.WriteAvailable {
		t.Error("WriteAvailable is false for a credential of the right shape")
	}
}

func TestThePolicyRowStatesAnAbsentCredential(t *testing.T) {
	row := policyRow("jbones", KindTailscale, secrets.Tailnet{}, "")

	if row.CredentialState != CredentialAbsent {
		t.Errorf("CredentialState = %q, want %q", row.CredentialState, CredentialAbsent)
	}
	if row.CredentialPresent {
		t.Error("CredentialPresent is true for a tailnet that holds no credential")
	}
}

func TestThePolicyRowStatesARejectionThatTheControlServerReported(t *testing.T) {
	// The shape of the value is right, therefore only the answer of the control server
	// states that the credential works for no request.
	cred := secrets.Tailnet{
		TailscaleOAuthClientID:     "khjuWjCNTRL",
		TailscaleOAuthClientSecret: "tskey-client-khjuWjCNTRL-abcdefghijklmnop",
	}

	row := policyRow("jbones", KindTailscale, cred, "API token invalid")

	if row.CredentialState != CredentialRejected {
		t.Errorf("CredentialState = %q, want %q", row.CredentialState, CredentialRejected)
	}
	if !strings.Contains(row.Reason, "API token invalid") {
		t.Errorf("Reason = %q, and it holds no message of the control server", row.Reason)
	}
}

func TestTheServerRecordsAndClearsACredentialRejection(t *testing.T) {
	s := &Server{}

	s.recordCredentialResult("jbones", &policy.APIError{Operation: "the token request", StatusCode: 401, Message: "API token invalid"})
	if got := s.credentialRejection("jbones"); got != "API token invalid" {
		t.Errorf("credentialRejection = %q, want the message of the control server", got)
	}

	// A 403 states that the credential is valid and that its scopes do not cover the
	// request, which is a different repair.
	s.recordCredentialResult("havoc", &policy.APIError{Operation: "the policy write", StatusCode: 403, Message: "insufficient scope"})
	if got := s.credentialRejection("havoc"); got != "" {
		t.Errorf("credentialRejection = %q for HTTP 403, want an empty string", got)
	}

	s.recordCredentialResult("jbones", nil)
	if got := s.credentialRejection("jbones"); got != "" {
		t.Errorf("credentialRejection = %q after a call that succeeded, want an empty string", got)
	}
}
