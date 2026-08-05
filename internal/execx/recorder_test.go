package execx

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// fakeReporter records the failure that a Recorder reports. A test uses it to assert a
// failure without a failure of the test itself.
type fakeReporter struct {
	failures []string
}

func (f *fakeReporter) Helper() {}

func (f *fakeReporter) Errorf(format string, args ...any) {
	f.failures = append(f.failures, fmt.Sprintf(format, args...))
}

func TestRecorderReturnsTheScriptedOutput(t *testing.T) {
	r := NewRecorder(t)
	r.Script(Result{Output: []byte("veth0 added\n")}, "ip", "link", "add", "veth0", "type", "veth")

	out, err := r.Run(context.Background(), "ip", "link", "add", "veth0", "type", "veth")
	if err != nil {
		t.Fatalf("Run returned the error %v, want no error", err)
	}
	if string(out) != "veth0 added\n" {
		t.Fatalf("Run returned %q, want %q", out, "veth0 added\n")
	}
}

func TestRecorderReturnsTheScriptedError(t *testing.T) {
	wantErr := errors.New("exit status 2")
	r := NewRecorder(t)
	r.Script(Result{Output: []byte("File exists\n"), Err: wantErr}, "ip", "link", "add", "veth0", "type", "veth")

	out, err := r.Run(context.Background(), "ip", "link", "add", "veth0", "type", "veth")
	if !errors.Is(err, wantErr) {
		t.Fatalf("Run returned the error %v, want %v", err, wantErr)
	}
	if string(out) != "File exists\n" {
		t.Fatalf("Run returned %q, want %q", out, "File exists\n")
	}
}

func TestRecorderRecordsTheExactArgumentList(t *testing.T) {
	r := NewRecorder(t)
	r.Script(Result{}, "ip", "netns", "add", "hs-work")
	r.Script(Result{}, "sysctl", "-w", "net.ipv4.ip_forward=1")

	if _, err := r.Run(context.Background(), "ip", "netns", "add", "hs-work"); err != nil {
		t.Fatalf("Run returned the error %v, want no error", err)
	}
	if _, err := r.Run(context.Background(), "sysctl", "-w", "net.ipv4.ip_forward=1"); err != nil {
		t.Fatalf("Run returned the error %v, want no error", err)
	}

	want := []Call{
		{Name: "ip", Args: []string{"netns", "add", "hs-work"}},
		{Name: "sysctl", Args: []string{"-w", "net.ipv4.ip_forward=1"}},
	}
	got := r.Calls()
	if len(got) != len(want) {
		t.Fatalf("Calls returned %d calls, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Name != want[i].Name || !slices.Equal(got[i].Args, want[i].Args) {
			t.Errorf("call %d is %v %v, want %v %v", i, got[i].Name, got[i].Args, want[i].Name, want[i].Args)
		}
	}
}

// The recording Runner fails a test when the code under test runs a command that the
// test did not script.
func TestRecorderFailsTheTestOnAnUnscriptedCommand(t *testing.T) {
	f := &fakeReporter{}
	r := NewRecorder(f)
	r.Script(Result{}, "ip", "netns", "add", "hs-work")

	out, err := r.Run(context.Background(), "ip", "netns", "del", "hs-work")
	if err == nil {
		t.Fatal("Run returned no error, want an error for an unscripted command")
	}
	if out != nil {
		t.Fatalf("Run returned the output %q, want no output", out)
	}
	if len(f.failures) != 1 {
		t.Fatalf("the Recorder reported %d failures, want 1: %v", len(f.failures), f.failures)
	}
	if !strings.Contains(f.failures[0], "ip netns del hs-work") {
		t.Errorf("the failure is %q, want the command in the text", f.failures[0])
	}
}

func TestRecorderRecordsAnUnscriptedCommand(t *testing.T) {
	f := &fakeReporter{}
	r := NewRecorder(f)

	_, _ = r.Run(context.Background(), "iptables", "-C", "FORWARD", "-i", "hs0", "-j", "ACCEPT")

	got := r.Calls()
	if len(got) != 1 {
		t.Fatalf("Calls returned %d calls, want 1", len(got))
	}
	if got[0].Name != "iptables" || !slices.Equal(got[0].Args, []string{"-C", "FORWARD", "-i", "hs0", "-j", "ACCEPT"}) {
		t.Errorf("call 0 is %v %v, want the unscripted command", got[0].Name, got[0].Args)
	}
}

// A command with the same name and a different argument list is a different command.
func TestRecorderMatchesTheWholeArgumentList(t *testing.T) {
	f := &fakeReporter{}
	r := NewRecorder(f)
	r.Script(Result{Output: []byte("added\n")}, "ip", "link", "add", "veth0", "type", "veth")

	if _, err := r.Run(context.Background(), "ip", "link", "add", "veth0"); err == nil {
		t.Fatal("Run returned no error, want an error for a shorter argument list")
	}
	if len(f.failures) != 1 {
		t.Fatalf("the Recorder reported %d failures, want 1", len(f.failures))
	}
}

func TestRecorderReturnsTheScriptedResultOnEveryCall(t *testing.T) {
	r := NewRecorder(t)
	r.Script(Result{Output: []byte("up\n")}, "ip", "link", "set", "veth0", "up")

	for i := range 3 {
		out, err := r.Run(context.Background(), "ip", "link", "set", "veth0", "up")
		if err != nil {
			t.Fatalf("call %d returned the error %v, want no error", i, err)
		}
		if string(out) != "up\n" {
			t.Fatalf("call %d returned %q, want %q", i, out, "up\n")
		}
	}
	if len(r.Calls()) != 3 {
		t.Fatalf("Calls returned %d calls, want 3", len(r.Calls()))
	}
}

// Calls returns a copy. A change to the copy does not change the record.
func TestRecorderCallsReturnsACopy(t *testing.T) {
	r := NewRecorder(t)
	r.Script(Result{}, "ip", "netns", "list")
	if _, err := r.Run(context.Background(), "ip", "netns", "list"); err != nil {
		t.Fatalf("Run returned the error %v, want no error", err)
	}

	got := r.Calls()
	got[0].Name = "changed"
	got[0].Args[0] = "changed"

	again := r.Calls()
	if again[0].Name != "ip" || again[0].Args[0] != "netns" {
		t.Fatalf("the record holds %v %v, want the original command", again[0].Name, again[0].Args)
	}
}
