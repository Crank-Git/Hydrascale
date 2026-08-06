package policy

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// The value below is not a credential. It is a fixed string that a test searches for in
// an error and in a log line.
const headscaleTestAPIKey = "TESTVALUE-headscale-api-key-156"

const headscaleTestDocument = `{"acls": [{"action": "accept", "src": ["*"], "dst": ["*:*"]}]}`

// headscaleRequest holds what the test server received.
type headscaleRequest struct {
	method string
	path   string
	auth   string
	body   string
}

// headscaleServer starts a test server that records the request and answers with the
// status and the body that the test names.
func headscaleServer(t *testing.T, status int, response string) (*HeadscaleClient, *headscaleRequest) {
	t.Helper()
	var got headscaleRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("ReadAll: %v", err)
		}
		got = headscaleRequest{
			method: r.Method,
			path:   r.URL.Path,
			auth:   r.Header.Get("Authorization"),
			body:   string(body),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := io.WriteString(w, response); err != nil {
			t.Errorf("WriteString: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	return NewHeadscaleClient(server.URL, headscaleTestAPIKey), &got
}

// The body below is what grpc-gateway returns for types.ErrPolicyUpdateIsDisabled of
// hscontrol/types/policy.go:11 at tag v0.29.3.
const headscaleFileModeBody = `{"code":2,"message":"update is disabled for modes other than 'database'","details":[]}`

func TestHeadscaleWrite_returns_a_distinct_error_for_the_file_policy_mode(t *testing.T) {
	client, _ := headscaleServer(t, http.StatusInternalServerError, headscaleFileModeBody)

	err := client.Write(context.Background(), headscaleTestDocument)
	if !errors.Is(err, ErrHeadscaleFileMode) {
		t.Fatalf("Write returned %v, want ErrHeadscaleFileMode", err)
	}
	if !strings.Contains(err.Error(), `policy.mode: "database"`) {
		t.Errorf("the error does not name policy.mode: \"database\": %v", err)
	}
	if !strings.Contains(err.Error(), "file policy mode") {
		t.Errorf("the error does not name the file policy mode: %v", err)
	}
}

// The body below is what the REST bridge returns for a read against a control server in
// the database policy mode that holds no stored policy. Measured on Headscale v0.29.3 on
// 2026-08-05.
const headscaleNoStoredPolicyBody = `{"code":2,"message":"loading ACL from database: acl policy not found","details":[]}`

func TestHeadscaleRead_reports_an_empty_database_policy_as_no_policy_and_not_as_a_failure(t *testing.T) {
	client, _ := headscaleServer(t, http.StatusInternalServerError, headscaleNoStoredPolicyBody)

	document, err := client.Read(context.Background())
	if err != nil {
		t.Fatalf("Read returned %v, want no error for a control server that holds no policy", err)
	}
	if document != "" {
		t.Errorf("Read returned %q, want an empty document", document)
	}
}

func TestHeadscale_returns_a_distinct_error_for_a_control_server_with_no_policy_route(t *testing.T) {
	cases := []struct {
		name string
		call func(*HeadscaleClient) error
	}{
		{"the read", func(c *HeadscaleClient) error { _, err := c.Read(context.Background()); return err }},
		{"the check", func(c *HeadscaleClient) error { return c.Check(context.Background(), headscaleTestDocument) }},
		{"the write", func(c *HeadscaleClient) error { return c.Write(context.Background(), headscaleTestDocument) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, _ := headscaleServer(t, http.StatusNotFound, `{"code":5,"message":"Not Found","details":[]}`)

			err := tc.call(client)
			if !errors.Is(err, ErrHeadscaleNoPolicyRoute) {
				t.Fatalf("the call returned %v, want ErrHeadscaleNoPolicyRoute", err)
			}
			if !strings.Contains(err.Error(), "v0.29") {
				t.Errorf("the error does not name the version that supports policy access: %v", err)
			}
		})
	}
}

func TestHeadscaleRead_sends_the_api_key_and_returns_the_policy_document(t *testing.T) {
	response, err := json.Marshal(map[string]string{"policy": headscaleTestDocument, "updatedAt": "2026-08-05T00:00:00Z"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	client, got := headscaleServer(t, http.StatusOK, string(response))

	document, err := client.Read(context.Background())
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if document != headscaleTestDocument {
		t.Errorf("Read returned %q, want %q", document, headscaleTestDocument)
	}
	if got.method != http.MethodGet {
		t.Errorf("method = %q, want GET", got.method)
	}
	if got.path != "/api/v1/policy" {
		t.Errorf("path = %q, want /api/v1/policy", got.path)
	}
	if got.auth != "Bearer "+headscaleTestAPIKey {
		t.Errorf("Authorization = %q, want the bearer token", got.auth)
	}
	if got.body != "" {
		t.Errorf("body = %q, want an empty body", got.body)
	}
}

func TestHeadscaleCheck_posts_the_document_to_the_check_route(t *testing.T) {
	client, got := headscaleServer(t, http.StatusOK, `{}`)

	if err := client.Check(context.Background(), headscaleTestDocument); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got.method != http.MethodPost {
		t.Errorf("method = %q, want POST", got.method)
	}
	if got.path != "/api/v1/policy/check" {
		t.Errorf("path = %q, want /api/v1/policy/check", got.path)
	}
	var sent struct {
		Policy string `json:"policy"`
	}
	if err := json.Unmarshal([]byte(got.body), &sent); err != nil {
		t.Fatalf("Unmarshal the request body: %v", err)
	}
	if sent.Policy != headscaleTestDocument {
		t.Errorf("policy = %q, want %q", sent.Policy, headscaleTestDocument)
	}
}

func TestHeadscaleCheck_returns_the_message_of_a_document_that_the_control_server_rejects(t *testing.T) {
	client, _ := headscaleServer(t, http.StatusInternalServerError,
		`{"code":2,"message":"parsing policy: line 3: unknown field \"acl\"","details":[]}`)

	err := client.Check(context.Background(), headscaleTestDocument)
	if err == nil {
		t.Fatal("Check returned no error, want the message of the control server")
	}
	if !strings.Contains(err.Error(), `line 3: unknown field "acl"`) {
		t.Errorf("the error does not hold the message of the control server: %v", err)
	}
	if !strings.Contains(err.Error(), "/api/v1/policy/check") {
		t.Errorf("the error does not name the endpoint: %v", err)
	}
}

func TestHeadscaleWrite_puts_the_document_to_the_policy_route(t *testing.T) {
	response, err := json.Marshal(map[string]string{"policy": headscaleTestDocument, "updatedAt": "2026-08-05T00:00:00Z"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	client, got := headscaleServer(t, http.StatusOK, string(response))

	if err := client.Write(context.Background(), headscaleTestDocument); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got.method != http.MethodPut {
		t.Errorf("method = %q, want PUT", got.method)
	}
	if got.path != "/api/v1/policy" {
		t.Errorf("path = %q, want /api/v1/policy", got.path)
	}
	var sent struct {
		Policy string `json:"policy"`
	}
	if err := json.Unmarshal([]byte(got.body), &sent); err != nil {
		t.Fatalf("Unmarshal the request body: %v", err)
	}
	if sent.Policy != headscaleTestDocument {
		t.Errorf("policy = %q, want %q", sent.Policy, headscaleTestDocument)
	}
}

func TestHeadscaleRead_returns_the_tls_error_of_a_certificate_that_the_host_does_not_trust(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(server.Close)
	client := NewHeadscaleClient(server.URL, headscaleTestAPIKey)

	_, err := client.Read(context.Background())
	if err == nil {
		t.Fatal("Read returned no error, want the TLS error")
	}
	var verification *tls.CertificateVerificationError
	if !errors.As(err, &verification) {
		t.Fatalf("Read returned %v, want a tls.CertificateVerificationError", err)
	}
	if !strings.Contains(err.Error(), "x509") {
		t.Errorf("the error does not hold the TLS message verbatim: %v", err)
	}
}

func TestTheHeadscaleClientAddsNoCertificateOverride(t *testing.T) {
	source, err := os.ReadFile("headscale.go")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	for _, word := range []string{"InsecureSkipVerify", "tls.Config", "RootCAs"} {
		if bytes.Contains(source, []byte(word)) {
			t.Errorf("headscale.go holds %q, and the client adds no certificate override", word)
		}
	}
}

func TestNoLogLineHoldsTheHeadscaleAPIKey(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	cases := []struct {
		name     string
		status   int
		response string
	}{
		{"a call that succeeds", http.StatusOK, `{"policy":"` + `{}` + `"}`},
		{"a control server in the file policy mode", http.StatusInternalServerError, headscaleFileModeBody},
		{"a control server with no policy route", http.StatusNotFound, `{"code":5,"message":"Not Found"}`},
	}
	for _, tc := range cases {
		client, _ := headscaleServer(t, tc.status, tc.response)
		_, readErr := client.Read(context.Background())
		checkErr := client.Check(context.Background(), headscaleTestDocument)
		writeErr := client.Write(context.Background(), headscaleTestDocument)
		for _, err := range []error{readErr, checkErr, writeErr} {
			if err != nil && strings.Contains(err.Error(), headscaleTestAPIKey) {
				t.Errorf("%s: the error holds the API key: %v", tc.name, err)
			}
		}
	}
	if strings.Contains(buf.String(), headscaleTestAPIKey) {
		t.Errorf("a log line holds the API key: %s", buf.String())
	}
}
