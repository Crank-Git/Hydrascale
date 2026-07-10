package main

// View-model DTOs for the GUI. Intentionally decoupled from hydrascale's
// internal/api types — those import the Linux-only daemon package, so importing
// them would stop the GUI compiling on macOS/Windows. The GUI is a remote
// client: it only needs the JSON contract, and the Go backend pre-formats data
// into these display-ready shapes so the frontend stays dumb.

// AddTailnetRequest is what the wizard submits to create a tailnet.
type AddTailnetRequest struct {
	ID         string `json:"id"`
	AuthKey    string `json:"authKey"`
	ControlURL string `json:"controlUrl"`
	ExitNode   string `json:"exitNode"`
	HostAccess bool   `json:"hostAccess"`
}

// Metrics is the summary strip on the dashboard.
type Metrics struct {
	Tailnets     int    `json:"tailnets"`
	Reconnecting int    `json:"reconnecting"`
	Peers        int    `json:"peers"`
	HostAccessOn int    `json:"hostAccessOn"`
	ReconcileSec int    `json:"reconcileSec"`
	Uptime       string `json:"uptime"`
}

// TailnetRow is one row in the dashboard table.
type TailnetRow struct {
	ID         string `json:"id"`
	Namespace  string `json:"namespace"`
	Status     string `json:"status"` // connected | reconnecting | off
	Address    string `json:"address"`
	Peers      int    `json:"peers"`
	HostAccess string `json:"hostAccess"` // enabled | off
	ExitNode   string `json:"exitNode"`   // "" renders as em dash
}

// Event is one line in the activity feed.
type Event struct {
	Time    string `json:"time"`
	Kind    string `json:"kind"` // ok | dns | skip | route | ns | boot
	Tailnet string `json:"tailnet"`
	Message string `json:"message"`
}

// Dashboard is the whole list view, assembled by the backend in one call.
type Dashboard struct {
	Host     string       `json:"host"`
	Healthy  int          `json:"healthy"`
	DNSOK    bool         `json:"dnsOk"`
	Version  string       `json:"version"`
	Metrics  Metrics      `json:"metrics"`
	Tailnets []TailnetRow `json:"tailnets"`
	Events   []Event      `json:"events"`
}

// Peer is one row in a tailnet's peers table.
type Peer struct {
	HostName string `json:"hostName"`
	Address  string `json:"address"`
	OS       string `json:"os"`
	Routes   string `json:"routes"` // "" renders as em dash
	LastSeen string `json:"lastSeen"`
	Status   string `json:"status"` // good | warn | dim
}

// KV is a labelled value inside a detail config card.
type KV struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Dim   bool   `json:"dim"`
}

// TailnetDetail backs the detail view.
type TailnetDetail struct {
	ID         string  `json:"id"`
	Namespace  string  `json:"namespace"`
	Status     string  `json:"status"`
	Address    string  `json:"address"`
	PeerCount  int     `json:"peerCount"`
	Uptime     string  `json:"uptime"`
	Network    []KV    `json:"network"`
	DNS        []KV    `json:"dns"`
	HostAccess []KV    `json:"hostAccess"`
	Routes     []KV    `json:"routes"`
	Peers      []Peer  `json:"peers"`
	Events     []Event `json:"events"`
	Error      string  `json:"error,omitempty"`
}
