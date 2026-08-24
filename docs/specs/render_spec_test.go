// Package specs runs the tests of the specification renderer from the Go test suite.
//
// render-spec.py writes spec.html and html/, which is the published form of the
// specification. render-spec-test.py holds the tests of that renderer. No gate ran them
// before issue #392. A developer had to remember the command, so a change that broke the
// renderer passed continuous integration. Issue #383 shows the cost: two requirements
// stated the wrong text on the page for as long as the defect was present.
//
// The test sits next to the renderer, the way scripts/hygiene_test.go sits next to
// check-hygiene.sh. It follows the gate rule of TestTheConsoleJavaScriptTestsPass in
// internal/ui/shell_test.go. A developer machine that holds no python3 skips, and a gate
// that holds no python3 fails.
package specs

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestTheSpecificationRendererTestsPass(t *testing.T) {
	// render-spec-test.py imports render-spec.py by path and needs no package, so the
	// interpreter alone is enough.
	python, err := pythonForRendererTests()
	if err != nil {
		t.Fatal(err)
	}
	if python == "" {
		t.Skip("python3 is not on this host, so the specification renderer tests do not run here")
	}

	// -B writes no bytecode cache. render-spec-test.py loads render-spec.py by path, and a
	// cache in docs/specs/__pycache__ can hold the previous renderer. Python accepts the
	// cache while the source keeps its size and its modification second. A one-character
	// correction keeps both, so the run reports a defect that the source no longer holds.
	out, err := exec.Command(python, "-B", "render-spec-test.py").CombinedOutput()
	t.Logf("python3 -B render-spec-test.py\n%s", out)
	if err != nil {
		t.Fatalf("the specification renderer tests fail: %v", err)
	}

	// The count is the signal that a comment cannot carry: a deleted test still passes,
	// so the floor fails the run instead. See issue 231, which states the same defect for
	// the console JavaScript tests.
	count, err := rendererTestCount(out)
	if err != nil {
		t.Fatal(err)
	}
	if count < minimumRendererTests {
		t.Errorf("the run passes %d specification renderer tests and the floor is %d, so "+
			"docs/specs/render-spec-test.py lost a test. Restore the test, or lower "+
			"minimumRendererTests and state the reason",
			count, minimumRendererTests)
	}
}

// minimumRendererTests is the floor that docs/specs/render-spec-test.py must reach. The
// value is a floor rather than an exact count, because a new renderer test must not fail
// the build. Raise it when a batch of tests lands.
const minimumRendererTests = 7

// rendererSummaryLine matches the line "ok 7 tests" that render-spec-test.py writes last.
var rendererSummaryLine = regexp.MustCompile(`(?m)^ok\s+(\d+)\s+tests\s*$`)

// rendererTestCount returns the number of tests that render-spec-test.py reports as
// passed. It reads the combined output of the run. It returns an error when the output
// holds no summary line, which is a changed report format that hides the count.
func rendererTestCount(out []byte) (int, error) {
	match := rendererSummaryLine.FindSubmatch(out)
	if match == nil {
		return 0, fmt.Errorf("the output of render-spec-test.py holds no summary line, so " +
			"the specification renderer test count is unknown. Correct rendererSummaryLine " +
			"for the report format of the file")
	}
	count, err := strconv.Atoi(string(match[1]))
	if err != nil {
		return 0, fmt.Errorf("the summary line of render-spec-test.py holds no number: %w", err)
	}
	return count, nil
}

func TestTheRendererTestCountComesFromTheSummaryLine(t *testing.T) {
	count, err := rendererTestCount([]byte("ok 7 tests\n"))
	if err != nil {
		t.Fatalf("the summary line is present and the count fails: %v", err)
	}
	if count != 7 {
		t.Errorf("the summary line reports 7 and the count is %d", count)
	}
}

func TestTheRendererTestCountFailsWhenTheOutputHoldsNoSummaryLine(t *testing.T) {
	// A changed report format must fail the run rather than report a count of zero,
	// because a count of zero reads the same as a lost test.
	count, err := rendererTestCount([]byte("FAIL a_code_span_escapes_html\n"))
	if err == nil {
		t.Fatalf("the output holds no summary line and the count is %d", count)
	}
	if !strings.Contains(err.Error(), "summary line") {
		t.Errorf("the message does not name the summary line: %v", err)
	}
}

// pythonForRendererTests looks for python3 on the path. It returns the path of python3
// when python3 is present. It returns an empty path and no error when the caller skips,
// which is a developer machine that holds no python3. It returns an error when the caller
// fails, which is a gate that holds no python3.
//
// A gate runs every test, so a gate that holds no python3 loses every test of
// render-spec-test.py and reports success all the same. Two environment variables mark a
// gate: GitHub Actions sets CI, and the gate script of the test host exports
// HYDRASCALE_GATE. See issue 181 and issue 191.
func pythonForRendererTests() (string, error) {
	python, err := exec.LookPath("python3")
	if err == nil {
		return python, nil
	}
	if os.Getenv("CI") == "" && os.Getenv("HYDRASCALE_GATE") == "" {
		return "", nil
	}
	return "", fmt.Errorf("python3 is not on the path of this gate, so the specification "+
		"renderer tests of docs/specs/render-spec-test.py do not run and their coverage "+
		"disappears without a report. Install python3 on the gate. LookPath: %w", err)
}

func TestTheSpecificationRendererTestsFailOnAGateThatHoldsNoPython(t *testing.T) {
	// A gate that loses python3 runs none of the renderer tests and still reports
	// success, so the coverage disappears and nothing states it.
	t.Setenv("CI", "true")
	t.Setenv("PATH", "")

	python, err := pythonForRendererTests()
	if err == nil {
		t.Fatalf("the gate holds no python3 and the run does not fail: the path is %q", python)
	}
	if !strings.Contains(err.Error(), "python3") {
		t.Errorf("the message does not name python3: %v", err)
	}
}

func TestTheSpecificationRendererTestsFailOnATestHostGateThatHoldsNoPython(t *testing.T) {
	// The gate script of the test host sets no CI, so it exports HYDRASCALE_GATE instead.
	// Without the marker the gate loses every renderer test and reports success.
	t.Setenv("CI", "")
	t.Setenv("HYDRASCALE_GATE", "1")
	t.Setenv("PATH", "")

	python, err := pythonForRendererTests()
	if err == nil {
		t.Fatalf("the gate holds no python3 and the run does not fail: the path is %q", python)
	}
	if !strings.Contains(err.Error(), "python3") {
		t.Errorf("the message does not name python3: %v", err)
	}
}

func TestTheSpecificationRendererTestsSkipOnADeveloperMachineThatHoldsNoPython(t *testing.T) {
	// A developer machine keeps the skip, because python3 is not a tool that a developer
	// must install to change the daemon.
	t.Setenv("CI", "")
	t.Setenv("HYDRASCALE_GATE", "")
	t.Setenv("PATH", "")

	python, err := pythonForRendererTests()
	if err != nil {
		t.Fatalf("the developer machine holds no python3 and the run fails: %v", err)
	}
	if python != "" {
		t.Errorf("the path holds no python3 and the function reports the path %q", python)
	}
}
