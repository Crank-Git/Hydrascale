package dns

import (
	"testing"
	"time"
)

func TestNewForwarder(t *testing.T) {
	f, err := NewForwarder([]string{"8.8.8.8", "8.8.4.4"}, 5*time.Second, "127.0.0.1:15353")
	if err != nil {
		t.Fatalf("NewForwarder: %v", err)
	}
	if f == nil {
		t.Fatal("NewForwarder returned nil")
	}
	if len(f.upstreams) != 2 {
		t.Fatalf("len(upstreams) = %d, want 2", len(f.upstreams))
	}
}

func TestNewForwarder_EmptyUpstreams(t *testing.T) {
	_, err := NewForwarder([]string{}, 5*time.Second, "")
	if err == nil {
		t.Fatal("expected error for empty upstreams")
	}
}

func TestNewForwarder_DefaultBind(t *testing.T) {
	f, err := NewForwarder([]string{"8.8.8.8"}, 5*time.Second, "")
	if err != nil {
		t.Fatalf("NewForwarder: %v", err)
	}
	if f.udpServer.Addr != DefaultBindAddress {
		t.Errorf("udpServer.Addr = %q, want %q", f.udpServer.Addr, DefaultBindAddress)
	}
	if f.tcpServer.Addr != DefaultBindAddress {
		t.Errorf("tcpServer.Addr = %q, want %q", f.tcpServer.Addr, DefaultBindAddress)
	}
}

func TestForwarder_StartStop(t *testing.T) {
	// Use a high port that doesn't require root
	f, err := NewForwarder([]string{"8.8.8.8"}, 5*time.Second, "127.0.0.1:15353")
	if err != nil {
		t.Fatalf("NewForwarder: %v", err)
	}

	if err := f.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Give servers time to bind
	time.Sleep(50 * time.Millisecond)

	if err := f.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestForwarder_SeparateUDPTCP(t *testing.T) {
	f, err := NewForwarder([]string{"8.8.8.8"}, 5*time.Second, "127.0.0.1:15354")
	if err != nil {
		t.Fatalf("NewForwarder: %v", err)
	}

	if f.udpServer.Net != "udp" {
		t.Errorf("udpServer.Net = %q, want %q", f.udpServer.Net, "udp")
	}
	if f.tcpServer.Net != "tcp" {
		t.Errorf("tcpServer.Net = %q, want %q", f.tcpServer.Net, "tcp")
	}
	if f.udpServer == f.tcpServer {
		t.Error("udpServer and tcpServer should be different objects")
	}
}

func TestStartResolver_UnsupportedMode(t *testing.T) {
	if err := StartResolver("unsupported", "127.0.0.1:15355"); err == nil {
		t.Fatal("expected error for unsupported resolver mode")
	}
}

func TestForwarder_RoundRobin_Atomic(t *testing.T) {
	f, err := NewForwarder([]string{"1.1.1.1", "8.8.8.8", "9.9.9.9"}, 5*time.Second, "")
	if err != nil {
		t.Fatalf("NewForwarder: %v", err)
	}

	// Verify atomic counter increments correctly
	initial := f.roundRobin.Load()
	if initial != 0 {
		t.Errorf("initial roundRobin = %d, want 0", initial)
	}

	// Simulate selections
	for i := 0; i < 10; i++ {
		idx := f.roundRobin.Add(1) - 1
		_ = f.upstreams[int(idx)%len(f.upstreams)]
	}

	if got := f.roundRobin.Load(); got != 10 {
		t.Errorf("roundRobin after 10 adds = %d, want 10", got)
	}
}
