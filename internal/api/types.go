// Package api provides the Unix socket HTTP API for Hydrascale.
package api

import (
	"hydrascale/internal/config"
	"hydrascale/internal/reconciler"
)

// DefaultSocketPath is the default Unix socket path for the API server.
const DefaultSocketPath = "/var/lib/hydrascale/api.sock"

// StatusResponse is the JSON response for GET /api/status.
type StatusResponse struct {
	Desired       map[string]config.Tailnet        `json:"desired"`
	Actual        map[string]*reconciler.TailnetState `json:"actual"`
	ErrorStates   map[string]bool                  `json:"error_states"`
	PausedStates  map[string]bool                  `json:"paused_states"`
	FailureCounts map[string]int                   `json:"failure_counts"`
	LastErrors    map[string]string                `json:"last_errors"`
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
	ID       string `json:"id"`
	AuthKey  string `json:"auth_key,omitempty"`
	ExitNode string `json:"exit_node,omitempty"`
}

// DNSConfigRequest is the request body for POST /api/config/dns.
type DNSConfigRequest struct {
	Mode        string `json:"mode"`
	BindAddress string `json:"bind_address,omitempty"`
}

// RedactedTailnet is a Tailnet with the auth key hidden.
type RedactedTailnet struct {
	ID       string `json:"id"`
	ExitNode string `json:"exit_node,omitempty"`
	AuthKey  string `json:"auth_key,omitempty"`
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
