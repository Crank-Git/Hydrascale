package policy

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"hydrascale/internal/secrets"
)

// credentials_test.go declares testClientID and testClientSecret. The values below are
// not credentials either. Each one is a fixed string that a test searches for.
const (
	testAccessToken = "TESTVALUE-access-token-155"
	testTailnet     = "example.com"
	testDocument    = "{\n\t// A comment, which huJSON allows.\n\t\"acls\": [],\n}\n"
	testETag        = "\"e0b2816b418\""
)

// recordedRequest holds what the fake control server received.
type recordedRequest struct {
	Method        string
	Path          string
	ContentType   string
	Accept        string
	Authorization string
	IfMatch       string
	Body          string
}

// fakeControlServer is a local control server that replaces api.tailscale.com.
// No test reaches a real tailnet, because a policy write reaches every device of that
// tailnet.
type fakeControlServer struct {
	server *httptest.Server

	mu       sync.Mutex
	requests []recordedRequest

	tokenStatus   int
	tokenBody     string
	tokenExpires  int
	aclStatus     int
	aclBody       string
	aclETag       string
	writeStatus   int
	writeBody     string
	validateBody  string
	validateState int
}

func newFakeControlServer(t *testing.T) *fakeControlServer {
	t.Helper()
	f := &fakeControlServer{
		tokenStatus:   http.StatusOK,
		tokenExpires:  3600,
		aclStatus:     http.StatusOK,
		aclBody:       testDocument,
		aclETag:       testETag,
		writeStatus:   http.StatusOK,
		writeBody:     testDocument,
		validateState: http.StatusOK,
	}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeControlServer) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.requests = append(f.requests, recordedRequest{
		Method:        r.Method,
		Path:          r.URL.Path,
		ContentType:   r.Header.Get("Content-Type"),
		Accept:        r.Header.Get("Accept"),
		Authorization: r.Header.Get("Authorization"),
		IfMatch:       r.Header.Get("If-Match"),
		Body:          string(body),
	})
	f.mu.Unlock()

	switch {
	case r.URL.Path == "/oauth/token":
		if f.tokenStatus != http.StatusOK {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(f.tokenStatus)
			io.WriteString(w, f.tokenBody)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"access_token":"`+testAccessToken+`","token_type":"Bearer","expires_in":`+strconv.Itoa(f.tokenExpires)+`}`)
	case r.URL.Path == "/tailnet/"+testTailnet+"/acl" && r.Method == http.MethodGet:
		if f.aclETag != "" {
			w.Header().Set("ETag", f.aclETag)
		}
		w.Header().Set("Content-Type", "application/hujson")
		w.WriteHeader(f.aclStatus)
		io.WriteString(w, f.aclBody)
	case r.URL.Path == "/tailnet/"+testTailnet+"/acl" && r.Method == http.MethodPost:
		w.Header().Set("Content-Type", "application/hujson")
		w.WriteHeader(f.writeStatus)
		io.WriteString(w, f.writeBody)
	case r.URL.Path == "/tailnet/"+testTailnet+"/acl/validate":
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(f.validateState)
		io.WriteString(w, f.validateBody)
	default:
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"message":"not found"}`)
	}
}

func (f *fakeControlServer) recorded() []recordedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]recordedRequest, len(f.requests))
	copy(out, f.requests)
	return out
}

// newTestClient returns a client that talks to the fake control server with a valid
// credential.
func newTestClient(f *fakeControlServer) *TailscaleClient {
	return NewTailscaleClient(f.server.URL, testTailnet, func() (secrets.Tailnet, error) {
		return secrets.Tailnet{
			TailscaleOAuthClientID:     testClientID,
			TailscaleOAuthClientSecret: testClientSecret,
		}, nil
	})
}

// captureLog sends the standard logger to a buffer for the duration of the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	previous := log.Writer()
	flags := log.Flags()
	log.SetOutput(&buf)
	t.Cleanup(func() {
		log.SetOutput(previous)
		log.SetFlags(flags)
	})
	return &buf
}

func TestTheClientRequestsAnAccessTokenWithTheClientCredentials(t *testing.T) {
	f := newFakeControlServer(t)
	client := newTestClient(f)

	if _, err := client.ReadPolicy(context.Background()); err != nil {
		t.Fatalf("ReadPolicy returned an error: %v", err)
	}

	got := f.recorded()
	if len(got) != 2 {
		t.Fatalf("the control server received %d requests, want 2", len(got))
	}
	token := got[0]
	if token.Method != http.MethodPost {
		t.Errorf("the token request method is %s, want POST", token.Method)
	}
	if token.Path != "/oauth/token" {
		t.Errorf("the token request path is %s, want /oauth/token", token.Path)
	}
	if token.ContentType != "application/x-www-form-urlencoded" {
		t.Errorf("the token request content type is %s, want application/x-www-form-urlencoded", token.ContentType)
	}
	want := "client_id=" + testClientID + "&client_secret=" + testClientSecret + "&grant_type=client_credentials"
	if token.Body != want {
		t.Errorf("the token request body is %q, want %q", token.Body, want)
	}
}

func TestTheClientReusesTheAccessTokenUntilItExpires(t *testing.T) {
	f := newFakeControlServer(t)
	client := newTestClient(f)

	for i := 0; i < 2; i++ {
		if _, err := client.ReadPolicy(context.Background()); err != nil {
			t.Fatalf("ReadPolicy returned an error: %v", err)
		}
	}

	tokenRequests := 0
	for _, r := range f.recorded() {
		if r.Path == "/oauth/token" {
			tokenRequests++
		}
	}
	if tokenRequests != 1 {
		t.Errorf("the client sent %d token requests, want 1", tokenRequests)
	}
}

func TestTheClientRequestsANewAccessTokenAfterTheCachedTokenExpires(t *testing.T) {
	f := newFakeControlServer(t)
	client := newTestClient(f)
	now := time.Unix(1000, 0)
	client.now = func() time.Time { return now }

	if _, err := client.ReadPolicy(context.Background()); err != nil {
		t.Fatalf("the first ReadPolicy returned an error: %v", err)
	}
	now = now.Add(2 * time.Hour)
	if _, err := client.ReadPolicy(context.Background()); err != nil {
		t.Fatalf("the second ReadPolicy returned an error: %v", err)
	}

	tokenRequests := 0
	for _, r := range f.recorded() {
		if r.Path == "/oauth/token" {
			tokenRequests++
		}
	}
	if tokenRequests != 2 {
		t.Errorf("the client sent %d token requests, want 2", tokenRequests)
	}
}

func TestTheClientReadsThePolicyDocumentAndTheETagValue(t *testing.T) {
	f := newFakeControlServer(t)
	client := newTestClient(f)

	doc, err := client.ReadPolicy(context.Background())
	if err != nil {
		t.Fatalf("ReadPolicy returned an error: %v", err)
	}
	if doc.Text != testDocument {
		t.Errorf("the document is %q, want %q", doc.Text, testDocument)
	}
	if doc.ETag != testETag {
		t.Errorf("the ETag value is %q, want %q", doc.ETag, testETag)
	}

	read := f.recorded()[1]
	if read.Method != http.MethodGet {
		t.Errorf("the read method is %s, want GET", read.Method)
	}
	if read.Path != "/tailnet/"+testTailnet+"/acl" {
		t.Errorf("the read path is %s, want /tailnet/%s/acl", read.Path, testTailnet)
	}
	if read.Accept != "application/hujson" {
		t.Errorf("the read accept header is %s, want application/hujson", read.Accept)
	}
	if read.Authorization != "Bearer "+testAccessToken {
		t.Errorf("the read authorization header is %q, want the bearer token", read.Authorization)
	}
}

func TestTheClientSendsIfMatchWithTheETagValueFromTheRead(t *testing.T) {
	f := newFakeControlServer(t)
	client := newTestClient(f)

	doc, err := client.ReadPolicy(context.Background())
	if err != nil {
		t.Fatalf("ReadPolicy returned an error: %v", err)
	}
	if _, err := client.WritePolicy(context.Background(), testDocument, doc.ETag); err != nil {
		t.Fatalf("WritePolicy returned an error: %v", err)
	}

	got := f.recorded()
	write := got[len(got)-1]
	if write.Method != http.MethodPost {
		t.Errorf("the write method is %s, want POST", write.Method)
	}
	if write.Path != "/tailnet/"+testTailnet+"/acl" {
		t.Errorf("the write path is %s, want /tailnet/%s/acl", write.Path, testTailnet)
	}
	if write.IfMatch != testETag {
		t.Errorf("the If-Match header is %q, want %q", write.IfMatch, testETag)
	}
	if write.ContentType != "application/hujson" {
		t.Errorf("the write content type is %s, want application/hujson", write.ContentType)
	}
	if write.Body != testDocument {
		t.Errorf("the write body is %q, want %q", write.Body, testDocument)
	}
}

func TestTheClientSendsNoIfMatchHeaderWhenTheETagValueIsEmpty(t *testing.T) {
	f := newFakeControlServer(t)
	client := newTestClient(f)

	if _, err := client.WritePolicy(context.Background(), testDocument, ""); err != nil {
		t.Fatalf("WritePolicy returned an error: %v", err)
	}

	got := f.recorded()
	write := got[len(got)-1]
	if write.IfMatch != "" {
		t.Errorf("the If-Match header is %q, want no header", write.IfMatch)
	}
}

func TestTheClientReturnsADistinctErrorForHTTP412(t *testing.T) {
	f := newFakeControlServer(t)
	f.writeStatus = http.StatusPreconditionFailed
	f.writeBody = `{"message":"precondition failed, invalid old hash"}`
	client := newTestClient(f)

	_, err := client.WritePolicy(context.Background(), testDocument, testETag)
	if !errors.Is(err, ErrPolicyConflict) {
		t.Fatalf("WritePolicy returned %v, want ErrPolicyConflict", err)
	}
	if !strings.Contains(err.Error(), "precondition failed, invalid old hash") {
		t.Errorf("the error is %q, and it does not hold the control server message", err)
	}
}

func TestTheClientPostsTheDocumentToTheValidateEndpointAndReturnsTheResult(t *testing.T) {
	f := newFakeControlServer(t)
	client := newTestClient(f)

	result, err := client.ValidatePolicy(context.Background(), testDocument)
	if err != nil {
		t.Fatalf("ValidatePolicy returned an error: %v", err)
	}
	if !result.Passed {
		t.Errorf("the result is not passed, and the control server returned an empty body")
	}

	got := f.recorded()
	validate := got[len(got)-1]
	if validate.Method != http.MethodPost {
		t.Errorf("the validate method is %s, want POST", validate.Method)
	}
	if validate.Path != "/tailnet/"+testTailnet+"/acl/validate" {
		t.Errorf("the validate path is %s, want /tailnet/%s/acl/validate", validate.Path, testTailnet)
	}
	if validate.Body != testDocument {
		t.Errorf("the validate body is %q, want the document", validate.Body)
	}
}

func TestTheValidateResultCarriesTheControlServerMessageOnAFailure(t *testing.T) {
	f := newFakeControlServer(t)
	f.validateBody = `{"message":"test(s) failed","data":[{"user":"user1@example.com"}]}`
	client := newTestClient(f)

	result, err := client.ValidatePolicy(context.Background(), testDocument)
	if err != nil {
		t.Fatalf("ValidatePolicy returned an error: %v", err)
	}
	if result.Passed {
		t.Errorf("the result is passed, and the control server returned a message")
	}
	if result.Body != f.validateBody {
		t.Errorf("the result body is %q, want %q", result.Body, f.validateBody)
	}
}

func TestTheValidateResultMarksAFailedTestAssertion(t *testing.T) {
	f := newFakeControlServer(t)
	f.validateBody = `{"message":"test(s) failed","data":[{"user":"user1@example.com","errors":["address \"2.2.2.2:22\": want: Drop, got: Accept"]}]}`
	client := newTestClient(f)

	result, err := client.ValidatePolicy(context.Background(), testDocument)
	if err != nil {
		t.Fatalf("ValidatePolicy returned an error: %v", err)
	}
	if result.Passed {
		t.Errorf("the result is passed, and an assertion of the document failed")
	}
	if !result.TestsFailed {
		t.Errorf("the result marks no failed test, and the control server stated test(s) failed")
	}
}

func TestTheValidateResultMarksNoFailedTestForAWarning(t *testing.T) {
	f := newFakeControlServer(t)
	f.validateBody = `{"message":"warning(s) found","data":[{"user":"group:unknown@example.com"}]}`
	client := newTestClient(f)

	result, err := client.ValidatePolicy(context.Background(), testDocument)
	if err != nil {
		t.Fatalf("ValidatePolicy returned an error: %v", err)
	}
	if result.TestsFailed {
		t.Errorf("the result marks a failed test, and the control server stated warning(s) found")
	}
}

func TestTheClientSendsNoRequestWhenTheCredentialIsAbsent(t *testing.T) {
	f := newFakeControlServer(t)
	client := NewTailscaleClient(f.server.URL, testTailnet, func() (secrets.Tailnet, error) {
		return secrets.Tailnet{}, nil
	})

	_, err := client.ReadPolicy(context.Background())
	if !errors.Is(err, ErrNoCredential) {
		t.Fatalf("ReadPolicy returned %v, want ErrNoCredential", err)
	}
	if n := len(f.recorded()); n != 0 {
		t.Errorf("the control server received %d requests, want 0", n)
	}
}

func TestAFailedTokenRequestProducesTheControlServerMessage(t *testing.T) {
	f := newFakeControlServer(t)
	f.tokenStatus = http.StatusUnauthorized
	f.tokenBody = `{"message":"invalid client credentials"}`
	client := newTestClient(f)

	_, err := client.ReadPolicy(context.Background())
	if err == nil {
		t.Fatal("ReadPolicy returned no error, and the token request failed")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("ReadPolicy returned %v, want an *APIError", err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("the status code is %d, want 401", apiErr.StatusCode)
	}
	if apiErr.Message != "invalid client credentials" {
		t.Errorf("the message is %q, want the control server message", apiErr.Message)
	}
}

func TestNoLogLineHoldsTheOAuthClientSecretOnASuccess(t *testing.T) {
	buf := captureLog(t)
	f := newFakeControlServer(t)
	client := newTestClient(f)

	doc, err := client.ReadPolicy(context.Background())
	if err != nil {
		t.Fatalf("ReadPolicy returned an error: %v", err)
	}
	if strings.Contains(buf.String(), testClientSecret) {
		t.Errorf("a log line holds the OAuth client secret: %q", buf.String())
	}
	if strings.Contains(doc.Text, testClientSecret) {
		t.Errorf("the document holds the OAuth client secret")
	}
}

func TestNoLogLineHoldsTheOAuthClientSecretOnATokenFailure(t *testing.T) {
	buf := captureLog(t)
	f := newFakeControlServer(t)
	f.tokenStatus = http.StatusUnauthorized
	f.tokenBody = `{"message":"invalid client credentials"}`
	client := newTestClient(f)

	_, err := client.ReadPolicy(context.Background())
	if err == nil {
		t.Fatal("ReadPolicy returned no error, and the token request failed")
	}
	if strings.Contains(buf.String(), testClientSecret) {
		t.Errorf("a log line holds the OAuth client secret: %q", buf.String())
	}
	if strings.Contains(err.Error(), testClientSecret) {
		t.Errorf("the error message holds the OAuth client secret: %q", err)
	}
	if strings.Contains(buf.String(), testClientID) {
		t.Errorf("a log line holds the OAuth client identifier: %q", buf.String())
	}
}

func TestTheClientReturnsTheControlServerStatusForAnUnknownTailnet(t *testing.T) {
	f := newFakeControlServer(t)
	client := NewTailscaleClient(f.server.URL, "absent.example.com", func() (secrets.Tailnet, error) {
		return secrets.Tailnet{
			TailscaleOAuthClientID:     testClientID,
			TailscaleOAuthClientSecret: testClientSecret,
		}, nil
	})

	_, err := client.ReadPolicy(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("ReadPolicy returned %v, want an *APIError", err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Errorf("the status code is %d, want 404", apiErr.StatusCode)
	}
	if apiErr.Message != "not found" {
		t.Errorf("the message is %q, want the control server message", apiErr.Message)
	}
}

func TestTheClientReturnsTheErrorOfTheCredentialReader(t *testing.T) {
	f := newFakeControlServer(t)
	want := errors.New("failed to read the secrets file /etc/hydrascale/secrets.yaml")
	client := NewTailscaleClient(f.server.URL, testTailnet, func() (secrets.Tailnet, error) {
		return secrets.Tailnet{}, want
	})

	_, err := client.ReadPolicy(context.Background())
	if !errors.Is(err, want) {
		t.Fatalf("ReadPolicy returned %v, want the error of the credential reader", err)
	}
	if n := len(f.recorded()); n != 0 {
		t.Errorf("the control server received %d requests, want 0", n)
	}
}
