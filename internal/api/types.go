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

	// The settings view of the console shows these three values, and the console shows no
	// invented data, so the daemon reports the paths that it holds. ConsoleAddress is an
	// empty string for a daemon that opened no console listener.
	ConfigPath     string `json:"config_path,omitempty"`
	SocketPath     string `json:"socket_path,omitempty"`
	ConsoleAddress string `json:"console_address,omitempty"`
}

// AccessStatus is the access field of GET /api/status. It holds the mode that the daemon
// applies the local rule set in, the count of rules, and the position of the jump rule.
// JumpPosition is the position of the jump rule into HYDRASCALE-FWD in the FORWARD chain,
// as the last tick measured it. It counts from 1. It is 0 before the first tick, and for a
// FORWARD chain that held no jump rule on the last tick.
type AccessStatus struct {
	Mode         string `json:"mode"`
	Rules        int    `json:"rules"`
	JumpPosition int    `json:"jump_position"`
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
// AllowUnprotected holds the configuration key dns.allow_unprotected. An unprotected
// namespace is an error state only when the key is false, therefore a reader cannot state
// the state from Protected alone.
type DNSResponse struct {
	BindAddress         string              `json:"bind_address"`
	Mode                string              `json:"mode"`
	Upstreams           []string            `json:"upstreams"`
	AllowUnprotected    bool                `json:"allow_unprotected"`
	HostResolvPath      string              `json:"host_resolv_path"`
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
// BackendState holds the state word of tailscaled, and LoginURL holds the address that
// authorizes a node that is not logged in. The console shows a warning dot and the login
// URL for that node, therefore it reads both from the daemon.
type TailnetDetailResponse struct {
	TailscaleIPs   []string   `json:"tailscale_ips"`
	MagicDNSName   string     `json:"magic_dns_name,omitempty"`
	MagicDNSSuffix string     `json:"magic_dns_suffix,omitempty"`
	PeerCount      int        `json:"peer_count"`
	OnlinePeers    int        `json:"online_peers"`
	Peers          []PeerInfo `json:"peers,omitempty"`
	BackendState   string     `json:"backend_state,omitempty"`
	LoginURL       string     `json:"login_url,omitempty"`
	FetchedAt      time.Time  `json:"fetched_at"`
	Error          string     `json:"error,omitempty"`
}

// TailnetRemovalPlanResponse is the JSON response for GET /api/tailnet/{id}/removal-plan.
// It states what the removal of one tailnet does on this host, so that the console dialog
// of FR-console-29 names every command and repeats no rule of the daemon. The route reads
// state and it runs no command.
type TailnetRemovalPlanResponse struct {
	ID        string   `json:"id"`
	Namespace string   `json:"namespace"`
	HostVeth  string   `json:"host_veth"`
	StateDir  string   `json:"state_dir"`
	RuleCount int      `json:"rule_count"`
	Commands  []string `json:"commands"`
}
