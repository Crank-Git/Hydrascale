package hostaccess

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"

	"hydrascale/internal/daemon"
	"hydrascale/internal/dns"
	"hydrascale/internal/execx"
)

// DNSForwarder routes DNS queries by domain suffix to per-tailnet upstreams.
// Implemented by *dns.Forwarder; defined here to avoid an import cycle.
type DNSForwarder interface {
	SetDomainRoutes(routes map[string]string)

	// SetAliasZones replaces the zones that the forwarder answers itself.
	SetAliasZones(zones map[string]dns.AliasZone)

	// SyncListeners makes the forwarder answer on each veth address of the list, and it
	// returns the addresses on which it answers now.
	SyncListeners(addrs []string) (map[string]bool, error)
}

// Manager coordinates host access features: routes, DNS, and namespace setup.
type Manager struct {
	// Runner runs every command that the Manager sends to the host. A test replaces
	// Runner with an execx.Recorder and asserts the exact argument list.
	Runner execx.Runner

	mu          sync.Mutex
	dnsMode     string // "hosts" or "resolved"
	hostsPath   string
	infraSubnet string
	// routeTable is the routing table that holds every route the Manager writes on the
	// host, which the configuration key `route_table` declares. The value 0 means that the
	// Manager writes into the main table and owns no routing policy rule.
	routeTable int
	resolved   *ResolvedManager
	forwarder  DNSForwarder

	// resolveAliases holds the value of resolver.resolve_aliases. With the value false,
	// syncDNS writes exactly the registration that the daemon writes without the key.
	resolveAliases bool

	// aliases holds the alias of each tailnet, keyed by the tailnet ID. A tailnet that
	// holds no alias is absent, and it gets no alias zone.
	aliases map[string]string

	// Track which tailnets have been synced so teardown knows what to clean up
	activeTailnets map[string]TailnetPeers
}

// NewManager creates a new host access Manager.
//
// routeTable is the value of the configuration key `route_table`. The value 0 means that
// the Manager writes every host route into the main table and owns no routing policy rule.
func NewManager(dnsMode string, hostsPath string, infraSubnet string, routeTable int) *Manager {
	if hostsPath == "" {
		hostsPath = "/etc/hosts"
	}
	if infraSubnet == "" {
		infraSubnet = "10.200.0.0/16"
	}
	m := &Manager{
		Runner:         execx.OSRunner{},
		dnsMode:        dnsMode,
		hostsPath:      hostsPath,
		infraSubnet:    infraSubnet,
		routeTable:     routeTable,
		activeTailnets: make(map[string]TailnetPeers),
	}
	if dnsMode == "resolved" {
		m.resolved = NewResolvedManager()
	}
	return m
}

// runner returns the command runner. A Manager with no Runner runs on the host.
func (m *Manager) runner() execx.Runner {
	if m.Runner == nil {
		return execx.OSRunner{}
	}
	return m.Runner
}

// run runs one command on the host and returns its combined output. Every command carries
// a deadline, because a command that does not return blocks the reconcile cycle.
func (m *Manager) run(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), hostCommandTimeout)
	defer cancel()
	return m.runner().Run(ctx, name, args...)
}

// SetForwarder wires a DNS forwarder for per-tailnet MagicDNS routing.
// If never called, syncDNS skips forwarder updates and existing DNS behaviour
// (hosts/resolved modes) is unaffected.
func (m *Manager) SetForwarder(f DNSForwarder) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.forwarder = f
}

// SetAliasResolution records the value of resolver.resolve_aliases and the alias of each
// tailnet, keyed by the tailnet ID.
// The reconciler calls SetAliasResolution on each cycle, because the operator adds a
// tailnet and changes an alias while the daemon runs.
func (m *Manager) SetAliasResolution(enabled bool, aliases map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resolveAliases = enabled
	m.aliases = aliases
}

// Sync updates host routes and DNS for a tailnet's peers.
func (m *Manager) Sync(tailnetID string, status *daemon.TailscaleStatus, vethGW, vethHost, vethHostIP, nsName string) {
	peers := ParsePeers(tailnetID, status, vethGW, vethHost, vethHostIP, nsName)

	if len(peers.Peers) == 0 && status == nil {
		return
	}

	m.mu.Lock()
	m.activeTailnets[tailnetID] = peers
	m.mu.Unlock()

	if err := m.SyncHostRoutes(peers); err != nil {
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
		if err := m.RemoveAllHostRoutes(peers.VethHost); err != nil {
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
		if err := m.RemoveAllHostRoutes(peers.VethHost); err != nil {
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

	// The daemon owns the routing policy rule and the route table, therefore the last
	// teardown removes both. A tailnet teardown removes the routes of that tailnet alone.
	if err := m.RemoveHostRouteTable(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// syncDNS writes the names of every active tailnet where the host resolver reads them.
// syncDNS starts the alias listeners before it registers a domain with systemd-resolved,
// because a link that names an address on which nothing answers loses the names of that
// tailnet. See SyncListeners in internal/dns/alias.go.
// syncDNS returns the failure of the hosts file write or of the resolved registration.
func (m *Manager) syncDNS() error {
	m.mu.Lock()
	allV4 := make(map[string]string)
	allV6 := make(map[string]string)
	domainRoutes := make(map[string]string)

	aliasZones := make(map[string]dns.AliasZone)
	var listenAddrs []string
	registrations := make([]TailnetPeers, 0, len(m.activeTailnets))

	for _, peers := range m.activeTailnets {
		v4, v6 := BuildDNSRecords(peers.TailnetID, peers.Peers)
		for k, v := range v4 {
			allV4[k] = v
		}
		for k, v := range v6 {
			allV6[k] = v
		}

		// The alias zone needs the alias of the tailnet and the host side address, on
		// which the forwarder answers.
		alias := m.aliases[peers.TailnetID]
		zoneOn := m.resolveAliases && alias != "" && peers.VethHostIP != ""
		if zoneOn {
			zv4, zv6 := BuildAliasRecords(peers.Peers)
			aliasZones[dns.AliasZoneName(alias)] = dns.AliasZone{V4: zv4, V6: zv6}
			listenAddrs = append(listenAddrs, peers.VethHostIP)
		}

		if peers.MagicDNSSuffix != "" && peers.VethGateway != "" {
			domainRoutes[peers.MagicDNSSuffix] = peers.VethGateway
		}
		// systemd-resolved registers the domains on the veth device of the tailnet,
		// because it refuses a per-link domain on the loopback device. A tailnet that
		// holds an alias zone registers the device even when the control server serves
		// no MagicDNS suffix, so the alias zone answers on such a tailnet as well.
		if peers.VethHost != "" && (zoneOn || (peers.MagicDNSSuffix != "" && peers.VethGateway != "")) {
			registrations = append(registrations, peers)
		}
	}
	fwd := m.forwarder
	resolveAliases := m.resolveAliases
	aliasOf := make(map[string]string, len(m.aliases))
	for k, v := range m.aliases {
		aliasOf[k] = v
	}
	m.mu.Unlock()

	var err error

	// The listeners start first, and live names every address that answers now. A link
	// that names an address with no listener loses the MagicDNS suffix of that tailnet as
	// well as the alias zone, therefore such a link keeps the namespace side address.
	live := map[string]bool{}
	if fwd != nil {
		fwd.SetDomainRoutes(domainRoutes)
		// The zones go in before the listeners start, so a listener never answers a
		// query of a zone that the forwarder does not hold yet.
		fwd.SetAliasZones(aliasZones)
		var e error
		if live, e = fwd.SyncListeners(listenAddrs); e != nil {
			err = errors.Join(err, fmt.Errorf("host-access: alias listener sync failed: %w", e))
		}
	}

	links := make([]Link, 0, len(registrations))
	for _, peers := range registrations {
		// A link carries one server for every domain that it holds. With an alias zone
		// the link therefore names the host side address, and the forwarder sends the
		// MagicDNS suffix onward to the namespace. Without one the link names the
		// namespace side address, which is the registration that the daemon writes
		// without resolver.resolve_aliases.
		alias := aliasOf[peers.TailnetID]
		zoneUp := resolveAliases && alias != "" && peers.VethHostIP != "" && live[peers.VethHostIP]
		address := peers.VethGateway
		var domains []string
		if peers.MagicDNSSuffix != "" && peers.VethGateway != "" {
			domains = append(domains, peers.MagicDNSSuffix)
		}
		if zoneUp {
			address = peers.VethHostIP
			domains = append(domains, dns.AliasZoneName(alias))
		}
		if address == "" || len(domains) == 0 {
			continue
		}
		links = append(links, Link{Device: peers.VethHost, Address: address, Domains: domains})
	}

	switch m.dnsMode {
	case "hosts":
		if e := UpdateHostsFile(m.hostsPath, allV4, allV6); e != nil {
			err = errors.Join(err, fmt.Errorf("host-access: failed to update hosts file: %w", e))
		}
	case "resolved":
		if m.resolved != nil {
			if e := m.resolved.RegisterDomains(links); e != nil {
				err = errors.Join(err, fmt.Errorf("host-access: resolved registration failed: %w", e))
			}
		}
	}
	return err
}
