package policy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"hydrascale/internal/secrets"
)

// DefaultTailscaleBaseURL is the base URL of the Tailscale API.
// Verified against the Tailscale OpenAPI schema, retrieved 2026-08-05.
const DefaultTailscaleBaseURL = "https://api.tailscale.com/api/v2"

// hujsonContentType is the media type of a policy document.
// The document is huJSON, which allows a comment and a trailing comma, so the daemon
// carries it as text and never parses it as JSON.
const hujsonContentType = "application/hujson"

// maxResponseBytes bounds one response of the control server. A policy document of 400
// kilobytes is a stated edge case, so the bound is large enough for one.
const maxResponseBytes = 4 << 20

// tokenExpiryMargin subtracts from the lifetime that the control server reports, so the
// daemon requests a new access token before the cached one expires in flight.
const tokenExpiryMargin = 60 * time.Second

// ErrNoCredential reports that the tailnet holds no OAuth client identifier and no OAuth
// client secret. The client sends no request in this case. See FR-policy-13.
var ErrNoCredential = errors.New("the tailnet has no Tailscale OAuth credential")

// ErrPolicyConflict reports that the policy changed since the read.
// The control server returns HTTP 412 when the If-Match value does not match its ETag
// value. See FR-policy-18.
var ErrPolicyConflict = errors.New("the policy changed since the read")

// APIError holds the status and the message that the control server returned.
// The daemon shows the message to the operator. The message never holds a credential,
// because the client reads the field message alone from the response body.
type APIError struct {
	Operation  string
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("%s failed with HTTP %d", e.Operation, e.StatusCode)
	}
	return fmt.Sprintf("%s failed with HTTP %d: %s", e.Operation, e.StatusCode, e.Message)
}

// TextDocument is one policy document and the ETag value that the control server returned.
type TextDocument struct {
	Text string
	ETag string
}

// ValidateResult is the answer of the validate endpoint.
// Passed is true when the control server accepted the document. Body holds the response
// verbatim, because the console shows each error with its line number. See FR-policy-26.
// TestsFailed is true when an assertion of the tests section failed, and false when the
// control server refused the document itself. The console states a different reason for
// each, per FR-vadv-16.
type ValidateResult struct {
	Passed      bool
	Body        string
	TestsFailed bool
}

// testsFailedMessage is the message that the validate endpoint states when an assertion
// of the tests section or of the sshTests section fails. The OpenAPI schema of
// operationId validateAndTestPolicyFile names this value and the value
// "warning(s) found", retrieved 2026-08-24. A message outside this set reaches the
// console as a document error, which states the answer of the control server verbatim.
const testsFailedMessage = "test(s) failed"

// TailscaleClient reads, validates, and writes the policy of one Tailscale tailnet.
//
// The client holds no credential between requests. It calls credential at the moment it
// needs one, which is the behaviour rule of features/08-upstream-policy.md. It does hold
// the access token, because a token request per call is wasteful and the control server
// rate-limits.
type TailscaleClient struct {
	baseURL    string
	tailnet    string
	credential func() (secrets.Tailnet, error)
	httpClient *http.Client
	now        func() time.Time

	mu          sync.Mutex
	token       string
	tokenExpiry time.Time
}

// NewTailscaleClient returns a client for the tailnet that tailnet names.
// baseURL names the API root, and an empty baseURL selects DefaultTailscaleBaseURL.
// credential returns the credential of the tailnet, which policy.LoadCredential supplies.
func NewTailscaleClient(baseURL, tailnet string, credential func() (secrets.Tailnet, error)) *TailscaleClient {
	if baseURL == "" {
		baseURL = DefaultTailscaleBaseURL
	}
	return &TailscaleClient{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		tailnet:    tailnet,
		credential: credential,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		now:        time.Now,
	}
}

// ReadPolicy returns the policy document of the tailnet and the ETag value.
// ReadPolicy returns ErrNoCredential when the tailnet holds no credential, and it then
// sends no request. It returns an *APIError when the control server rejects the request.
// See FR-policy-9 and FR-policy-12.
func (c *TailscaleClient) ReadPolicy(ctx context.Context) (TextDocument, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/tailnet/"+url.PathEscape(c.tailnet)+"/acl", "")
	if err != nil {
		return TextDocument{}, err
	}
	req.Header.Set("Accept", hujsonContentType)

	body, header, err := c.send(req, "the policy read")
	if err != nil {
		return TextDocument{}, err
	}
	return TextDocument{Text: body, ETag: header.Get("ETag")}, nil
}

// WritePolicy writes document to the control server and returns the document that the
// control server returned.
// etag carries the ETag value from the read, and the client sends it as the If-Match
// header. An empty etag sends no If-Match header. A mismatch returns an error that wraps
// ErrPolicyConflict. See FR-policy-16 and FR-policy-17.
func (c *TailscaleClient) WritePolicy(ctx context.Context, document, etag string) (TextDocument, error) {
	req, err := c.newRequest(ctx, http.MethodPost, "/tailnet/"+url.PathEscape(c.tailnet)+"/acl", document)
	if err != nil {
		return TextDocument{}, err
	}
	req.Header.Set("Content-Type", hujsonContentType)
	req.Header.Set("Accept", hujsonContentType)
	if etag != "" {
		req.Header.Set("If-Match", etag)
	}

	body, header, err := c.send(req, "the policy write")
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusPreconditionFailed {
		return TextDocument{}, fmt.Errorf("%w: %s", ErrPolicyConflict, apiErr.Message)
	}
	if err != nil {
		return TextDocument{}, err
	}
	return TextDocument{Text: body, ETag: header.Get("ETag")}, nil
}

// ValidatePolicy sends document to the validate endpoint and returns the result.
// The control server returns an empty body when the document passes, which the OpenAPI
// schema states. See FR-policy-14.
func (c *TailscaleClient) ValidatePolicy(ctx context.Context, document string) (ValidateResult, error) {
	req, err := c.newRequest(ctx, http.MethodPost, "/tailnet/"+url.PathEscape(c.tailnet)+"/acl/validate", document)
	if err != nil {
		return ValidateResult{}, err
	}
	// The schema names application/json for this endpoint alone. It names
	// application/hujson for the read and for the write.
	req.Header.Set("Content-Type", "application/json")

	body, _, err := c.send(req, "the policy validate")
	if err != nil {
		return ValidateResult{}, err
	}

	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return ValidateResult{Passed: true, Body: body}, nil
	}
	var answer struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(trimmed), &answer); err != nil {
		return ValidateResult{Passed: false, Body: body}, nil
	}
	// A non-2xx status reaches send, which returns an error, therefore this point holds
	// status 200 alone. The control server states a failed assertion at status 200, and it
	// refuses a malformed document with status 400.
	return ValidateResult{
		Passed:      answer.Message == "",
		Body:        body,
		TestsFailed: strings.TrimSpace(answer.Message) == testsFailedMessage,
	}, nil
}

// newRequest returns a request to path with the access token as the bearer token.
// newRequest reads the credential, so it returns ErrNoCredential before it builds the
// request when the tailnet holds none.
func (c *TailscaleClient) newRequest(ctx context.Context, method, path, body string) (*http.Request, error) {
	token, err := c.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, fmt.Errorf("failed to build the request to the control server: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return req, nil
}

// accessToken returns a valid access token. It returns the cached token while that token
// is valid, and it requests a new token otherwise.
func (c *TailscaleClient) accessToken(ctx context.Context) (string, error) {
	cred, err := c.credential()
	if err != nil {
		return "", err
	}
	if cred.TailscaleOAuthClientID == "" || cred.TailscaleOAuthClientSecret == "" {
		return "", ErrNoCredential
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && c.now().Before(c.tokenExpiry) {
		return c.token, nil
	}

	token, lifetime, err := c.requestToken(ctx, cred)
	if err != nil {
		return "", err
	}
	c.token = token
	c.tokenExpiry = c.now().Add(lifetime - tokenExpiryMargin)
	return token, nil
}

// requestToken returns a new access token and its lifetime.
// The request conforms to the OAuth 2.0 client credentials grant, which the Tailscale
// OAuth documentation states. requestToken returns an *APIError when the control server
// rejects the client credentials.
func (c *TailscaleClient) requestToken(ctx context.Context, cred secrets.Tailnet) (string, time.Duration, error) {
	form := url.Values{
		"client_id":     {cred.TailscaleOAuthClientID},
		"client_secret": {cred.TailscaleOAuthClientSecret},
		"grant_type":    {"client_credentials"},
	}
	endpoint := c.baseURL + "/oauth/token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, fmt.Errorf("failed to build the token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// The URL of the error of net/http holds the endpoint alone, because the client
		// secret travels in the body.
		return "", 0, fmt.Errorf("the token request to the control server failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return "", 0, fmt.Errorf("failed to read the token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// The log line names the endpoint and the status. It never names the request
		// body, which holds the client secret. See FR-policy-8.
		log.Printf("policy: the token request to %s failed with HTTP %d", endpoint, resp.StatusCode)
		return "", 0, &APIError{
			Operation:  "the token request",
			StatusCode: resp.StatusCode,
			Message:    messageOf(body),
		}
	}

	var answer struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		// The message names no part of the body, because a parser error can quote the
		// line that failed.
		return "", 0, fmt.Errorf("failed to parse the token response of the control server")
	}
	if answer.AccessToken == "" {
		return "", 0, fmt.Errorf("the control server returned no access token")
	}
	lifetime := time.Duration(answer.ExpiresIn) * time.Second
	if lifetime <= tokenExpiryMargin {
		lifetime = tokenExpiryMargin + time.Second
	}
	return answer.AccessToken, lifetime, nil
}

// send runs req and returns the response body and the response header.
// operation names the call in an error message. send returns an *APIError for every
// status other than HTTP 200.
func (c *TailscaleClient) send(req *http.Request, operation string) (string, http.Header, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("%s failed: %w", operation, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return "", nil, fmt.Errorf("failed to read the answer of the control server: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", resp.Header, &APIError{
			Operation:  operation,
			StatusCode: resp.StatusCode,
			Message:    messageOf(body),
		}
	}
	return string(body), resp.Header, nil
}

// messageOf returns the field message of an error body of the control server.
// messageOf returns an empty string when the body is not the documented shape. It never
// returns the body itself, because a body that the daemon did not parse can hold text
// that the daemon must not repeat.
func messageOf(body []byte) string {
	var answer struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		return ""
	}
	return answer.Message
}
