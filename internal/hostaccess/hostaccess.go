package hostaccess

import (
	"log"
	"sync"

	"hydrascale/internal/daemon"
)

// DNSForwarder routes DNS queries by domain suffix to per-tailnet upstreams.
// Implemented by *dns.Forwarder; defined here to avoid an import cycle.
type DNSForwarder interface {
	SetDomainRoutes(routes map[string]string)
}

// Manager coordinates host access features: routes, DNS, and namespace setup.
type Manager struct {
	mu          sync.Mutex
	dnsMode     string // "hosts" or "resolved"
	hostsPath   string
	infraSubnet string
	resolved    *ResolvedManager
	forwarder   DNSForwarder

	// Track which tailnets have been synced so teardown knows what to clean up
	activeTailnets map[string]TailnetPeers
}

// NewManager creates a new host access Manager.
func NewManager(dnsMode string, hostsPath string, infraSubnet string) *Manager {
	if hostsPath == "" {
		hostsPath = "/etc/hosts"
	}
	if infraSubnet == "" {
		infraSubnet = "10.200.0.0/16"
	}
	m := &Manager{
		dnsMode:        dnsMode,
		hostsPath:      hostsPath,
		infraSubnet:    infraSubnet,
		activeTailnets: make(map[string]TailnetPeers),
	}
	if dnsMode == "resolved" {
		m.resolved = NewResolvedManager()
	}
	return m
}

// SetForwarder wires a DNS forwarder for per-tailnet MagicDNS routing.
// If never called, syncDNS skips forwarder updates and existing DNS behaviour
// (hosts/resolved modes) is unaffected.
func (m *Manager) SetForwarder(f DNSForwarder) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.forwarder = f
}

// Sync updates host routes and DNS for a tailnet's peers.
func (m *Manager) Sync(tailnetID string, status *daemon.TailscaleStatus, vethGW, vethHost, nsName string) {
	peers := ParsePeers(tailnetID, status, vethGW, vethHost, nsName)

	if len(peers.Peers) == 0 && status == nil {
		return
	}

	m.mu.Lock()
	m.activeTailnets[tailnetID] = peers
	m.mu.Unlock()

	if err := SyncHostRoutes(peers, m.infraSubnet); err != nil {
		log.Printf("host-access: route sync failed for %s: %v", tailnetID, err)
	}

	m.syncDNS()
}

// Teardown removes all host access state for a tailnet.
func (m *Manager) Teardown(tailnetID string) {
	m.mu.Lock()
	peers, exists := m.activeTailnets[tailnetID]
	if exists {
		delete(m.activeTailnets, tailnetID)
	}
	m.mu.Unlock()

	if exists {
		RemoveAllHostRoutes(peers.VethHost, m.infraSubnet)
	}

	m.syncDNS()
}

// TeardownAll removes all host access state. Called during shutdown.
func (m *Manager) TeardownAll() {
	m.mu.Lock()
	tailnets := make(map[string]TailnetPeers, len(m.activeTailnets))
	for k, v := range m.activeTailnets {
		tailnets[k] = v
	}
	m.activeTailnets = make(map[string]TailnetPeers)
	m.mu.Unlock()

	for _, peers := range tailnets {
		RemoveAllHostRoutes(peers.VethHost, m.infraSubnet)
	}

	m.syncDNS()

	if m.resolved != nil {
		m.resolved.DeregisterAll()
	}
}

func (m *Manager) syncDNS() {
	m.mu.Lock()
	allV4 := make(map[string]string)
	allV6 := make(map[string]string)
	var domains []string
	domainRoutes := make(map[string]string)

	for _, peers := range m.activeTailnets {
		v4, v6 := BuildDNSRecords(peers.TailnetID, peers.Peers)
		for k, v := range v4 {
			allV4[k] = v
		}
		for k, v := range v6 {
			allV6[k] = v
		}
		if peers.MagicDNSSuffix != "" {
			domains = append(domains, peers.MagicDNSSuffix)
			if peers.VethGateway != "" {
				domainRoutes[peers.MagicDNSSuffix] = peers.VethGateway + ":53"
			}
		}
	}
	fwd := m.forwarder
	m.mu.Unlock()

	switch m.dnsMode {
	case "hosts":
		if err := UpdateHostsFile(m.hostsPath, allV4, allV6); err != nil {
			log.Printf("host-access: failed to update hosts file: %v", err)
		}
	case "resolved":
		if m.resolved != nil {
			if err := m.resolved.RegisterDomains(domains); err != nil {
				log.Printf("host-access: resolved registration failed: %v", err)
			}
		}
	}

	if fwd != nil {
		fwd.SetDomainRoutes(domainRoutes)
	}
}
