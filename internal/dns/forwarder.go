package dns

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
)

// DefaultBindAddress is the default DNS forwarder listen address.
// Uses a non-standard port to avoid conflicting with systemd-resolved on port 53.
const DefaultBindAddress = "127.0.0.53:5354"

// Forwarder represents a DNS forwarder that forwards queries to multiple upstream servers.
// It supports domain-based routing: queries matching a domain suffix are forwarded to
// a specific upstream (e.g., via a veth pair into a Tailscale namespace), while all
// other queries go to the host's original DNS servers via round-robin.
type Forwarder struct {
	mu           sync.RWMutex
	upstreams    []string          // host DNS servers (round-robin fallback)
	domainRoutes map[string]string // domain suffix -> upstream address (e.g., "tail1234.ts.net" -> "100.100.100.100")
	hostDNS      []string          // original host DNS servers from /etc/resolv.conf

	client     *dns.Client
	udpServer  *dns.Server
	tcpServer  *dns.Server
	roundRobin atomic.Int64
	timeout    time.Duration
	context    context.Context
	cancelFunc context.CancelFunc
}

// NewForwarder creates a new DNS forwarder with the given upstream servers.
func NewForwarder(upstreams []string, timeout time.Duration, bindAddr string) (*Forwarder, error) {
	if len(upstreams) == 0 {
		return nil, errors.New("at least one upstream DNS server must be specified")
	}

	if bindAddr == "" {
		bindAddr = DefaultBindAddress
	}

	ctx, cancel := context.WithCancel(context.Background())

	f := &Forwarder{
		upstreams:    upstreams,
		hostDNS:      upstreams, // default: host DNS = provided upstreams
		domainRoutes: make(map[string]string),
		client:       &dns.Client{Timeout: timeout},
		timeout:      timeout,
		context:      ctx,
		cancelFunc:   cancel,
	}

	mux := dns.NewServeMux()
	mux.HandleFunc(".", f.handleDNSRequest)

	f.udpServer = &dns.Server{
		Addr:    bindAddr,
		Net:     "udp",
		Handler: mux,
	}
	f.tcpServer = &dns.Server{
		Addr:    bindAddr,
		Net:     "tcp",
		Handler: mux,
	}

	return f, nil
}

// NewForwarderFromResolvConf creates a new DNS forwarder that reads the host's
// /etc/resolv.conf to discover default upstream DNS servers. Domain routes can
// be added later via SetDomainRoutes.
func NewForwarderFromResolvConf(timeout time.Duration, bindAddr string) (*Forwarder, error) {
	hostServers, err := ParseResolvConf("/etc/resolv.conf")
	if err != nil {
		return nil, fmt.Errorf("failed to parse /etc/resolv.conf: %w", err)
	}
	if len(hostServers) == 0 {
		// Fall back to well-known public DNS
		hostServers = []string{"8.8.8.8", "8.8.4.4"}
	}

	f, err := NewForwarder(hostServers, timeout, bindAddr)
	if err != nil {
		return nil, err
	}
	f.hostDNS = hostServers
	return f, nil
}

// Start begins the DNS forwarder service on both UDP and TCP.
func (f *Forwarder) Start() error {
	go func() {
		if err := f.udpServer.ListenAndServe(); err != nil && !errors.Is(err, net.ErrClosed) {
			log.Printf("DNS UDP server error: %v", err)
		}
	}()

	go func() {
		if err := f.tcpServer.ListenAndServe(); err != nil && !errors.Is(err, net.ErrClosed) {
			log.Printf("DNS TCP server error: %v", err)
		}
	}()

	return nil
}

// Stop gracefully stops the DNS forwarder.
func (f *Forwarder) Stop() error {
	f.cancelFunc()
	f.udpServer.Shutdown()
	f.tcpServer.Shutdown()
	return nil
}

// SetUpstreams hot-swaps the upstream DNS servers used for non-domain-routed queries.
func (f *Forwarder) SetUpstreams(upstreams []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.upstreams = upstreams
}

// SetDomainRoutes replaces the domain routing table.
// Each key is a domain suffix (e.g., "tail1234.ts.net") and each value
// is the upstream DNS address to use for queries matching that suffix
// (e.g., "100.100.100.100").
func (f *Forwarder) SetDomainRoutes(routes map[string]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.domainRoutes = routes
}

// handleDNSRequest handles incoming DNS requests and forwards them to upstream servers.
// It first checks if the query domain matches any domain route suffix. If so, it
// forwards to that route's upstream. Otherwise, it falls back to host DNS round-robin.
func (f *Forwarder) handleDNSRequest(w dns.ResponseWriter, r *dns.Msg) {
	f.mu.RLock()
	upstreams := f.upstreams
	routes := f.domainRoutes
	f.mu.RUnlock()

	if len(upstreams) == 0 {
		msg := new(dns.Msg)
		msg.SetReply(r)
		msg.Rcode = dns.RcodeServerFailure
		if err := w.WriteMsg(msg); err != nil {
			log.Printf("dns: write error: %v", err)
		}
		return
	}

	// Determine upstream based on domain routing
	var upstream string
	if len(r.Question) > 0 {
		qname := strings.TrimSuffix(r.Question[0].Name, ".")
		qname = strings.ToLower(qname)
		for suffix, addr := range routes {
			if strings.HasSuffix(qname, suffix) || qname == suffix {
				upstream = addr
				break
			}
		}
	}

	if upstream == "" {
		// No domain route match: round-robin across host DNS
		idx := f.roundRobin.Add(1) - 1
		upstream = upstreams[int(idx)%len(upstreams)]
	}

	resp, _, err := f.client.Exchange(r, upstream+":53")
	if err != nil {
		log.Printf("DNS forward error to %s: %v", upstream, err)
		msg := new(dns.Msg)
		msg.SetReply(r)
		msg.Rcode = dns.RcodeServerFailure
		if err := w.WriteMsg(msg); err != nil {
			log.Printf("dns: write error: %v", err)
		}
		return
	}

	if err := w.WriteMsg(resp); err != nil {
		log.Printf("dns: write error: %v", err)
	}
}

// ParseResolvConf reads a resolv.conf file and returns the nameserver addresses.
func ParseResolvConf(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var servers []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "nameserver" {
			servers = append(servers, fields[1])
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return servers, nil
}

// StartResolver starts the DNS resolver in unified mode.
func StartResolver(mode string, bindAddr string) error {
	if mode != "unified" {
		return fmt.Errorf("unsupported resolver mode: %s", mode)
	}

	forwarder, err := NewForwarderFromResolvConf(5*time.Second, bindAddr)
	if err != nil {
		// Fall back to hardcoded upstreams if resolv.conf parsing fails
		forwarder, err = NewForwarder([]string{"8.8.8.8", "8.8.4.4"}, 5*time.Second, bindAddr)
		if err != nil {
			return err
		}
	}

	return forwarder.Start()
}
