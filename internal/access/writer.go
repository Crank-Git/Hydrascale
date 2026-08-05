package access

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"hydrascale/internal/execx"
)

// CommandRunner runs a command that takes no input, and a command that reads its rule file
// on standard input. iptables-restore reads on standard input, therefore the Writer holds
// both methods. execx.OSRunner and execx.Recorder carry both.
type CommandRunner interface {
	execx.Runner
	execx.InputRunner
}

// markerPrefix opens the comment of the marker rule. The Writer puts one marker rule at
// the head of each chain, and the comment holds the fingerprint of the compiled rule set.
//
// FR-access-17 asks the daemon to write only when the compiled rule set and the live chain
// differ. `iptables -S` returns a rule in a normal form: it orders the options, it appends
// a prefix length to an address, and it adds the implicit protocol match. A comparison of
// that text against the compiled arguments therefore reports a difference on every tick.
// The marker rule carries the fingerprint through the kernel unchanged, so the comparison
// is exact and it costs one command for each chain.
const markerPrefix = "hydrascale:"

// Writer writes the compiled rule set into the two chains that the daemon owns, and it
// keeps one jump rule into FORWARD and one into INPUT.
type Writer struct {
	// Runner runs every command that the Writer sends to the host. A test replaces
	// Runner with an execx.Recorder and asserts the exact argument list.
	Runner CommandRunner
}

// NewWriter returns a Writer that runs each command on the host.
func NewWriter() *Writer {
	return &Writer{Runner: execx.OSRunner{}}
}

// runner returns the command runner. A Writer with no Runner runs on the host.
func (w *Writer) runner() CommandRunner {
	if w.Runner == nil {
		return execx.OSRunner{}
	}
	return w.Runner
}

// jump names one jump rule that the daemon owns.
type jump struct {
	parent string
	chain  string
}

// jumps holds the two jump rules that the daemon owns. The operator decided on 2026-08-05
// that the daemon inserts the jump at position 1.
var jumps = []jump{
	{parent: "FORWARD", chain: ChainForward},
	{parent: "INPUT", chain: ChainOut},
}

// Apply makes the two chains hold the compiled rule set, and it makes FORWARD and INPUT
// each hold one jump rule into the matching chain.
// ctx bounds every command, and c is the output of Compile.
// Apply reports true when it wrote the chains, and false when the live chains already held
// the compiled rule set.
// Apply returns an error when a command fails. A failed write leaves the previous chains in
// place, because iptables-restore applies the file as one transaction.
func (w *Writer) Apply(ctx context.Context, c Compiled) (bool, error) {
	want := fingerprint(c)

	live, err := w.liveFingerprints(ctx)
	if err != nil {
		return false, err
	}

	wrote := false
	if live[ChainForward] != want || live[ChainOut] != want {
		file := restoreFile(c, want)
		if out, err := w.runner().RunInput(ctx, file, "iptables-restore", "--noflush"); err != nil {
			return false, fmt.Errorf("iptables-restore --noflush: %v (%s)", err, out)
		}
		wrote = true
	}

	var errs []error
	for _, j := range jumps {
		errs = append(errs, w.ensureJump(ctx, j))
	}
	return wrote, errors.Join(errs...)
}

// Teardown removes both chains and both jump rules.
// Teardown treats an absent rule and an absent chain as success, because an operator who
// already removed one reached the wanted result. A step that fails does not stop the
// remaining steps; Teardown collects every failure and returns the failures together.
func (w *Writer) Teardown(ctx context.Context) error {
	var errs []error
	for _, j := range jumps {
		errs = append(errs, w.deleteRule(ctx, "-D", j.parent, "-j", j.chain))
	}
	for _, j := range jumps {
		errs = append(errs, w.deleteRule(ctx, "-F", j.chain))
		errs = append(errs, w.deleteRule(ctx, "-X", j.chain))
	}
	return errors.Join(errs...)
}

// ensureJump makes the parent chain hold one jump rule into the daemon chain.
// ensureJump runs the check first, so that a tick on an unchanged host writes nothing. An
// operator firewall that reloads and removes the jump gets the jump back on the next tick.
func (w *Writer) ensureJump(ctx context.Context, j jump) error {
	if _, err := w.runner().Run(ctx, "iptables", "-C", j.parent, "-j", j.chain); err == nil {
		return nil
	}
	out, err := w.runner().Run(ctx, "iptables", "-I", j.parent, "1", "-j", j.chain)
	if err != nil {
		return fmt.Errorf("iptables -I %s 1 -j %s: %v (%s)", j.parent, j.chain, err, out)
	}
	return nil
}

// deleteRule runs one iptables command that removes a rule or a chain.
// deleteRule returns nil when the command succeeds, when the rule is absent, and when the
// chain is absent. Any other failure carries the command line and the output.
func (w *Writer) deleteRule(ctx context.Context, args ...string) error {
	out, err := w.runner().Run(ctx, "iptables", args...)
	if err == nil || absent(out) {
		return nil
	}
	return fmt.Errorf("iptables %s: %v (%s)", strings.Join(args, " "), err, out)
}

// liveFingerprints returns the fingerprint that the marker rule of each chain holds.
// A chain that the host does not hold, and a chain that holds no marker rule, gets an empty
// string, therefore the Writer writes it.
func (w *Writer) liveFingerprints(ctx context.Context) (map[string]string, error) {
	found := make(map[string]string, len(jumps))
	for _, j := range jumps {
		out, err := w.runner().Run(ctx, "iptables", "-S", j.chain)
		if err != nil {
			if absent(out) {
				found[j.chain] = ""
				continue
			}
			return nil, fmt.Errorf("iptables -S %s: %v (%s)", j.chain, err, out)
		}
		found[j.chain] = markerOf(string(out))
	}
	return found, nil
}

// absent reports whether the output of an iptables command states that the rule or the
// chain is not present. iptables exits non-zero for both states.
func absent(output []byte) bool {
	text := string(output)
	return strings.Contains(text, "does a matching rule exist") ||
		strings.Contains(text, "No chain/target/match by that name")
}

// markerOf returns the fingerprint that the marker rule in the output of `iptables -S`
// holds, and an empty string when the output holds no marker rule.
func markerOf(output string) string {
	for _, line := range strings.Split(output, "\n") {
		for _, field := range strings.Fields(line) {
			value := strings.Trim(field, `"`)
			if rest, ok := strings.CutPrefix(value, markerPrefix); ok {
				return rest
			}
		}
	}
	return ""
}

// fingerprint returns a value that changes when any rule of the compiled set changes.
// fingerprint is stable: the same compiled set always produces the same value, because
// Compile emits the rules in a fixed order.
func fingerprint(c Compiled) string {
	sum := sha256.New()
	for _, rules := range [][][]string{c.Forward, c.Out} {
		for _, rule := range rules {
			for _, arg := range rule {
				sum.Write([]byte(arg))
				sum.Write([]byte{0})
			}
			sum.Write([]byte{'\n'})
		}
	}
	return hex.EncodeToString(sum.Sum(nil))[:16]
}

// restoreFile returns the rule file that iptables-restore --noflush reads.
// c is the compiled rule set and mark is its fingerprint.
// The file names the filter table and the two chains that the daemon owns. It holds one
// flush command for each of those chains, so that the write replaces the previous rules
// inside the same transaction. The file names no other chain, therefore an operator rule
// keeps its place.
func restoreFile(c Compiled, mark string) []byte {
	var b strings.Builder
	b.WriteString("*filter\n")
	for _, j := range jumps {
		fmt.Fprintf(&b, ":%s - [0:0]\n", j.chain)
	}
	for _, j := range jumps {
		fmt.Fprintf(&b, "-F %s\n", j.chain)
	}
	// The marker rule holds no target, so the packet continues to the next rule.
	for _, j := range jumps {
		fmt.Fprintf(&b, "-A %s -m comment --comment %s%s\n", j.chain, markerPrefix, mark)
	}
	// A packet that touches no namespace device returns to FORWARD. The jump sits at
	// position 1, therefore every forwarded packet of the host enters this chain, and the
	// closing DROP rule would also stop the container traffic and the subnet route traffic
	// of the operator. The daemon denies the paths of a namespace; it owns no other path.
	fmt.Fprintf(&b, "-A %s ! -i %s+ ! -o %s+ -j RETURN\n", ChainForward, devicePrefix, devicePrefix)
	for _, rules := range [][][]string{c.Forward, c.Out} {
		for _, rule := range rules {
			b.WriteString(ruleLine(rule))
			b.WriteString("\n")
		}
	}
	b.WriteString("COMMIT\n")
	return []byte(b.String())
}

// ruleLine returns one argument list as one line of the rule file.
// ruleLine puts an argument that holds a space inside double quotes, because
// iptables-restore splits a line on a space.
func ruleLine(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		if strings.ContainsAny(arg, " \t\"") {
			quoted[i] = `"` + strings.ReplaceAll(arg, `"`, `\"`) + `"`
			continue
		}
		quoted[i] = arg
	}
	return strings.Join(quoted, " ")
}
