package main

// DataSource supplies the GUI with data. Two implementations:
//   - mockSource: built-in fixtures, so the app runs standalone (no daemon).
//   - socketSource: real HTTP over the daemon's unix socket (added with the
//     SSH-tunnel transport). Selected at startup by whether a socket is set.
type DataSource interface {
	Dashboard() (Dashboard, error)
	TailnetDetail(id string) (TailnetDetail, error)
}

// mockSource returns the same fixtures the design mockup used, so the window
// renders a realistic dashboard with no daemon attached.
type mockSource struct{}

func (mockSource) Dashboard() (Dashboard, error) {
	return Dashboard{
		Host:    "mars",
		Healthy: 3,
		DNSOK:   true,
		Version: "v0.8.2",
		Metrics: Metrics{
			Tailnets: 4, Reconnecting: 1, Peers: 14,
			HostAccessOn: 3, ReconcileSec: 10, Uptime: "2:59",
		},
		Tailnets: []TailnetRow{
			{ID: "jbones", Namespace: "mars-1", Status: "connected", Address: "100.72.247.43", Peers: 6, HostAccess: "enabled"},
			{ID: "havoc", Namespace: "mars-2", Status: "connected", Address: "100.121.171.43", Peers: 5, HostAccess: "enabled"},
			{ID: "homelab", Namespace: "mars-4", Status: "connected", Address: "100.98.12.7", Peers: 3, HostAccess: "off"},
			{ID: "corp-prod", Namespace: "mars-3", Status: "reconnecting", Address: "reauthenticating…", Peers: 0, HostAccess: "enabled", ExitNode: "us-nyc"},
		},
		Events: []Event{
			{Time: "16:47:22", Kind: "ok", Message: "reconcile complete · 2 actions"},
			{Time: "16:47:20", Kind: "ok", Tailnet: "havoc", Message: "authenticated mars-2"},
			{Time: "16:47:19", Kind: "dns", Message: "host DNS synced · 14 peers"},
			{Time: "16:47:18", Kind: "skip", Tailnet: "jbones", Message: "route 192.168.1.0/24 would clobber host LAN"},
			{Time: "16:47:18", Kind: "route", Tailnet: "jbones", Message: "+ 100.114.149.115"},
			{Time: "16:47:11", Kind: "ns", Tailnet: "corp-prod", Message: "namespace created"},
			{Time: "16:47:04", Kind: "boot", Message: "daemon start · reconcile 10s"},
		},
	}, nil
}

func (mockSource) TailnetDetail(id string) (TailnetDetail, error) {
	return TailnetDetail{
		ID: "jbones", Namespace: "ns-jbones", Status: "connected",
		Address: "100.72.247.43", PeerCount: 6, Uptime: "2:41",
		Network: []KV{
			{Key: "Tailscale IPv4", Value: "100.72.247.43"},
			{Key: "Tailscale IPv6", Value: "fd7a:1c…:a3f2", Dim: true},
			{Key: "MagicDNS name", Value: "jbones.ts.net"},
			{Key: "Control", Value: "login.tailscale.com"},
		},
		DNS: []KV{
			{Key: "Upstream", Value: "100.100.100.100"},
			{Key: "Search domain", Value: "jbones.ts.net"},
			{Key: "Host resolv", Value: "synced"},
			{Key: "Split routes", Value: "1"},
		},
		HostAccess: []KV{
			{Key: "veth", Value: "10.88.1.1/30"},
			{Key: "Allowed LAN", Value: "192.168.1.0/24"},
			{Key: "NAT", Value: "masquerade"},
			{Key: "Guard", Value: "host-route safe"},
		},
		Routes: []KV{
			{Key: "Advertised", Value: "—", Dim: true},
			{Key: "Accepted", Value: "100.114.149.115"},
			{Key: "Subnet", Value: "none", Dim: true},
			{Key: "Exit node", Value: "—", Dim: true},
		},
		Peers: []Peer{
			{HostName: "orin-brain", Address: "100.114.149.115", OS: "linux", Routes: "100.114.149.115/32", LastSeen: "now", Status: "good"},
			{HostName: "phobos", Address: "100.72.19.4", OS: "linux", LastSeen: "now", Status: "good"},
			{HostName: "pixel-7", Address: "100.88.203.51", OS: "android", LastSeen: "now", Status: "good"},
			{HostName: "mbp-work", Address: "100.102.7.19", OS: "macOS", LastSeen: "3m", Status: "good"},
			{HostName: "nas-attic", Address: "100.66.140.8", OS: "linux", Routes: "192.168.1.0/24", LastSeen: "12m", Status: "warn"},
			{HostName: "old-laptop", Address: "100.75.11.2", OS: "windows", LastSeen: "2d", Status: "dim"},
		},
		Events: []Event{
			{Time: "16:47:18", Kind: "route", Message: "+ 100.114.149.115"},
			{Time: "16:47:18", Kind: "skip", Message: "route 192.168.1.0/24 would clobber host LAN"},
			{Time: "16:44:02", Kind: "dns", Message: "MagicDNS synced · 6 peers"},
			{Time: "16:44:01", Kind: "ok", Message: "authenticated mars-1"},
			{Time: "16:44:00", Kind: "ns", Message: "namespace ns-jbones up"},
		},
	}, nil
}
