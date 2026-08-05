package access

import (
	"context"
	"errors"
	"strings"
	"testing"

	"hydrascale/internal/execx"
)

// absentChain is the result that iptables returns for a chain that the host does not hold.
var absentChain = execx.Result{
	Output: []byte("iptables: No chain/target/match by that name.\n"),
	Err:    errors.New("exit status 1"),
}

// absentRule is the result that iptables returns for a delete of a rule that is not
// present.
var absentRule = execx.Result{
	Output: []byte("iptables: Bad rule (does a matching rule exist in that chain?).\n"),
	Err:    errors.New("exit status 1"),
}

// missingRule is the result that iptables returns for a check of a rule that is not
// present. The check writes no message that names the state.
var missingRule = execx.Result{Err: errors.New("exit status 1")}

// testSet returns a small compiled rule set: one namespace that reaches the host, and the
// enforce tail.
func testSet() Compiled {
	return Compiled{
		Forward: [][]string{
			{"-A", ChainForward, "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"},
			{"-A", ChainForward, "-i", "vh0123456789ab", "-o", "vhba9876543210", "-j", "ACCEPT"},
			{"-A", ChainForward, "-j", "DROP"},
		},
		Out: [][]string{
			{"-A", ChainOut, "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"},
			{"-A", ChainOut, "-i", "vh0123456789ab", "-j", "DROP"},
		},
	}
}

// writerFixture returns a Recorder and a Writer that runs every command through it.
// present holds the fingerprint that each chain reports; an empty fingerprint makes the
// chain absent.
func writerFixture(t *testing.T, present string) (*execx.Recorder, *Writer) {
	t.Helper()

	rec := execx.NewRecorder(t)
	for _, j := range jumps {
		if present == "" {
			rec.Script(absentChain, "iptables", "-S", j.chain)
		} else {
			out := "-N " + j.chain + "\n-A " + j.chain + " -m comment --comment " + markerPrefix + present + "\n"
			rec.Script(execx.Result{Output: []byte(out)}, "iptables", "-S", j.chain)
		}
		rec.Script(missingRule, "iptables", "-C", j.parent, "-j", j.chain)
		rec.Script(execx.Result{}, "iptables", "-I", j.parent, "1", "-j", j.chain)
	}
	rec.Script(execx.Result{}, "iptables-restore", "--noflush")

	return rec, &Writer{Runner: rec}
}

func TestApplyWritesTheCompiledRuleSetWithIptablesRestoreOnStandardInput(t *testing.T) {
	rec, w := writerFixture(t, "")

	wrote, err := w.Apply(context.Background(), testSet())
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !wrote {
		t.Error("Apply reported no write for a host that holds no chain")
	}

	var file string
	restores := 0
	for _, c := range rec.Calls() {
		if c.Name != "iptables-restore" {
			continue
		}
		restores++
		if len(c.Args) != 1 || c.Args[0] != "--noflush" {
			t.Errorf("iptables-restore arguments = %v, want [--noflush]", c.Args)
		}
		file = string(c.Stdin)
	}
	if restores != 1 {
		t.Fatalf("Apply ran iptables-restore %d times, want 1", restores)
	}

	want := "*filter\n" +
		":HYDRASCALE-FWD - [0:0]\n" +
		":HYDRASCALE-OUT - [0:0]\n" +
		"-F HYDRASCALE-FWD\n" +
		"-F HYDRASCALE-OUT\n" +
		"-A HYDRASCALE-FWD -m comment --comment " + markerPrefix + fingerprint(testSet()) + "\n" +
		"-A HYDRASCALE-OUT -m comment --comment " + markerPrefix + fingerprint(testSet()) + "\n" +
		"-A HYDRASCALE-FWD ! -i vh+ ! -o vh+ -j RETURN\n" +
		"-A HYDRASCALE-FWD -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT\n" +
		"-A HYDRASCALE-FWD -i vh0123456789ab -o vhba9876543210 -j ACCEPT\n" +
		"-A HYDRASCALE-FWD -j DROP\n" +
		"-A HYDRASCALE-OUT -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT\n" +
		"-A HYDRASCALE-OUT -i vh0123456789ab -j DROP\n" +
		"COMMIT\n"
	if file != want {
		t.Errorf("the rule file =\n%s\nwant\n%s", file, want)
	}
}

func TestApplyWritesOneJumpIntoForwardThatTargetsTheForwardChain(t *testing.T) {
	rec, w := writerFixture(t, "")

	if _, err := w.Apply(context.Background(), testSet()); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	want := execx.Call{Name: "iptables", Args: []string{"-I", "FORWARD", "1", "-j", ChainForward}}
	if got := countCalls(rec, want); got != 1 {
		t.Errorf("Apply wrote %d jump rules into FORWARD, want 1", got)
	}
}

func TestApplyWritesOneJumpIntoInputThatTargetsTheOutChain(t *testing.T) {
	rec, w := writerFixture(t, "")

	if _, err := w.Apply(context.Background(), testSet()); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	want := execx.Call{Name: "iptables", Args: []string{"-I", "INPUT", "1", "-j", ChainOut}}
	if got := countCalls(rec, want); got != 1 {
		t.Errorf("Apply wrote %d jump rules into INPUT, want 1", got)
	}
}

func TestApplyWritesNoRuleOutsideTheTwoChainsAndTheTwoJumps(t *testing.T) {
	rec, w := writerFixture(t, "")

	if _, err := w.Apply(context.Background(), testSet()); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	for _, c := range rec.Calls() {
		if c.Name != "iptables" {
			continue
		}
		if c.Args[0] != "-I" {
			continue
		}
		if len(c.Args) != 5 || c.Args[2] != "1" || c.Args[3] != "-j" {
			t.Errorf("Apply wrote a rule that is not a jump: %s", c)
		}
	}
}

func TestApplyRunsNoWriteWhenTheLiveChainHoldsTheCompiledRuleSet(t *testing.T) {
	rec, w := writerFixture(t, fingerprint(testSet()))

	wrote, err := w.Apply(context.Background(), testSet())
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if wrote {
		t.Error("Apply reported a write for a host that already holds the compiled rule set")
	}
	for _, c := range rec.Calls() {
		if c.Name == "iptables-restore" {
			t.Errorf("Apply ran a write for an unchanged rule set: %s", c)
		}
	}
}

func TestApplyWritesWhenTheLiveChainHoldsAnotherRuleSet(t *testing.T) {
	rec, w := writerFixture(t, "0123456789abcdef")

	wrote, err := w.Apply(context.Background(), testSet())
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !wrote {
		t.Error("Apply reported no write for a chain that holds another rule set")
	}
	if countName(rec, "iptables-restore") != 1 {
		t.Error("Apply ran no write for a chain that holds another rule set")
	}
}

func TestApplyKeepsAJumpRuleThatIsAlreadyPresent(t *testing.T) {
	rec, w := writerFixture(t, fingerprint(testSet()))
	for _, j := range jumps {
		rec.Script(execx.Result{}, "iptables", "-C", j.parent, "-j", j.chain)
	}

	if _, err := w.Apply(context.Background(), testSet()); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	for _, c := range rec.Calls() {
		if c.Name == "iptables" && c.Args[0] == "-I" {
			t.Errorf("Apply wrote a jump rule that is already present: %s", c)
		}
	}
}

func TestApplyReturnsTheErrorOfAFailedWrite(t *testing.T) {
	rec, w := writerFixture(t, "")
	rec.Script(execx.Result{
		Output: []byte("iptables-restore: line 7 failed"),
		Err:    errors.New("exit status 1"),
	}, "iptables-restore", "--noflush")

	wrote, err := w.Apply(context.Background(), testSet())
	if err == nil {
		t.Fatal("Apply returned no error for a failed write")
	}
	if wrote {
		t.Error("Apply reported a write that failed")
	}
	if !strings.Contains(err.Error(), "line 7 failed") {
		t.Errorf("the error = %q, want the output of the command", err)
	}
}

func TestTeardownRemovesBothJumpRulesAndBothChains(t *testing.T) {
	rec := execx.NewRecorder(t)
	want := []execx.Call{
		{Name: "iptables", Args: []string{"-D", "FORWARD", "-j", ChainForward}},
		{Name: "iptables", Args: []string{"-D", "INPUT", "-j", ChainOut}},
		{Name: "iptables", Args: []string{"-F", ChainForward}},
		{Name: "iptables", Args: []string{"-X", ChainForward}},
		{Name: "iptables", Args: []string{"-F", ChainOut}},
		{Name: "iptables", Args: []string{"-X", ChainOut}},
	}
	for _, c := range want {
		rec.Script(execx.Result{}, c.Name, c.Args...)
	}

	w := &Writer{Runner: rec}
	if err := w.Teardown(context.Background()); err != nil {
		t.Fatalf("Teardown: %v", err)
	}

	got := rec.Calls()
	if len(got) != len(want) {
		t.Fatalf("Teardown ran %d commands, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].String() != want[i].String() {
			t.Errorf("command %d = %q, want %q", i, got[i].String(), want[i].String())
		}
	}
}

func TestTeardownTreatsAnAbsentRuleAsSuccess(t *testing.T) {
	rec := execx.NewRecorder(t)
	rec.Script(absentRule, "iptables", "-D", "FORWARD", "-j", ChainForward)
	rec.Script(absentRule, "iptables", "-D", "INPUT", "-j", ChainOut)
	for _, j := range jumps {
		rec.Script(absentChain, "iptables", "-F", j.chain)
		rec.Script(absentChain, "iptables", "-X", j.chain)
	}

	w := &Writer{Runner: rec}
	if err := w.Teardown(context.Background()); err != nil {
		t.Errorf("Teardown returned an error for an absent rule: %v", err)
	}
	if len(rec.Calls()) != 6 {
		t.Errorf("Teardown ran %d commands, want 6", len(rec.Calls()))
	}
}

func TestTeardownReturnsEveryFailureTogether(t *testing.T) {
	broken := execx.Result{
		Output: []byte("iptables: Permission denied (you must be root)."),
		Err:    errors.New("exit status 4"),
	}

	rec := execx.NewRecorder(t)
	rec.Script(broken, "iptables", "-D", "FORWARD", "-j", ChainForward)
	rec.Script(broken, "iptables", "-D", "INPUT", "-j", ChainOut)
	for _, j := range jumps {
		rec.Script(broken, "iptables", "-F", j.chain)
		rec.Script(broken, "iptables", "-X", j.chain)
	}

	w := &Writer{Runner: rec}
	err := w.Teardown(context.Background())
	if err == nil {
		t.Fatal("Teardown returned no error for six failed commands")
	}
	if len(rec.Calls()) != 6 {
		t.Errorf("Teardown ran %d commands after a failure, want 6", len(rec.Calls()))
	}
	if lines := strings.Count(err.Error(), "\n") + 1; lines != 6 {
		t.Errorf("the error holds %d failures, want 6: %v", lines, err)
	}
}

func TestTheRuleFileQuotesAnArgumentThatHoldsASpace(t *testing.T) {
	got := ruleLine([]string{"-j", "LOG", "--log-prefix", LogPrefix})
	want := `-j LOG --log-prefix "hydrascale-would-deny: "`
	if got != want {
		t.Errorf("ruleLine = %q, want %q", got, want)
	}
}

func TestTheForwardChainReturnsAPacketThatTouchesNoNamespaceDevice(t *testing.T) {
	file := string(restoreFile(testSet(), "0123456789abcdef"))

	want := "-A " + ChainForward + " ! -i vh+ ! -o vh+ -j RETURN"
	if !strings.Contains(file, want+"\n") {
		t.Errorf("the rule file holds no rule %q:\n%s", want, file)
	}

	// The return rule comes before every rule of the compiled set, so the operator keeps
	// the container traffic and the subnet route traffic of the host.
	drop := "-A " + ChainForward + " -j DROP"
	if strings.Index(file, want) > strings.Index(file, drop) {
		t.Error("the return rule comes after the closing DROP rule")
	}
}

func TestTheOutChainHoldsNoReturnForAPacketThatTouchesNoNamespaceDevice(t *testing.T) {
	file := string(restoreFile(testSet(), "0123456789abcdef"))

	if strings.Contains(file, "-A "+ChainOut+" ! -i vh+") {
		t.Error("the out chain holds the return rule of the forward chain")
	}
}

func TestTheFingerprintChangesWhenARuleChanges(t *testing.T) {
	base := testSet()
	other := testSet()
	other.Forward[1] = []string{"-A", ChainForward, "-i", "vh0123456789ab", "-j", "ACCEPT"}

	if fingerprint(base) == fingerprint(other) {
		t.Error("two different rule sets have the same fingerprint")
	}
	if fingerprint(base) != fingerprint(testSet()) {
		t.Error("the same rule set has two fingerprints")
	}
}

func TestAWriterWithNoRunnerUsesTheOSRunner(t *testing.T) {
	var w Writer
	if _, ok := w.runner().(execx.OSRunner); !ok {
		t.Errorf("runner() = %T, want execx.OSRunner", w.runner())
	}
}

// countCalls returns the number of recorded commands that equal want.
func countCalls(rec *execx.Recorder, want execx.Call) int {
	n := 0
	for _, c := range rec.Calls() {
		if c.String() == want.String() {
			n++
		}
	}
	return n
}

// countName returns the number of recorded commands with the given command name.
func countName(rec *execx.Recorder, name string) int {
	n := 0
	for _, c := range rec.Calls() {
		if c.Name == name {
			n++
		}
	}
	return n
}
