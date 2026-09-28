package hostaccess

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"

	"hydrascale/internal/daemon"
	"hydrascale/internal/dns"
	"hydrascale/internal/execx"
)

// SplitDNSEventConflict is the event that the daemon records when two tailnets claim one
// split DNS domain. The losing tailnet names the event, and the message names the winner.
const SplitDNSEventConflict = "dns.split_domain_conflict"

// SplitDNSEntry holds the split DNS domains that one tailnet holds on the host, and the
// conflict that removed a domain from it. Domains is empty when the tailnet holds none or
// when every domain was dropped. The console reads this type through SplitDNSReport.
type SplitDNSEntry struct {
	TailnetID string
	Domains   []string
	Conflict  string
}

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

	// splitReport holds the surviving split DNS domains and the conflict of each tailnet,
	// keyed by the tailnet ID. The console reads it through SplitDNSReport.
	splitReport map[string]SplitDNSEntry

	// eventRecorder records one event for each new split DNS conflict. It is nil until the
	// reconciler wires one with SetEventRecorder.
	eventRecorder func(kind, tailnetID, message string)

	// reportedConflicts holds the message of every split DNS conflict that the daemon
	// reported. syncDNS emits one event for a message that is absent from the set and it
	// drops a message that is gone, so a repeat reports again and a steady state reports
	// no event on every tick. See FR-split-8.
	reportedConflicts map[string]bool
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
		Runner:            execx.OSRunner{},
		dnsMode:           dnsMode,
		hostsPath:         hostsPath,
		infraSubnet:       infraSubnet,
		routeTable:        routeTable,
		activeTailnets:    make(map[string]TailnetPeers),
		splitReport:       make(map[string]SplitDNSEntry),
		reportedConflicts: make(map[string]bool),
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

// SetEventRecorder wires the recorder of split DNS conflict events. The reconciler calls it
// once, because the Manager records the event through the event log of the reconciler.
func (m *Manager) SetEventRecorder(fn func(kind, tailnetID, message string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.eventRecorder = fn
}

// SplitDNSReport returns the surviving split DNS domains and the conflict of each tailnet,
// sorted by tailnet ID. The console reads it through GET /api/dns.
func (m *Manager) SplitDNSReport() []SplitDNSEntry {
	m.mu.Lock()
	defer m.mu.Unlock()

	ids := make([]string, 0, len(m.splitReport))
	for id := range m.splitReport {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	report := make([]SplitDNSEntry, 0, len(ids))
	for _, id := range ids {
		entry := m.splitReport[id]
		entry.Domains = append([]string(nil), entry.Domains...)
		report = append(report, entry)
	}
	return report
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

// normalizeSplitDomain returns the normalized form of a split DNS domain, and false when
// the domain is empty or is not a DNS name. The conflict rule compares the normalized form,
// therefore a value that differs in case or in a trailing dot resolves to one domain.
func normalizeSplitDomain(raw string) (string, bool) {
	domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(raw), "."))
	if !validDNSName(domain) {
		return "", false
	}
	return domain, true
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

	// The conflict rule needs a fixed order, because map iteration is not deterministic.
	// The first tailnet in sorted order keeps a domain that another tailnet also holds.
	// See FR-split-4.
	ids := make([]string, 0, len(m.activeTailnets))
	for id := range m.activeTailnets {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	// Pass 1 claims every MagicDNS suffix and every active alias zone name, so that a
	// MagicDNS suffix and an alias zone always beat a split domain. See FR-split-4.
	owner := make(map[string]string)
	for _, id := range ids {
		peers := m.activeTailnets[id]
		if peers.MagicDNSSuffix != "" {
			if suffix, ok := normalizeSplitDomain(peers.MagicDNSSuffix); ok {
				if _, taken := owner[suffix]; !taken {
					owner[suffix] = id
				}
			}
		}
		alias := m.aliases[id]
		if m.resolveAliases && alias != "" && peers.VethHostIP != "" {
			if _, taken := owner[dns.AliasZoneName(alias)]; !taken {
				owner[dns.AliasZoneName(alias)] = id
			}
		}
	}

	// surviving holds the split domains that each tailnet keeps, and currentConflicts
	// holds the message of every conflict, keyed by the message, with the losing tailnet.
	surviving := make(map[string][]string)
	currentConflicts := make(map[string]string)
	type conflictEvent struct{ tailnetID, message string }
	var newConflicts []conflictEvent

	for _, id := range ids {
		peers := m.activeTailnets[id]
		entry := SplitDNSEntry{TailnetID: id}
		seen := make(map[string]bool)
		for _, raw := range peers.SplitDNSDomains {
			domain, ok := normalizeSplitDomain(raw)
			if !ok {
				// A value that is not a DNS name never reaches resolvectl. See SA-19.
				log.Printf("host-access: split DNS domain %q of %s is not a DNS name; dropped", raw, id)
				continue
			}
			if domain == "ts.net" || strings.HasSuffix(domain, ".ts.net") {
				// ts.net is the reserved base zone of MagicDNS. One tailnet holds no claim on the
				// zone: the MagicDNS suffix of every tailnet already reaches its own resolver, and
				// an export of the zone would send the names of every tailnet to one of them.
				log.Printf("host-access: split DNS domain %q of %s names the reserved ts.net zone; dropped", domain, id)
				continue
			}
			if seen[domain] {
				continue
			}
			seen[domain] = true
			if ownerID, taken := owner[domain]; taken {
				message := fmt.Sprintf("the split DNS domain %s of %s is claimed by %s", domain, id, ownerID)
				if entry.Conflict == "" {
					entry.Conflict = message
				} else {
					entry.Conflict = entry.Conflict + "; " + message
				}
				currentConflicts[message] = id
				log.Printf("host-access: %s", message)
				continue
			}
			owner[domain] = id
			entry.Domains = append(entry.Domains, domain)
		}
		surviving[id] = entry.Domains
		m.splitReport[id] = entry
	}

	// The report holds every active tailnet, and a tailnet that left the map leaves the
	// report as well.
	for id := range m.splitReport {
		if _, active := m.activeTailnets[id]; !active {
			delete(m.splitReport, id)
		}
	}

	// One event for each conflict that the previous tick did not hold. The set replaces
	// the previous set, so a conflict that is gone reports again if it returns.
	for message, id := range currentConflicts {
		if !m.reportedConflicts[message] {
			newConflicts = append(newConflicts, conflictEvent{tailnetID: id, message: message})
		}
	}
	reported := make(map[string]bool, len(currentConflicts))
	for message := range currentConflicts {
		reported[message] = true
	}
	m.reportedConflicts = reported
	recorder := m.eventRecorder

	for _, peers := range m.activeTailnets {
		v4, v6 := BuildDNSRecords(peers.TailnetID, peers.Peers)
		for k, v := range v4 {
			allV4[k] = v
		}
		for k, v := range v6 {
			allV6[k] = v
		}
	}

	for _, id := range ids {
		peers := m.activeTailnets[id]

		// The alias zone needs the alias of the tailnet and the host side address, on
		// which the forwarder answers.
		alias := m.aliases[id]
		zoneOn := m.resolveAliases && alias != "" && peers.VethHostIP != ""
		if zoneOn {
			zv4, zv6 := BuildAliasRecords(peers.Peers)
			aliasZones[dns.AliasZoneName(alias)] = dns.AliasZone{V4: zv4, V6: zv6}
			listenAddrs = append(listenAddrs, peers.VethHostIP)
		}

		if peers.MagicDNSSuffix != "" && peers.VethGateway != "" {
			domainRoutes[peers.MagicDNSSuffix] = peers.VethGateway
		}
		// A split domain reaches the veth device of its tailnet, therefore a tailnet that
		// holds one registers even when it holds no MagicDNS suffix and no alias zone.
		// See FR-split-3.
		hasSplit := len(surviving[id]) > 0 && peers.VethGateway != ""
		for _, domain := range surviving[id] {
			if peers.VethGateway != "" {
				domainRoutes[domain] = peers.VethGateway
			}
		}
		// systemd-resolved registers the domains on the veth device of the tailnet,
		// because it refuses a per-link domain on the loopback device. A tailnet that
		// holds an alias zone registers the device even when the control server serves
		// no MagicDNS suffix, so the alias zone answers on such a tailnet as well.
		if peers.VethHost != "" && (zoneOn || (peers.MagicDNSSuffix != "" && peers.VethGateway != "") || hasSplit) {
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

	// The event reaches the log outside the lock, because the recorder writes the log.
	sort.Slice(newConflicts, func(i, j int) bool { return newConflicts[i].message < newConflicts[j].message })
	if recorder != nil {
		for _, conflict := range newConflicts {
			recorder(SplitDNSEventConflict, conflict.tailnetID, conflict.message)
		}
	}

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
		// The order is: MagicDNS suffix, alias zone, split DNS domains.
		var domains []string
		if peers.MagicDNSSuffix != "" && peers.VethGateway != "" {
			domains = append(domains, peers.MagicDNSSuffix)
		}
		if zoneUp {
			address = peers.VethHostIP
			domains = append(domains, dns.AliasZoneName(alias))
		}
		// Every surviving split domain reaches the same server as the MagicDNS suffix.
		if peers.VethGateway != "" {
			domains = append(domains, surviving[peers.TailnetID]...)
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
