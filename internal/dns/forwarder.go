package dns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
)

// DefaultBindAddress is the default DNS forwarder listen address.
// Uses a non-standard port to avoid conflicting with systemd-resolved on port 53.
const DefaultBindAddress = "127.0.0.53:5354"

// Forwarder represents a DNS forwarder that forwards queries to multiple upstream servers.
type Forwarder struct {
	upstreams  []string
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
		upstreams:  upstreams,
		timeout:    timeout,
		context:    ctx,
		cancelFunc: cancel,
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

// Start begins the DNS forwarder service on both UDP and TCP.
func (f *Forwarder) Start() error {
	go func() {
		if err := f.udpServer.ListenAndServe(); err != nil && !errors.Is(err, net.ErrClosed) {
			fmt.Printf("DNS UDP server error: %v\n", err)
		}
	}()

	go func() {
		if err := f.tcpServer.ListenAndServe(); err != nil && !errors.Is(err, net.ErrClosed) {
			fmt.Printf("DNS TCP server error: %v\n", err)
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

// handleDNSRequest handles incoming DNS requests and forwards them to upstream servers.
func (f *Forwarder) handleDNSRequest(w dns.ResponseWriter, r *dns.Msg) {
	if len(f.upstreams) == 0 {
		msg := new(dns.Msg)
		msg.SetReply(r)
		msg.Rcode = dns.RcodeServerFailure
		w.WriteMsg(msg)
		return
	}

	// Atomic round-robin selection (no lock needed)
	idx := f.roundRobin.Add(1) - 1
	upstream := f.upstreams[int(idx)%len(f.upstreams)]

	client := new(dns.Client)
	client.Timeout = f.timeout

	resp, _, err := client.Exchange(r, upstream+":53")
	if err != nil {
		fmt.Printf("DNS forward error to %s: %v\n", upstream, err)
		msg := new(dns.Msg)
		msg.SetReply(r)
		msg.Rcode = dns.RcodeServerFailure
		w.WriteMsg(msg)
		return
	}

	w.WriteMsg(resp)
}

// StartResolver starts the DNS resolver in unified mode.
func StartResolver(mode string, bindAddr string) error {
	if mode != "unified" {
		return fmt.Errorf("unsupported resolver mode: %s", mode)
	}

	upstreams := []string{"8.8.8.8", "8.8.4.4"}
	forwarder, err := NewForwarder(upstreams, 5*time.Second, bindAddr)
	if err != nil {
		return err
	}

	return forwarder.Start()
}
