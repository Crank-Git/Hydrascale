package reach

import (
	"context"
	"errors"
	"testing"
	"time"

	"hydrascale/internal/execx"
)

func TestTheProbeRunsPingInsideTheNamespace(t *testing.T) {
	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{Output: []byte("1 packets transmitted, 1 received")},
		"ip", "netns", "exec", "ns-havoc", "ping", "-n", "-c", "1", "-W", "1", "192.168.1.1")

	p := &Prober{Runner: rec}
	res := p.Probe(context.Background(), "ns-havoc", "192.168.1.1")

	if res.State != StateReachable {
		t.Errorf("the state is %q, want %q (detail: %s)", res.State, StateReachable, res.Detail)
	}
	if res.Target != "192.168.1.1" {
		t.Errorf("the target is %q, want 192.168.1.1", res.Target)
	}
	if res.CheckedAt.IsZero() {
		t.Error("the result carries no time of measurement")
	}
	calls := rec.Calls()
	if len(calls) != 1 {
		t.Fatalf("the probe ran %d commands, want 1: %v", len(calls), calls)
	}
	want := "ip netns exec ns-havoc ping -n -c 1 -W 1 192.168.1.1"
	if calls[0].String() != want {
		t.Errorf("the probe ran %q, want %q", calls[0].String(), want)
	}
}

func TestAFailedPingReportsUnreachableAndKeepsTheReasonOfTheFailure(t *testing.T) {
	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{
		Output: []byte("1 packets transmitted, 0 received, 100% packet loss, time 0ms"),
		Err:    errors.New("exit status 1"),
	}, "ip", "netns", "exec", "ns-havoc", "ping", "-n", "-c", "1", "-W", "1", "1.1.1.1")

	p := &Prober{Runner: rec}
	res := p.Probe(context.Background(), "ns-havoc", "1.1.1.1")

	if res.State != StateUnreachable {
		t.Errorf("the state is %q, want %q", res.State, StateUnreachable)
	}
	if res.Detail == "" {
		t.Error("the result carries no reason of the failure")
	}
}

func TestAnEmptyTargetMakesTheProbeUseTheDefaultGatewayOfTheHost(t *testing.T) {
	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{Output: []byte("default via 192.168.1.1 dev eth0 proto dhcp metric 100\n")},
		"ip", "-4", "route", "show", "default")
	rec.Script(execx.Result{Output: []byte("1 packets transmitted, 1 received")},
		"ip", "netns", "exec", "ns-jbones", "ping", "-n", "-c", "1", "-W", "1", "192.168.1.1")

	p := &Prober{Runner: rec}
	res := p.Probe(context.Background(), "ns-jbones", "")

	if res.State != StateReachable {
		t.Errorf("the state is %q, want %q (detail: %s)", res.State, StateReachable, res.Detail)
	}
	if res.Target != "192.168.1.1" {
		t.Errorf("the target is %q, want the default gateway 192.168.1.1", res.Target)
	}
}

func TestAHostWithNoDefaultRouteAndNoDeclaredTargetReportsNotProbed(t *testing.T) {
	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{Output: nil}, "ip", "-4", "route", "show", "default")

	p := &Prober{Runner: rec}
	res := p.Probe(context.Background(), "ns-jbones", "")

	if res.State != StateNotProbed {
		t.Errorf("the state is %q, want %q", res.State, StateNotProbed)
	}
	if res.Detail == "" {
		t.Error("the result states no reason for the absent measurement")
	}
}

func TestTheProbeGivesEveryCommandADeadline(t *testing.T) {
	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{Output: []byte("default via 10.0.0.1 dev eth0\n")},
		"ip", "-4", "route", "show", "default")
	rec.Script(execx.Result{Output: []byte("ok")},
		"ip", "netns", "exec", "ns-havoc", "ping", "-n", "-c", "1", "-W", "1", "10.0.0.1")

	p := &Prober{Runner: rec}
	p.Probe(context.Background(), "ns-havoc", "")

	for i, hasDeadline := range rec.Deadlines() {
		if !hasDeadline {
			t.Errorf("command %d ran with no deadline: %s", i, rec.Calls()[i])
		}
	}
}

func TestTheProbeTimeoutStaysWellInsideTheReconcilerTickBound(t *testing.T) {
	if Timeout >= time.Second {
		t.Fatalf("the probe timeout is %v, and the specification bounds the tick at 1 second", Timeout)
	}
}
