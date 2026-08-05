// Package api provides the Unix socket HTTP API for Hydrascale.
package api

import (
	"time"

	"hydrascale/internal/access"
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
	Access        *AccessStatus                       `json:"access,omitempty"`
}

// AccessStatus is the access field of GET /api/status. It holds the mode that the daemon
// applies the local rule set in, and the count of rules.
type AccessStatus struct {
	Mode  string `json:"mode"`
	Rules int    `json:"rules"`
}

// AccessNode is one endpoint of the local rule model. Kind holds tailnet, host, or
// internet. Peers and Veth carry a value for a tailnet only.
type AccessNode struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Peers int    `json:"peers,omitempty"`
	Veth  string `json:"veth,omitempty"`
}

// AccessRequest is the request body of PUT /api/access. It replaces the whole rule set.
type AccessRequest struct {
	Mode  string        `json:"mode"`
	Rules []access.Rule `json:"rules"`
}

// AccessResponse is the JSON response for GET /api/access and for PUT /api/access.
// Rules is never null, because the console reads the field as a list.
type AccessResponse struct {
	Mode  string        `json:"mode"`
	Rules []access.Rule `json:"rules"`
	Nodes []AccessNode  `json:"nodes"`
}

// EventsResponse is the JSON response for GET /api/events.
type EventsResponse struct {
	Events []reconciler.Event `json:"events"`
}

// ErrorResponse is the JSON body that a route returns when it refuses a request.
type ErrorResponse struct {
	Error string `json:"error"`
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

// DNSNamespaceState is the DNS protection state of one namespace.
type DNSNamespaceState struct {
	ID        string `json:"id"`
	Protected bool   `json:"protected"`
	Error     string `json:"error"`
}

// DNSResponse is the JSON response for GET /api/dns.
// Every field is explicit, because an embedded configuration struct returns the auth key
// of a tailnet to the client. HostResolvChangedAt holds an RFC 3339 time, and it holds an
// empty string when the daemon observes no change to the host resolv.conf file.
type DNSResponse struct {
	BindAddress         string              `json:"bind_address"`
	Mode                string              `json:"mode"`
	Upstreams           []string            `json:"upstreams"`
	HostResolvSHA256    string              `json:"host_resolv_sha256"`
	HostResolvChangedAt string              `json:"host_resolv_changed_at"`
	Namespaces          []DNSNamespaceState `json:"namespaces"`
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
