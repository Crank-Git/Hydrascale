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
	// Stdin holds the bytes that the code under test sent to the standard input of the
	// command. Stdin is nil for a command that Run received.
	Stdin []byte
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

	mu           sync.Mutex
	scripts      map[string]Result
	startScripts map[string]StartResult
	calls        []Call
	deadlines    []bool
	specs        []Spec
}

// NewRecorder returns a Recorder that reports a failure to t.
func NewRecorder(t TestReporter) *Recorder {
	return &Recorder{
		t:            t,
		scripts:      make(map[string]Result),
		startScripts: make(map[string]StartResult),
	}
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
func (r *Recorder) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	r.mu.Lock()
	call := Call{Name: name, Args: slices.Clone(args)}
	r.calls = append(r.calls, call)
	_, hasDeadline := ctx.Deadline()
	r.deadlines = append(r.deadlines, hasDeadline)
	res, scripted := r.scripts[key(name, args)]
	r.mu.Unlock()

	if !scripted {
		r.t.Helper()
		r.t.Errorf("execx: the test did not script the command: %s", call)
		return nil, fmt.Errorf("execx: unscripted command: %s", call)
	}
	return res.Output, res.Err
}

// RunInput records the command together with stdin, and returns the scripted Result. The
// Recorder matches the name and the argument list, in the manner of Run; it does not match
// stdin, because a test reads the recorded stdin and asserts it.
// For an unscripted command, RunInput reports a failure to the test and returns an error.
func (r *Recorder) RunInput(ctx context.Context, stdin []byte, name string, args ...string) ([]byte, error) {
	r.mu.Lock()
	call := Call{Name: name, Args: slices.Clone(args), Stdin: slices.Clone(stdin)}
	r.calls = append(r.calls, call)
	_, hasDeadline := ctx.Deadline()
	r.deadlines = append(r.deadlines, hasDeadline)
	res, scripted := r.scripts[key(name, args)]
	r.mu.Unlock()

	if !scripted {
		r.t.Helper()
		r.t.Errorf("execx: the test did not script the command: %s", call)
		return nil, fmt.Errorf("execx: unscripted command: %s", call)
	}
	return res.Output, res.Err
}

// StartResult is the outcome that a Recorder returns for one start.
type StartResult struct {
	Pid int
	Err error
}

// ScriptStart sets the StartResult that Start returns for the command name with args.
// ScriptStart matches the name and the whole argument list, in the manner of Script.
func (r *Recorder) ScriptStart(res StartResult, name string, args ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.startScripts[key(name, args)] = res
}

// Start records the command and the Spec, and returns a Handle for the scripted
// StartResult. For an unscripted command, Start reports a failure to the test and returns
// an error.
func (r *Recorder) Start(spec Spec) (Handle, error) {
	call := Call{Name: spec.Name, Args: slices.Clone(spec.Args)}

	r.mu.Lock()
	r.calls = append(r.calls, call)
	// A start carries no context, because the child outlives the call.
	r.deadlines = append(r.deadlines, false)
	r.specs = append(r.specs, spec)
	res, scripted := r.startScripts[key(spec.Name, spec.Args)]
	r.mu.Unlock()

	if !scripted {
		r.t.Helper()
		r.t.Errorf("execx: the test did not script the start: %s", call)
		return nil, fmt.Errorf("execx: unscripted start: %s", call)
	}
	if res.Err != nil {
		return nil, res.Err
	}
	return recordedHandle{pid: res.Pid}, nil
}

// recordedHandle is the Handle that a Recorder returns for a scripted start.
type recordedHandle struct{ pid int }

func (h recordedHandle) Pid() int    { return h.pid }
func (h recordedHandle) Kill() error { return nil }
func (h recordedHandle) Wait() error { return nil }

// Specs returns the Spec of every start that the Recorder received, in order.
func (r *Recorder) Specs() []Spec {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.specs)
}

// Deadlines reports, for every recorded command in order, whether its context carried a
// deadline. The entry of a start is always false, because a start carries no context.
func (r *Recorder) Deadlines() []bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.deadlines)
}

// Calls returns every command that Run and Start received, in order. Calls returns a
// copy.
func (r *Recorder) Calls() []Call {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]Call, len(r.calls))
	for i, c := range r.calls {
		out[i] = Call{Name: c.Name, Args: slices.Clone(c.Args), Stdin: slices.Clone(c.Stdin)}
	}
	return out
}

// key returns one string that holds the name and the argument list. The %q verb quotes
// each element, so two different argument lists never make the same key.
func key(name string, args []string) string {
	return fmt.Sprintf("%q %q", name, args)
}
