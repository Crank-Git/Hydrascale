package dns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// Forwarder represents a DNS forwarder that can forward queries to multiple upstream servers.
type Forwarder struct {
	upstreams  []string
	mutex      sync.RWMutex
	server     *dns.Server
	roundRobin int
	timeout    time.Duration
	context    context.Context
	cancelFunc context.CancelFunc
	wg         sync.WaitGroup
}

// NewForwarder creates a new DNS forwarder with the given upstream servers.
func NewForwarder(upstreams []string, timeout time.Duration) (*Forwarder, error) {
	if len(upstreams) == 0 {
		return nil, errors.New("at least one upstream DNS server must be specified")
	}

	ctx, cancel := context.WithCancel(context.Background())

	f := &Forwarder{
		upstreams:  upstreams,
		timeout:    timeout,
		context:    ctx,
		cancelFunc: cancel,
		server: &dns.Server{
			Addr: ":53",
			Net:  "udp",
		},
	}

	dns.HandleFunc(".", f.handleDNSRequest)

	return f, nil
}

// Start begins the DNS forwarder service.
func (f *Forwarder) Start() error {
	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		if err := f.server.ListenAndServe(); err != nil && !errors.Is(err, net.ErrClosed) {
			fmt.Printf("DNS server error: %v\n", err)
		}
	}()

	// Also listen on TCP
	f.server.Net = "tcp"
	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		if err := f.server.ListenAndServe(); err != nil && !errors.Is(err, net.ErrClosed) {
			fmt.Printf("DNS TCP server error: %v\n", err)
		}
	}()

	return nil
}

// Stop gracefully stops the DNS forwarder.
func (f *Forwarder) Stop() error {
	f.cancelFunc()
	f.server.Shutdown()
	f.wg.Wait()
	return nil
}

// handleDNSRequest handles incoming DNS requests and forwards them to upstream servers.
func (f *Forwarder) handleDNSRequest(w dns.ResponseWriter, r *dns.Msg) {
	// Create a new message for the response
	msg := new(dns.Msg)
	msg.SetReply(r)
	msg.Authoritative = true

	// Select upstream using round-robin
	f.mutex.RLock()
	if len(f.upstreams) == 0 {
		f.mutex.RUnlock()
		msg.Rcode = dns.RcodeServerFailure
		w.WriteMsg(msg)
		return
	}

	upstream := f.upstreams[f.roundRobin%len(f.upstreams)]
	f.roundRobin++
	f.mutex.RUnlock()

	// Create a client to forward the request
	client := new(dns.Client)
	client.Timeout = f.timeout

	// Send the request to the upstream server
	resp, _, err := client.Exchange(r, upstream+":53")
	if err != nil {
		fmt.Printf("DNS forward error to %s: %v\n", upstream, err)
		msg.Rcode = dns.RcodeServerFailure
		w.WriteMsg(msg)
		return
	}

	// Write the response back to the client
	w.WriteMsg(resp)
}

// StartResolver starts the DNS resolver in unified mode.
func StartResolver(mode string) error {
	if mode != "unified" {
		return fmt.Errorf("unsupported resolver mode: %s", mode)
	}

	// For now, we'll use a simple implementation that reads from /etc/resolv.conf
	// In a full implementation, this would dynamically discover Tailscale DNS servers
	// from each namespace

	upstreams := []string{"8.8.8.8", "8.8.4.4"} // Fallback to Google DNS
	forwarder, err := NewForwarder(upstreams, 5*time.Second)
	if err != nil {
		return err
	}

	// Start the forwarder
	if err := forwarder.Start(); err != nil {
		return err
	}

	// In a real implementation, we'd keep the forwarder running and manage its lifecycle
	// For this example, we'll just return nil indicating the resolver is ready
	// The actual server runs in goroutines started by Start()

	return nil
}
