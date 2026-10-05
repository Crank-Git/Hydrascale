package reconciler

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"hydrascale/internal/namespaces"
)

// fakeIPv6Host reports fixed IPv6 facts and records each write in the shared call log.
type fakeIPv6Host struct {
	host  namespaces.IPv6Host
	log   *callLog
	paths []string
	force []bool
}

func (f *fakeIPv6Host) ReadIPv6Host() (namespaces.IPv6Host, error) { return f.host, nil }

func (f *fakeIPv6Host) EnableForceForwarding(upstreams []string) ([]string, error) {
	f.log.add("force_forwarding " + strings.Join(upstreams, ","))
	return upstreams, nil
}

func (f *fakeIPv6Host) DisableForceForwarding(upstreams []string) error {
	f.log.add("reset force_forwarding " + strings.Join(upstreams, ","))
	return nil
}

func (f *fakeIPv6Host) EnableAllForwarding() ([]string, error) {
	f.log.add("all.forwarding")
	return []string{"enp1s0f0"}, nil
}

func (f *fakeIPv6Host) EnsureIPv6Path(nsName string, index int, forceForwarding bool) ([]string, error) {
	f.log.add("path " + nsName)
	f.paths = append(f.paths, nsName)
	f.force = append(f.force, forceForwarding)
	return nil, nil
}

// ipv6Fixture returns a Reconciler with one running namespace, an IPv4 chain writer, an
// IPv6 chain writer, and a fake IPv6 host. Every double writes into one call log.
func ipv6Fixture(t *testing.T, body string, host namespaces.IPv6Host) (*Reconciler, *fakeChainWriter, *fakeIPv6Host, *callLog) {
	t.Helper()

	cfgPath := writeAccessConfig(t, body, "alpha")
	ns := newMockNS()
	ns.namespaces[ns.GetName("alpha")] = true
	r := newTestReconciler(cfgPath, ns, newMockDaemon(), newMockRouting())

	log := &callLog{}
	r.SetChainWriter(&fakeChainWriter{})
	w6 := &fakeChainWriter{log: log}
	h := &fakeIPv6Host{host: host, log: log}
	r.access6 = w6
	r.ipv6 = h
	return r, w6, h, log
}

// upstreamHost returns a host with one IPv6 upstream device, one global prefix, and no forwarding.
func upstreamHost(forceForwarding bool) namespaces.IPv6Host {
	return namespaces.IPv6Host{
		Upstreams:       []string{"enp1s0f0"},
		Prefixes:        []string{"2001:db8:1:2::/64"},
		ForceForwarding: forceForwarding,
	}
}

// eventMessages returns the message of each event of the type.
func eventMessages(r *Reconciler, eventType string) []string {
	var out []string
	for _, e := range r.Events() {
		if e.Type == eventType {
			out = append(out, e.Message)
		}
	}
	return out
}

func TestTheIPv6PathOpensWithForceForwardingAfterTheGuardedChainsExist(t *testing.T) {
	r, w6, h, log := ipv6Fixture(t, "access:\n  mode: enforce\n", upstreamHost(true))

	if err := r.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	want := []string{"access.write", "force_forwarding enp1s0f0", "path ns-alpha"}
	if got := log.recorded(); !slices.Equal(got, want) {
		t.Errorf("the order of the IPv6 writes = %v, want %v", got, want)
	}
	if len(w6.applied) != 1 {
		t.Fatalf("the IPv6 chains were written %d times, want 1", len(w6.applied))
	}
	guard := lines(w6.applied[0].Guard)
	if !slices.Equal(guard, []string{"-A HYDRASCALE-FWD -i enp1s0f0 ! -o vh+ -j DROP"}) {
		t.Errorf("the guard = %v", guard)
	}
	if !slices.Equal(h.force, []bool{true}) {
		t.Errorf("EnsureIPv6Path received forceForwarding %v, want [true]", h.force)
	}
}

func TestTheIPv6ChainsHoldNoGuardOnAHostThatAlreadyForwards(t *testing.T) {
	host := upstreamHost(true)
	host.Forwarding = true
	r, w6, _, _ := ipv6Fixture(t, "access:\n  mode: enforce\n", host)

	if err := r.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if len(w6.applied) != 1 || len(w6.applied[0].Guard) != 0 {
		t.Errorf("the IPv6 chains hold a guard on a host that forwards on every device: %+v", w6.applied)
	}
}

func TestAnOldKernelGetsNoIPv6PathWithoutTheIPv6Key(t *testing.T) {
	r, w6, h, log := ipv6Fixture(t, "access:\n  mode: enforce\n", upstreamHost(false))

	for i := 0; i < 2; i++ {
		if err := r.Reconcile(); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
	}

	if len(w6.applied) != 0 || len(h.paths) != 0 || len(log.recorded()) != 0 {
		t.Errorf("the daemon wrote an IPv6 path without the ipv6 key: %v", log.recorded())
	}
	states := eventMessages(r, "ipv6.state")
	if len(states) != 1 || !strings.Contains(states[0], "ipv6: true") {
		t.Errorf("ipv6.state events = %v, want one that names the ipv6 key", states)
	}
}

func TestAnOldKernelGetsAllForwardingWithTheIPv6Key(t *testing.T) {
	r, w6, h, log := ipv6Fixture(t, "ipv6: true\naccess:\n  mode: enforce\n", upstreamHost(false))

	if err := r.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	want := []string{"access.write", "all.forwarding", "path ns-alpha"}
	if got := log.recorded(); !slices.Equal(got, want) {
		t.Errorf("the order of the IPv6 writes = %v, want %v", got, want)
	}
	if len(w6.applied[0].Guard) != 0 {
		t.Errorf("the all.forwarding mode wrote a guard: %v", w6.applied[0].Guard)
	}
	if !slices.Equal(h.force, []bool{false}) {
		t.Errorf("EnsureIPv6Path received forceForwarding %v, want [false]", h.force)
	}
	if msgs := eventMessages(r, "ipv6.forwarding"); len(msgs) != 1 || !strings.Contains(msgs[0], "accept_ra from 1 to 2 on enp1s0f0") {
		t.Errorf("ipv6.forwarding events = %v", msgs)
	}
}

func TestAHostWithoutAnIPv6DefaultRouteGetsNoIPv6Path(t *testing.T) {
	r, _, _, log := ipv6Fixture(t, "access:\n  mode: enforce\n", namespaces.IPv6Host{ForceForwarding: true})

	if err := r.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if len(log.recorded()) != 0 {
		t.Errorf("the daemon wrote %v on a host with no IPv6 default route", log.recorded())
	}
	if states := eventMessages(r, "ipv6.state"); len(states) != 1 || !strings.Contains(states[0], "no IPv6 default route") {
		t.Errorf("ipv6.state events = %v", states)
	}
}

func TestAFailedIPv6ChainWriteOpensNoForwarding(t *testing.T) {
	r, w6, _, log := ipv6Fixture(t, "access:\n  mode: enforce\n", upstreamHost(true))
	w6.err = errors.New("ip6tables-restore --noflush: exit status 2")

	if err := r.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got := log.recorded(); !slices.Equal(got, []string{"access.write"}) {
		t.Errorf("the daemon ran %v after a failed IPv6 chain write, want the write alone", got)
	}
}

func TestShutdownResetsTheUpstreamDeviceBeforeItRemovesTheIPv6Chains(t *testing.T) {
	r, w6, _, log := ipv6Fixture(t, "access:\n  mode: enforce\n", upstreamHost(true))
	if err := r.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if err := r.Shutdown(); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	got := log.recorded()
	reset := slices.Index(got, "reset force_forwarding enp1s0f0")
	if reset < 0 {
		t.Fatalf("Shutdown did not reset the upstream device: %v", got)
	}
	if w6.teardown != 1 {
		t.Errorf("Shutdown removed the IPv6 chains %d times, want 1", w6.teardown)
	}
}

func TestTheGuardKeepsAnEarlierUpstreamDeviceAfterTheDefaultRouteMoves(t *testing.T) {
	r, w6, h, _ := ipv6Fixture(t, "access:\n  mode: enforce\n", upstreamHost(true))
	if err := r.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	h.host.Upstreams = []string{"wlan0"}
	if err := r.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	guard := lines(w6.applied[len(w6.applied)-1].Guard)
	want := []string{
		"-A HYDRASCALE-FWD -i enp1s0f0 ! -o vh+ -j DROP",
		"-A HYDRASCALE-FWD -i wlan0 ! -o vh+ -j DROP",
	}
	if !slices.Equal(guard, want) {
		t.Errorf("the guard = %v, want %v", guard, want)
	}
}

func TestAccessDiffReportsTheIPv6Chains(t *testing.T) {
	r, w6, _, _ := ipv6Fixture(t, "access:\n  mode: enforce\n", upstreamHost(true))
	w6.diffs = []string{"write chain HYDRASCALE-FWD: the host holds no chain or no marker rule"}

	diffs, err := r.AccessDiff()
	if err != nil {
		t.Fatalf("AccessDiff: %v", err)
	}
	if !slices.Contains(diffs, "IPv6: write chain HYDRASCALE-FWD: the host holds no chain or no marker rule") {
		t.Errorf("AccessDiff = %v, want the IPv6 difference", diffs)
	}
}

// lines returns each rule as one line.
func lines(rules [][]string) []string {
	out := make([]string, 0, len(rules))
	for _, r := range rules {
		out = append(out, strings.Join(r, " "))
	}
	return out
}

// failingResetHost is a fake IPv6 host whose upstream device reset fails.
type failingResetHost struct{ fakeIPv6Host }

func (f *failingResetHost) DisableForceForwarding(upstreams []string) error {
	return errors.New("sysctl: permission denied")
}

func TestShutdownRemovesBothChainsWhenTheUpstreamDeviceResetFails(t *testing.T) {
	r, w6, _, log := ipv6Fixture(t, "access:\n  mode: enforce\n", upstreamHost(true))
	if err := r.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	w4 := &fakeChainWriter{}
	r.SetChainWriter(w4)
	r.ipv6 = &failingResetHost{fakeIPv6Host{host: upstreamHost(true), log: log}}

	if err := r.Shutdown(); err == nil {
		t.Error("Shutdown returned no error for a failed upstream device reset")
	}
	if w6.teardown != 1 || w4.teardown != 1 {
		t.Errorf("Shutdown removed the IPv6 chains %d times and the IPv4 chains %d times, want 1 and 1", w6.teardown, w4.teardown)
	}
}
