package reconciler

import (
	"os"
	"strings"
	"testing"

	"hydrascale/internal/config"
	"hydrascale/internal/daemon"
)

func countUnprotectedEvents(events []Event, tailnetID string) []Event {
	var found []Event
	for _, e := range events {
		if e.Type == "dns.unprotected" && e.TailnetID == tailnetID {
			found = append(found, e)
		}
	}
	return found
}

func TestReconcile_records_dns_unprotected_and_places_the_tailnet_in_an_error_state(t *testing.T) {
	cfgPath := writeTestConfig(t, "alpha")
	ns := newMockNS()
	dm := newMockDaemon()
	dm.unprotected["alpha"] = daemon.UnprotectedRecord{
		Reason: "overlay /etc failed: invalid argument",
	}
	r := newTestReconciler(cfgPath, ns, dm, newMockRouting())

	if err := r.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	events := countUnprotectedEvents(r.Events(), "alpha")
	if len(events) != 1 {
		t.Fatalf("dns.unprotected events = %d, want 1", len(events))
	}
	if !strings.Contains(events[0].Message, "invalid argument") {
		t.Errorf("event message = %q, want it to hold the mount error text", events[0].Message)
	}
	if !r.ErrorStates()["alpha"] {
		t.Error("error state for alpha = false, want true")
	}
	if !strings.Contains(r.LastErrors()["alpha"], "invalid argument") {
		t.Errorf("last error = %q, want it to hold the mount error text", r.LastErrors()["alpha"])
	}
}

func TestReconcile_records_dns_unprotected_one_time_for_one_failure(t *testing.T) {
	cfgPath := writeTestConfig(t, "alpha")
	dm := newMockDaemon()
	dm.unprotected["alpha"] = daemon.UnprotectedRecord{Reason: "overlay /etc failed: no such device"}
	r := newTestReconciler(cfgPath, newMockNS(), dm, newMockRouting())

	r.Reconcile()
	r.Reconcile()
	r.Reconcile()

	events := countUnprotectedEvents(r.Events(), "alpha")
	if len(events) != 1 {
		t.Errorf("dns.unprotected events = %d, want 1", len(events))
	}
}

func TestReconcile_records_dns_unprotected_and_starts_the_tailnet_when_allow_unprotected_is_true(t *testing.T) {
	cfgPath := writeTestConfig(t, "alpha")
	dm := newMockDaemon()
	dm.unprotected["alpha"] = daemon.UnprotectedRecord{
		Reason:  "overlay /etc failed: no such device",
		Allowed: true,
	}
	r := newTestReconciler(cfgPath, newMockNS(), dm, newMockRouting())

	if err := r.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	events := countUnprotectedEvents(r.Events(), "alpha")
	if len(events) != 1 {
		t.Fatalf("dns.unprotected events = %d, want 1", len(events))
	}
	if r.ErrorStates()["alpha"] {
		t.Error("error state for alpha = true, want false when dns.allow_unprotected is true")
	}
}

func TestReconcile_records_no_event_when_the_overlay_mount_holds(t *testing.T) {
	cfgPath := writeTestConfig(t, "alpha")
	r := newTestReconciler(cfgPath, newMockNS(), newMockDaemon(), newMockRouting())

	if err := r.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if events := countUnprotectedEvents(r.Events(), "alpha"); len(events) != 0 {
		t.Errorf("dns.unprotected events = %d, want 0", len(events))
	}
}

func TestStartDaemon_passes_dns_allow_unprotected_from_the_configuration(t *testing.T) {
	cfgPath := writeTestConfig(t, "alpha")
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	cfg.DNS.AllowUnprotected = true
	if err := config.SaveConfig(cfgPath, cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	dm := newMockDaemon()
	r := newTestReconciler(cfgPath, newMockNS(), dm, newMockRouting())
	if err := r.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	dm.mu.Lock()
	defer dm.mu.Unlock()
	if !dm.allowedStart["alpha"] {
		t.Error("Start received allowUnprotected = false, want true")
	}
}

func TestReadUnprotected_reads_what_the_helper_wrote(t *testing.T) {
	path := t.TempDir() + "/dns-unprotected"
	if err := os.WriteFile(path, []byte(`{"reason":"overlay /etc failed: invalid argument","allowed":false}`), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	rec, ok := daemon.ReadUnprotected(path)
	if !ok {
		t.Fatal("ReadUnprotected returned false, want true")
	}
	if rec.Reason != "overlay /etc failed: invalid argument" {
		t.Errorf("reason = %q", rec.Reason)
	}
	if rec.Allowed {
		t.Error("allowed = true, want false")
	}
}

func TestReadUnprotected_reports_no_record_when_the_file_is_absent(t *testing.T) {
	if _, ok := daemon.ReadUnprotected(t.TempDir() + "/absent"); ok {
		t.Error("ReadUnprotected returned true, want false")
	}
}
