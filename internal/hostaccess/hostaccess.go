package hostaccess

import (
	"errors"
	"fmt"
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

	if err := m.syncDNS(); err != nil {
		log.Printf("host-access: %v", err)
	}
}

// Teardown removes all host access state for a tailnet.
// A step that fails does not stop the remaining steps. Teardown collects every failure and
// returns the failures together. The reconciler calls Teardown when it removes a tailnet,
// because the host resolves a name of that tailnet until the DNS sync runs again.
func (m *Manager) Teardown(tailnetID string) error {
	m.mu.Lock()
	peers, exists := m.activeTailnets[tailnetID]
	if exists {
		delete(m.activeTailnets, tailnetID)
	}
	m.mu.Unlock()

	var errs []error
	if exists {
		if err := RemoveAllHostRoutes(peers.VethHost, m.infraSubnet); err != nil {
			errs = append(errs, fmt.Errorf("remove the host routes of %s: %w", tailnetID, err))
		}
	}
	if err := m.syncDNS(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// TeardownAll removes all host access state. Called during shutdown.
// A step that fails does not stop the remaining steps. TeardownAll collects every failure
// and returns the failures together.
func (m *Manager) TeardownAll() error {
	m.mu.Lock()
	tailnets := make(map[string]TailnetPeers, len(m.activeTailnets))
	for k, v := range m.activeTailnets {
		tailnets[k] = v
	}
	m.activeTailnets = make(map[string]TailnetPeers)
	m.mu.Unlock()

	var errs []error
	for id, peers := range tailnets {
		if err := RemoveAllHostRoutes(peers.VethHost, m.infraSubnet); err != nil {
			errs = append(errs, fmt.Errorf("remove the host routes of %s: %w", id, err))
		}
	}

	if err := m.syncDNS(); err != nil {
		errs = append(errs, err)
	}

	if m.resolved != nil {
		if err := m.resolved.DeregisterAll(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// syncDNS writes the names of every active tailnet where the host resolver reads them.
// syncDNS returns the failure of the hosts file write or of the resolved registration.
func (m *Manager) syncDNS() error {
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
				domainRoutes[peers.MagicDNSSuffix] = peers.VethGateway
			}
		}
	}
	fwd := m.forwarder
	m.mu.Unlock()

	var err error
	switch m.dnsMode {
	case "hosts":
		if e := UpdateHostsFile(m.hostsPath, allV4, allV6); e != nil {
			err = fmt.Errorf("host-access: failed to update hosts file: %w", e)
		}
	case "resolved":
		if m.resolved != nil {
			if e := m.resolved.RegisterDomains(domains); e != nil {
				err = fmt.Errorf("host-access: resolved registration failed: %w", e)
			}
		}
	}

	if fwd != nil {
		fwd.SetDomainRoutes(domainRoutes)
	}
	return err
}
