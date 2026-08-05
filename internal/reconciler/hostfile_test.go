package reconciler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hydrascale/internal/dns"
)

// hostFileEvents returns the dns.host_file_changed events of the reconciler.
func hostFileEvents(r *Reconciler) []Event {
	var out []Event
	for _, e := range r.Events() {
		if e.Type == "dns.host_file_changed" {
			out = append(out, e)
		}
	}
	return out
}

func TestReconcileHostFileCheck(t *testing.T) {
	t.Run("records one event when the host file changes once", func(t *testing.T) {
		hostPath := filepath.Join(t.TempDir(), "resolv.conf")
		if err := os.WriteFile(hostPath, []byte("nameserver 127.0.0.53\nsearch internal.example.com\n"), 0o644); err != nil {
			t.Fatalf("write host file: %v", err)
		}

		cfgPath := writeTestConfig(t, "alpha")
		r := newTestReconciler(cfgPath, newMockNS(), newMockDaemon(), newMockRouting())
		r.SetHostFileMonitor(dns.NewHostFileMonitor(hostPath))

		if err := r.Reconcile(); err != nil {
			t.Fatalf("first Reconcile: %v", err)
		}
		if got := len(hostFileEvents(r)); got != 0 {
			t.Fatalf("events after the first tick = %d, want 0", got)
		}

		if err := os.WriteFile(hostPath, []byte("nameserver 100.100.100.100\nsearch internal.example.com\n"), 0o644); err != nil {
			t.Fatalf("rewrite host file: %v", err)
		}

		if err := r.Reconcile(); err != nil {
			t.Fatalf("second Reconcile: %v", err)
		}
		if err := r.Reconcile(); err != nil {
			t.Fatalf("third Reconcile: %v", err)
		}

		events := hostFileEvents(r)
		if len(events) != 1 {
			t.Fatalf("events after the change = %d, want exactly 1", len(events))
		}

		msg := events[0].Message
		if !strings.Contains(msg, "nameserver 127.0.0.53") {
			t.Errorf("message %q holds no previous first line", msg)
		}
		if !strings.Contains(msg, "nameserver 100.100.100.100") {
			t.Errorf("message %q holds no current first line", msg)
		}
		if strings.Contains(msg, "search internal.example.com") {
			t.Errorf("message %q holds a line other than the first line", msg)
		}
	})

	t.Run("records no event when the host file does not change", func(t *testing.T) {
		hostPath := filepath.Join(t.TempDir(), "resolv.conf")
		if err := os.WriteFile(hostPath, []byte("nameserver 127.0.0.53\n"), 0o644); err != nil {
			t.Fatalf("write host file: %v", err)
		}

		cfgPath := writeTestConfig(t, "alpha")
		r := newTestReconciler(cfgPath, newMockNS(), newMockDaemon(), newMockRouting())
		r.SetHostFileMonitor(dns.NewHostFileMonitor(hostPath))

		for i := 0; i < 3; i++ {
			if err := r.Reconcile(); err != nil {
				t.Fatalf("Reconcile %d: %v", i+1, err)
			}
		}
		if got := len(hostFileEvents(r)); got != 0 {
			t.Errorf("events = %d, want 0", got)
		}
	})

	t.Run("records no event when the host file is missing", func(t *testing.T) {
		hostPath := filepath.Join(t.TempDir(), "resolv.conf")

		cfgPath := writeTestConfig(t, "alpha")
		r := newTestReconciler(cfgPath, newMockNS(), newMockDaemon(), newMockRouting())
		r.SetHostFileMonitor(dns.NewHostFileMonitor(hostPath))

		if err := r.Reconcile(); err != nil {
			t.Fatalf("first Reconcile: %v", err)
		}
		if err := r.Reconcile(); err != nil {
			t.Fatalf("second Reconcile: %v", err)
		}
		if got := len(hostFileEvents(r)); got != 0 {
			t.Errorf("events = %d, want 0", got)
		}

		state, _ := r.HostFileMonitor().State()
		if !state.Missing {
			t.Error("Missing = false, want true for a file that does not exist")
		}
		if state.Checksum != "" {
			t.Errorf("Checksum = %q, want an empty checksum", state.Checksum)
		}
	})
}
