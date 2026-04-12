package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// Client connects to the Hydrascale API over a Unix socket.
type Client struct {
	socketPath string
	httpClient *http.Client
}

// NewClient creates a Client that communicates over the given Unix socket path.
func NewClient(socketPath string) *Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
		},
	}
	return &Client{
		socketPath: socketPath,
		httpClient: &http.Client{Transport: transport},
	}
}

// IsAvailable returns true if the socket exists and accepts connections.
func (c *Client) IsAvailable() bool {
	conn, err := net.Dial("unix", c.socketPath)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// Status calls GET /api/status and returns the response.
func (c *Client) Status() (*StatusResponse, error) {
	resp, err := c.httpClient.Get("http://localhost/api/status")
	if err != nil {
		return nil, fmt.Errorf("status request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status returned HTTP %d", resp.StatusCode)
	}

	var result StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode status response: %w", err)
	}
	return &result, nil
}

// Events calls GET /api/events and returns the response.
func (c *Client) Events() (*EventsResponse, error) {
	resp, err := c.httpClient.Get("http://localhost/api/events")
	if err != nil {
		return nil, fmt.Errorf("events request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("events returned HTTP %d", resp.StatusCode)
	}

	var result EventsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode events response: %w", err)
	}
	return &result, nil
}

// Reconcile calls POST /api/reconcile and returns the response.
func (c *Client) Reconcile() (*ReconcileResponse, error) {
	resp, err := c.httpClient.Post("http://localhost/api/reconcile", "application/json", strings.NewReader("{}"))
	if err != nil {
		return nil, fmt.Errorf("reconcile request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("reconcile returned HTTP %d", resp.StatusCode)
	}

	var result ReconcileResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode reconcile response: %w", err)
	}
	return &result, nil
}

// postJSON marshals body as JSON and POSTs it to path, decoding the response into result.
func (c *Client) postJSON(path string, body interface{}, result interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}
	resp, err := c.httpClient.Post("http://localhost"+path, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("request to %s failed: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned HTTP %d", path, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("failed to decode response from %s: %w", path, err)
	}
	return nil
}

// AddTailnet calls POST /api/tailnet/add.
func (c *Client) AddTailnet(id, authKey, exitNode string) (*ReconcileResponse, error) {
	req := TailnetRequest{ID: id, AuthKey: authKey, ExitNode: exitNode}
	var result ReconcileResponse
	if err := c.postJSON("/api/tailnet/add", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// RemoveTailnet calls POST /api/tailnet/remove.
func (c *Client) RemoveTailnet(id string) (*ReconcileResponse, error) {
	req := TailnetRequest{ID: id}
	var result ReconcileResponse
	if err := c.postJSON("/api/tailnet/remove", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ConnectTailnet calls POST /api/tailnet/connect.
func (c *Client) ConnectTailnet(id string) (*ReconcileResponse, error) {
	req := TailnetRequest{ID: id}
	var result ReconcileResponse
	if err := c.postJSON("/api/tailnet/connect", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DisconnectTailnet calls POST /api/tailnet/disconnect.
func (c *Client) DisconnectTailnet(id string) (*ReconcileResponse, error) {
	req := TailnetRequest{ID: id}
	var result ReconcileResponse
	if err := c.postJSON("/api/tailnet/disconnect", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// UpdateDNS calls POST /api/config/dns.
func (c *Client) UpdateDNS(mode, bindAddress string) (*ReconcileResponse, error) {
	req := DNSConfigRequest{Mode: mode, BindAddress: bindAddress}
	var result ReconcileResponse
	if err := c.postJSON("/api/config/dns", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// TailnetDetail calls GET /api/tailnet/{id}/detail and returns live Tailscale status.
// A non-nil response with a non-empty Error field means the daemon was unreachable.
func (c *Client) TailnetDetail(id string) (*TailnetDetailResponse, error) {
	resp, err := c.httpClient.Get("http://localhost/api/tailnet/" + url.PathEscape(id) + "/detail")
	if err != nil {
		return nil, fmt.Errorf("detail request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("tailnet %s not found", id)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("detail returned HTTP %d", resp.StatusCode)
	}
	var result TailnetDetailResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode detail response: %w", err)
	}
	return &result, nil
}

// GetConfig calls GET /api/config and returns the redacted config.
func (c *Client) GetConfig() (*ConfigResponse, error) {
	resp, err := c.httpClient.Get("http://localhost/api/config")
	if err != nil {
		return nil, fmt.Errorf("config request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("config returned HTTP %d", resp.StatusCode)
	}
	var result ConfigResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode config response: %w", err)
	}
	return &result, nil
}
