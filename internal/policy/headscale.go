package policy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrHeadscaleFileMode reports that the control server runs the file policy mode.
// A write needs the database policy mode. hscontrol/types/config.go:54 of the Headscale
// source at tag v0.29.3 declares the two mode values as PolicyModeDB = "database" and
// PolicyModeFile = "file".
var ErrHeadscaleFileMode = errors.New(`the Headscale control server runs the file policy mode, and a policy write needs policy.mode: "database"`)

// ErrHeadscaleNoPolicyRoute reports that the control server exposes no policy route.
// A control server older than v0.29 does not serve /api/v1/policy.
var ErrHeadscaleNoPolicyRoute = errors.New("the Headscale control server exposes no policy route, and policy access needs Headscale v0.29 or later")

// headscaleFileModeMessage is the text of types.ErrPolicyUpdateIsDisabled, which
// hscontrol/types/policy.go:11 declares at tag v0.29.3. The REST bridge carries the text
// in the message field, and it answers with status 500, so the text is the only signal
// that names the policy mode.
const headscaleFileModeMessage = "update is disabled for modes other than 'database'"

// headscaleNoStoredPolicyMessage is the text that a read returns when the control server
// runs the database policy mode and holds no policy document. The REST bridge answers
// with status 500, so an empty policy reaches the client as an error status rather than
// as an empty document.
const headscaleNoStoredPolicyMessage = "acl policy not found"

// errHeadscaleNoStoredPolicy reports that the control server holds no policy document.
// Read turns this error into an empty document, because no policy is a state of the
// control server and not a failure.
var errHeadscaleNoStoredPolicy = errors.New("the Headscale control server holds no policy document")

// headscaleMaxResponse limits a response body to 1 megabyte, which is the limit that the
// control API applies to a request body.
const headscaleMaxResponse = 1 << 20

const (
	headscalePolicyPath = "/api/v1/policy"
	headscaleCheckPath  = "/api/v1/policy/check"
)

// HeadscaleClient reads, checks, and writes the policy document of one Headscale control
// server. Headscale exposes its gRPC service over a grpc-gateway REST bridge, so the
// client uses net/http alone.
type HeadscaleClient struct {
	address string
	apiKey  string
	http    *http.Client
}

// NewHeadscaleClient returns a client for the control server that address names.
// address is the value of headscale_address in the credential block, and apiKey is the
// value of headscale_api_key. The client keeps the trust store of the host, because
// version 1.0 adds no certificate override.
func NewHeadscaleClient(address, apiKey string) *HeadscaleClient {
	return &HeadscaleClient{
		address: strings.TrimRight(address, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Read returns the policy document of the control server.
// Read works in both policy modes. Read returns an empty document and no error when the
// control server holds no policy document. Read returns ErrHeadscaleNoPolicyRoute when
// the control server exposes no policy route, and it returns the transport error
// unchanged when the request does not reach the control server.
func (c *HeadscaleClient) Read(ctx context.Context) (string, error) {
	data, err := c.do(ctx, http.MethodGet, headscalePolicyPath, nil)
	if errors.Is(err, errHeadscaleNoStoredPolicy) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var response struct {
		Policy string `json:"policy"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return "", fmt.Errorf("the answer of the Headscale control server to %s is not valid JSON", headscalePolicyPath)
	}
	return response.Policy, nil
}

// Check sends document to the check route of the control server.
// Check returns nil when the control server accepts the document. Check returns an error
// that holds the message of the control server when the control server rejects it. The
// control server checks a document in both policy modes.
func (c *HeadscaleClient) Check(ctx context.Context, document string) error {
	_, err := c.do(ctx, http.MethodPost, headscaleCheckPath, document)
	return err
}

// Write sends document to the policy route of the control server.
// Write returns ErrHeadscaleFileMode when the control server runs the file policy mode,
// because a write succeeds only in the database policy mode. Write returns
// ErrHeadscaleNoPolicyRoute when the control server exposes no policy route.
func (c *HeadscaleClient) Write(ctx context.Context, document string) error {
	_, err := c.do(ctx, http.MethodPut, headscalePolicyPath, document)
	return err
}

// do sends one request and returns the response body.
// method and path name the request. document is the policy document of a request that
// carries a body, and it is nil for a request that carries none. do returns the error of
// the transport unchanged, so a TLS failure reaches the operator verbatim.
func (c *HeadscaleClient) do(ctx context.Context, method, path string, document any) ([]byte, error) {
	var body io.Reader
	if document != nil {
		encoded, err := json.Marshal(map[string]any{"policy": document})
		if err != nil {
			return nil, fmt.Errorf("failed to encode the policy document for %s: %w", path, err)
		}
		body = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.address+path, body)
	if err != nil {
		return nil, fmt.Errorf("failed to build the request for %s: %w", path, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	if document != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, headscaleMaxResponse))
	if err != nil {
		return nil, fmt.Errorf("failed to read the answer of the Headscale control server to %s: %w", path, err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrHeadscaleNoPolicyRoute
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		message := headscaleMessage(data, resp.StatusCode)
		if strings.Contains(message, headscaleFileModeMessage) {
			return nil, ErrHeadscaleFileMode
		}
		if strings.Contains(message, headscaleNoStoredPolicyMessage) {
			return nil, errHeadscaleNoStoredPolicy
		}
		return nil, fmt.Errorf("the Headscale control server answered %s with status %d: %s", path, resp.StatusCode, message)
	}
	return data, nil
}

// headscaleMessage returns the message field of an error answer of the REST bridge.
// data is the response body and status is the HTTP status. headscaleMessage returns the
// text of the status when the body holds no message field, because a body of an unknown
// shape can hold a value that the operator must not see in a log line.
func headscaleMessage(data []byte, status int) string {
	var answer struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(data, &answer); err == nil && answer.Message != "" {
		return answer.Message
	}
	return http.StatusText(status)
}
