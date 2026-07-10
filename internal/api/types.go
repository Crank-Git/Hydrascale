// Package api provides the Unix socket HTTP API for Hydrascale.
package api

import (
	"time"

	"hydrascale/internal/config"
	"hydrascale/internal/reconciler"
)

// DefaultSocketPath is the default Unix socket path for the API server.
const DefaultSocketPath = "/var/lib/hydrascale/api.sock"

// StatusResponse is the JSON response for GET /api/status.
type StatusResponse struct {
	Desired       map[string]config.Tailnet           `json:"desired"`
	Actual        map[string]*reconciler.TailnetState `json:"actual"`
	ErrorStates   map[string]bool                     `json:"error_states"`
	PausedStates  map[string]bool                     `json:"paused_states"`
	FailureCounts map[string]int                      `json:"failure_counts"`
	LastErrors    map[string]string                   `json:"last_errors"`
	ServerVersion string                              `json:"server_version,omitempty"`
}

// EventsResponse is the JSON response for GET /api/events.
type EventsResponse struct {
	Events []reconciler.Event `json:"events"`
}

// ReconcileResponse is the JSON response for POST /api/reconcile.
type ReconcileResponse struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

// TailnetRequest is the request body for tailnet management endpoints.
type TailnetRequest struct {
	ID         string `json:"id"`
	AuthKey    string `json:"auth_key,omitempty"`
	ExitNode   string `json:"exit_node,omitempty"`
	ControlURL string `json:"control_url,omitempty"`
	HostAccess *bool  `json:"host_access,omitempty"`
}

// DNSConfigRequest is the request body for POST /api/config/dns.
type DNSConfigRequest struct {
	Mode        string `json:"mode"`
	BindAddress string `json:"bind_address,omitempty"`
}

// RedactedTailnet is a Tailnet with the auth key hidden.
type RedactedTailnet struct {
	ID         string `json:"id"`
	ExitNode   string `json:"exit_node,omitempty"`
	AuthKey    string `json:"auth_key,omitempty"`
	HostAccess *bool  `json:"host_access,omitempty"`
}

// RedactedConfig mirrors config.Config but with auth keys redacted.
type RedactedConfig struct {
	Version  int               `json:"version"`
	Tailnets []RedactedTailnet `json:"tailnets"`
	Resolver interface{}       `json:"resolver"`
}

// ConfigResponse is the JSON response for GET /api/config.
type ConfigResponse struct {
	Config RedactedConfig `json:"config"`
}

// PeerInfo is a single peer within a tailnet, derived from tailscale status --json.
type PeerInfo struct {
	HostName     string    `json:"host_name"`
	DNSName      string    `json:"dns_name,omitempty"`
	OS           string    `json:"os,omitempty"`
	TailscaleIPs []string  `json:"tailscale_ips,omitempty"`
	AllowedIPs   []string  `json:"allowed_ips,omitempty"`
	Online       bool      `json:"online"`
	LastSeen     time.Time `json:"last_seen,omitempty"`
}

// TailnetDetailResponse is the JSON response for GET /api/tailnet/{id}/detail.
// It contains live data fetched from inside the tailnet's network namespace.
// Config fields (ExitNode, HostAccess) and reconciler route state are NOT included
// here — clients assemble those from GET /api/status and GET /api/config.
// Error is set (with HTTP 200) when the live fetch fails; clients render it inline.
type TailnetDetailResponse struct {
	TailscaleIPs   []string   `json:"tailscale_ips"`
	MagicDNSName   string     `json:"magic_dns_name,omitempty"`
	MagicDNSSuffix string     `json:"magic_dns_suffix,omitempty"`
	PeerCount      int        `json:"peer_count"`
	OnlinePeers    int        `json:"online_peers"`
	Peers          []PeerInfo `json:"peers,omitempty"`
	FetchedAt      time.Time  `json:"fetched_at"`
	Error          string     `json:"error,omitempty"`
}
