package hostaccess

import (
	"log"
	"sync"

	"hydrascale/internal/daemon"
)

// Manager coordinates host access features: routes, DNS, and namespace setup.
type Manager struct {
	mu        sync.Mutex
	dnsMode   string // "hosts" or "resolved"
	hostsPath string
	resolved  *ResolvedManager

	// Track which tailnets have been synced so teardown knows what to clean up
	activeTailnets map[string]TailnetPeers
}

// NewManager creates a new host access Manager.
func NewManager(dnsMode string, hostsPath string) *Manager {
	if hostsPath == "" {
		hostsPath = "/etc/hosts"
	}
	m := &Manager{
		dnsMode:        dnsMode,
		hostsPath:      hostsPath,
		activeTailnets: make(map[string]TailnetPeers),
	}
	if dnsMode == "resolved" {
		m.resolved = NewResolvedManager()
	}
	return m
}

// Sync updates host routes and DNS for a tailnet's peers.
func (m *Manager) Sync(tailnetID string, status *daemon.TailscaleStatus, vethGW, vethHost string) {
	peers := ParsePeers(tailnetID, status, vethGW, vethHost)

	if len(peers.Peers) == 0 && status == nil {
		return
	}

	m.mu.Lock()
	m.activeTailnets[tailnetID] = peers
	m.mu.Unlock()

	if err := SyncHostRoutes(peers); err != nil {
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
		RemoveAllHostRoutes(peers.VethHost)
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
		RemoveAllHostRoutes(peers.VethHost)
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
		}
	}
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
}
