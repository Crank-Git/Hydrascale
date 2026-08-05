package execx

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
)

// TestReporter is the part of *testing.T that a Recorder uses.
type TestReporter interface {
	Helper()
	Errorf(format string, args ...any)
}

// Call is one command that a Recorder ran.
type Call struct {
	Name string
	Args []string
}

// String returns the command as one line, for a failure message.
func (c Call) String() string {
	return strings.Join(append([]string{c.Name}, c.Args...), " ")
}

// Result is the output and the error that a Recorder returns for one command.
type Result struct {
	Output []byte
	Err    error
}

// Recorder is a Runner for a test. It returns a scripted Result for a command and it
// records every command that the code under test ran.
//
// A test scripts each command that it expects. The Recorder fails the test when the code
// under test runs a command that the test did not script, because a silent default hides
// a defect.
type Recorder struct {
	t TestReporter

	mu      sync.Mutex
	scripts map[string]Result
	calls   []Call
}

// NewRecorder returns a Recorder that reports a failure to t.
func NewRecorder(t TestReporter) *Recorder {
	return &Recorder{t: t, scripts: make(map[string]Result)}
}

// Script sets the Result that Run returns for the command name with args. The Recorder
// matches the name and the whole argument list. A later Script call for the same command
// replaces the earlier Result.
func (r *Recorder) Script(res Result, name string, args ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.scripts[key(name, args)] = res
}

// Run records the command and returns the scripted Result. For an unscripted command,
// Run reports a failure to the test and returns an error.
func (r *Recorder) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.mu.Lock()
	call := Call{Name: name, Args: slices.Clone(args)}
	r.calls = append(r.calls, call)
	res, scripted := r.scripts[key(name, args)]
	r.mu.Unlock()

	if !scripted {
		r.t.Helper()
		r.t.Errorf("execx: the test did not script the command: %s", call)
		return nil, fmt.Errorf("execx: unscripted command: %s", call)
	}
	return res.Output, res.Err
}

// Calls returns every command that Run received, in order. Calls returns a copy.
func (r *Recorder) Calls() []Call {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]Call, len(r.calls))
	for i, c := range r.calls {
		out[i] = Call{Name: c.Name, Args: slices.Clone(c.Args)}
	}
	return out
}

// key returns one string that holds the name and the argument list. The %q verb quotes
// each element, so two different argument lists never make the same key.
func key(name string, args []string) string {
	return fmt.Sprintf("%q %q", name, args)
}
