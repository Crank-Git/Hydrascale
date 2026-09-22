package dns

import (
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"

	"github.com/miekg/dns"
)

// AliasDomainSuffix is the parent domain of every alias zone.
// ICANN reserved `.internal` for a private name in 2024, therefore a name under this
// suffix never collides with a name of the public domain name system.
const AliasDomainSuffix = "ts.internal"

// aliasRecordTTL is the time to live, in seconds, of an address record that the forwarder
// answers from an alias zone. The reconcile cycle rebuilds the zone every ten seconds, so
// a resolver that holds an answer for longer than that reports a peer that moved.
const aliasRecordTTL = 30

// aliasListenPort is the port on which the forwarder answers on a veth address.
// The loopback listener uses another port, because systemd-resolved holds 127.0.0.53:53.
// A veth address carries no other service, therefore the alias listener uses port 53 and
// `resolvectl dns <device> <address>` needs no port.
// A test replaces the port, because a test account binds no port below 1024.
var aliasListenPort = 53

// AliasZone holds the address records of one tailnet, keyed by the short host name of a
// peer. V4 holds one IPv4 address per name and V6 holds one IPv6 address per name.
type AliasZone struct {
	V4 map[string]string
	V6 map[string]string
}

// AliasZoneName returns the zone of a tailnet alias, for example "mmo.ts.internal".
func AliasZoneName(alias string) string {
	return strings.ToLower(alias) + "." + AliasDomainSuffix
}

// listenerPair holds the two servers that answer on one address.
type listenerPair struct {
	udp *dns.Server
	tcp *dns.Server
}

// SetAliasZones replaces the alias zones that the forwarder answers.
// Each key is a zone name, for example "mmo.ts.internal", and each value holds the
// address records of that tailnet. The forwarder answers a query of a zone from this
// table alone, and it sends no such query to an upstream server.
func (f *Forwarder) SetAliasZones(zones map[string]AliasZone) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.aliasZones = zones
}

// matchAliasZone returns the zone and the host name of a query name.
// The third result is false when no zone holds the name.
// The host name holds every label below the zone. A MagicDNS name carries more than one
// label below the suffix of the tailnet on some control servers, and the alias zone keeps
// that shape, therefore `a.b.mmo.ts.internal` names the peer `a.b`. See BuildAliasRecords
// in internal/hostaccess/peers.go.
// The apex of the zone names no peer, therefore matchAliasZone reports an empty host name
// for it.
func matchAliasZone(zones map[string]AliasZone, qname string) (AliasZone, string, bool) {
	name := strings.ToLower(strings.TrimSuffix(qname, "."))
	for zone, records := range zones {
		if name == zone {
			return records, "", true
		}
		if strings.HasSuffix(name, "."+zone) {
			return records, name[:len(name)-len(zone)-1], true
		}
	}
	return AliasZone{}, "", false
}

// answerAlias writes the answer of an alias zone query.
// answerAlias answers a name that the zone holds with the address record that the query
// asks for. It answers NXDOMAIN for a name that the zone does not hold, and it answers
// NOERROR with no record for a name that the zone holds with no record of that type.
// answerAlias is authoritative, because the forwarder owns the zone.
func (f *Forwarder) answerAlias(w dns.ResponseWriter, r *dns.Msg, zone AliasZone, host string, q dns.Question) {
	msg := new(dns.Msg)
	msg.SetReply(r)
	msg.Authoritative = true

	v4, hasV4 := zone.V4[host]
	v6, hasV6 := zone.V6[host]

	switch {
	case host == "" || (!hasV4 && !hasV6):
		// The apex holds no address, and an unknown host name does not exist.
		if host != "" {
			msg.Rcode = dns.RcodeNameError
		}
	case q.Qtype == dns.TypeA && hasV4:
		msg.Answer = append(msg.Answer, &dns.A{
			Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: aliasRecordTTL},
			A:   net.ParseIP(v4),
		})
	case q.Qtype == dns.TypeAAAA && hasV6:
		msg.Answer = append(msg.Answer, &dns.AAAA{
			Hdr:  dns.RR_Header{Name: q.Name, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: aliasRecordTTL},
			AAAA: net.ParseIP(v6),
		})
	}

	if err := w.WriteMsg(msg); err != nil {
		log.Printf("dns: write error: %v", err)
	}
}

// SyncListeners makes the forwarder answer on each address of addrs, and on no other
// veth address. SyncListeners starts a listener for an address that holds none, and it
// stops the listener of an address that addrs does not name.
// Each listener answers on port 53 of that address, on UDP and on TCP.
// SyncListeners returns the addresses that hold a running listener. An address that fails
// to bind is absent from that set, and the caller then registers the namespace side
// address with systemd-resolved rather than an address that answers nothing.
// SyncListeners is idempotent, therefore the reconcile cycle calls it on each tick. It
// retries an address that failed to bind on the next tick, because it records no listener
// for such an address.
// SyncListeners continues after a failure, and it returns the failures together.
func (f *Forwarder) SyncListeners(addrs []string) (map[string]bool, error) {
	want := make(map[string]bool, len(addrs))
	for _, a := range addrs {
		if a != "" {
			want[a] = true
		}
	}

	f.listenerMu.Lock()
	defer f.listenerMu.Unlock()
	if f.listeners == nil {
		f.listeners = make(map[string]*listenerPair)
	}

	var errs []error

	// The reconcile cycle calls SyncListeners on each tick, therefore the address set of
	// the host follows a veth pair that the daemon adds or removes.
	held, err := readHostAddrs()
	if err != nil {
		errs = append(errs, err)
	} else {
		f.hostAddrs = held
	}

	for addr, pair := range f.listeners {
		if want[addr] {
			continue
		}
		errs = append(errs, pair.stop(addr)...)
		delete(f.listeners, addr)
	}

	for addr := range want {
		if _, held := f.listeners[addr]; held {
			continue
		}
		pair, err := f.startListener(addr)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		f.listeners[addr] = pair
	}

	live := make(map[string]bool, len(f.listeners))
	for addr := range f.listeners {
		live[addr] = true
	}
	return live, errors.Join(errs...)
}

// startListener starts the UDP server and the TCP server of one address.
// startListener opens each socket before it returns, therefore a bind failure reaches the
// caller rather than the log alone. A port 53 that another resolver of the host already
// holds is the failure that this reports.
// startListener returns no server when one of the two sockets fails, and it closes the
// socket that did open.
func (f *Forwarder) startListener(addr string) (*listenerPair, error) {
	if net.ParseIP(addr) == nil {
		return nil, fmt.Errorf("the alias listener address %q is not an IP address", addr)
	}
	bind := net.JoinHostPort(addr, fmt.Sprint(aliasListenPort))

	mux := dns.NewServeMux()
	mux.HandleFunc(".", f.vethHandler(addr))

	packet, err := net.ListenPacket("udp", bind)
	if err != nil {
		return nil, fmt.Errorf("open the UDP listener on %s: %w", bind, err)
	}
	stream, err := net.Listen("tcp", bind)
	if err != nil {
		if e := packet.Close(); e != nil {
			log.Printf("dns: close the UDP listener on %s: %v", bind, e)
		}
		return nil, fmt.Errorf("open the TCP listener on %s: %w", bind, err)
	}

	pair := &listenerPair{
		udp: &dns.Server{PacketConn: packet, Handler: mux},
		tcp: &dns.Server{Listener: stream, Handler: mux},
	}

	// The socket is open, therefore ActivateAndServe fails only at shutdown.
	go func() {
		if err := pair.udp.ActivateAndServe(); err != nil && !errors.Is(err, net.ErrClosed) {
			log.Printf("dns: UDP listener on %s: %v", bind, err)
		}
	}()
	go func() {
		if err := pair.tcp.ActivateAndServe(); err != nil && !errors.Is(err, net.ErrClosed) {
			log.Printf("dns: TCP listener on %s: %v", bind, err)
		}
	}()

	return pair, nil
}

// vethHandler returns the handler of the listener on the veth address listen.
// The veth address carries the traffic of the namespace as well as the traffic of the
// host, therefore a process inside the namespace, and a tailnet peer that the namespace
// forwards, can reach this port. The forwarder answers the alias zone of every tailnet
// and it forwards every other name to the upstream resolvers of the host, so an answer to
// such a source gives one tailnet the names of another, and it gives a third party a
// resolver to send traffic through.
// The handler therefore answers the host alone. The host holds the veth address of each
// tailnet, and the namespace holds the address at the other end of that pair, which the
// host does not hold. sourceIsHost separates the two.
// A refusal reaches the log with the source that it refused, because a refusal that
// states nothing cannot be diagnosed from a journal.
func (f *Forwarder) vethHandler(listen string) dns.HandlerFunc {
	return func(w dns.ResponseWriter, r *dns.Msg) {
		if !f.sourceIsHost(w.RemoteAddr()) {
			log.Printf("dns: the listener on %s refused a query from %v, which is no address of the host",
				listen, w.RemoteAddr())
			msg := new(dns.Msg)
			msg.SetRcode(r, dns.RcodeRefused)
			if err := w.WriteMsg(msg); err != nil {
				log.Printf("dns: write error: %v", err)
			}
			return
		}
		f.handleDNSRequest(w, r)
	}
}

// sourceIsHost reports whether remote is an address that the host holds.
// A loopback address is the host. Every other address must appear on an interface of the
// host, which the veth address of a tailnet does and the address inside the namespace
// does not.
// sourceIsHost reports false when the address cannot be read, because an unreadable
// source is not the host.
func (f *Forwarder) sourceIsHost(remote net.Addr) bool {
	if remote == nil {
		return false
	}
	host, _, err := net.SplitHostPort(remote.String())
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	f.listenerMu.Lock()
	held := f.hostAddrs[ip.String()]
	f.listenerMu.Unlock()
	return held
}

// readHostAddrs returns every unicast address that an interface of the host holds.
// The list holds the veth address of each tailnet, because the host holds one end of each
// veth pair. It holds no address of a namespace, because a namespace holds its own
// network stack.
func readHostAddrs() (map[string]bool, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, fmt.Errorf("read the addresses of the host: %w", err)
	}
	held := make(map[string]bool, len(addrs))
	for _, a := range addrs {
		switch v := a.(type) {
		case *net.IPNet:
			held[v.IP.String()] = true
		case *net.IPAddr:
			held[v.IP.String()] = true
		}
	}
	return held, nil
}

// stop shuts the two servers of one address down and returns the failures.
func (p *listenerPair) stop(addr string) []error {
	var errs []error
	if err := p.udp.Shutdown(); err != nil {
		errs = append(errs, fmt.Errorf("stop the UDP listener on %s: %w", addr, err))
	}
	if err := p.tcp.Shutdown(); err != nil {
		errs = append(errs, fmt.Errorf("stop the TCP listener on %s: %w", addr, err))
	}
	return errs
}

// stopAllListeners stops every veth listener. Stop calls it at shutdown.
func (f *Forwarder) stopAllListeners() error {
	f.listenerMu.Lock()
	defer f.listenerMu.Unlock()

	var errs []error
	for addr, pair := range f.listeners {
		errs = append(errs, pair.stop(addr)...)
		delete(f.listeners, addr)
	}
	return errors.Join(errs...)
}

// listenerMutex guards the listener table. It is a separate lock from the mutex that
// guards the routing tables, because a listener start holds the lock across a bind.
type listenerMutex = sync.Mutex
