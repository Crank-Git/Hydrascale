package execx

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Runner is the interface that the daemon holds. Both implementations satisfy it.
var (
	_ Runner      = OSRunner{}
	_ Runner      = (*Recorder)(nil)
	_ InputRunner = OSRunner{}
	_ InputRunner = (*Recorder)(nil)
)

func TestRunInputSendsTheBytesToTheStandardInputOfTheCommand(t *testing.T) {
	out, err := OSRunner{}.RunInput(context.Background(), []byte("the rule file\n"), "cat")
	if err != nil {
		t.Fatalf("RunInput returned the error %v, want no error", err)
	}
	if string(out) != "the rule file\n" {
		t.Errorf("RunInput output = %q, want %q", out, "the rule file\n")
	}
}

func TestRunInputReturnsTheErrorOfACommandThatExitsNonZero(t *testing.T) {
	if _, err := (OSRunner{}).RunInput(context.Background(), nil, "sh", "-c", "exit 3"); err == nil {
		t.Error("RunInput returned no error for a command that exits 3")
	}
}

func TestTheRecorderKeepsTheStandardInputOfEveryCommand(t *testing.T) {
	rec := NewRecorder(t)
	rec.Script(Result{}, "iptables-restore", "--noflush")

	if _, err := rec.RunInput(context.Background(), []byte("*filter\nCOMMIT\n"), "iptables-restore", "--noflush"); err != nil {
		t.Fatalf("RunInput: %v", err)
	}

	calls := rec.Calls()
	if len(calls) != 1 {
		t.Fatalf("the Recorder kept %d commands, want 1", len(calls))
	}
	if string(calls[0].Stdin) != "*filter\nCOMMIT\n" {
		t.Errorf("Stdin = %q, want %q", calls[0].Stdin, "*filter\nCOMMIT\n")
	}
}

func TestOSRunnerReturnsCombinedOutput(t *testing.T) {
	out, err := OSRunner{}.Run(context.Background(), "sh", "-c", "echo out; echo err 1>&2")
	if err != nil {
		t.Fatalf("Run returned the error %v, want no error", err)
	}
	text := string(out)
	if !strings.Contains(text, "out") || !strings.Contains(text, "err") {
		t.Fatalf("Run returned %q, want both streams", text)
	}
}

func TestOSRunnerReturnsErrorOnNonZeroExit(t *testing.T) {
	out, err := OSRunner{}.Run(context.Background(), "sh", "-c", "echo failed 1>&2; exit 3")
	if err == nil {
		t.Fatal("Run returned no error, want an error for exit code 3")
	}
	if !strings.Contains(string(out), "failed") {
		t.Fatalf("Run returned the output %q, want the output of the failed command", out)
	}
}

func TestOSRunnerReturnsErrorWhenTheCommandDoesNotStart(t *testing.T) {
	if _, err := (OSRunner{}).Run(context.Background(), "hydrascale-no-such-command"); err == nil {
		t.Fatal("Run returned no error, want an error for a command that does not start")
	}
}

// A command that outlives its context is killed.
func TestOSRunnerKillsTheCommandWhenTheContextEnds(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := OSRunner{}.Run(ctx, "sleep", "30")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Run returned no error, want an error for a killed command")
	}
	if elapsed > 10*time.Second {
		t.Fatalf("Run returned after %v, want a return within 10s of the deadline", elapsed)
	}
	if ctx.Err() != context.DeadlineExceeded {
		t.Fatalf("the context holds the error %v, want context.DeadlineExceeded", ctx.Err())
	}
}
