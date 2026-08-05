// Package scripts holds the tests for the repository hygiene script.
package scripts

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// scriptPath returns the absolute path of the hygiene script.
func scriptPath(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs("check-hygiene.sh")
	if err != nil {
		t.Fatalf("resolve the script path: %v", err)
	}
	return path
}

// runScript runs the hygiene script in dir and returns the exit code and the output.
func runScript(t *testing.T, dir string) (int, string) {
	t.Helper()
	cmd := exec.Command(scriptPath(t))
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatalf("run the script: %v", err)
		}
		code = exit.ExitCode()
	}
	return code, string(out)
}

// git runs one Git command in dir and fails the test when the command fails.
func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// newRepo returns a temporary repository that holds the hygiene script and one tracked
// file. The script reads the tracked file list, so the test must add every file.
func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "--quiet")
	if err := os.MkdirAll(filepath.Join(dir, "scripts"), 0o755); err != nil {
		t.Fatalf("create the scripts directory: %v", err)
	}
	source, err := os.ReadFile(scriptPath(t))
	if err != nil {
		t.Fatalf("read the script: %v", err)
	}
	writeFile(t, dir, "scripts/check-hygiene.sh", string(source), 0o755)
	writeFile(t, dir, "README.md", "A placeholder auth key is tskey-auth-xxxxx.\n", 0o644)
	git(t, dir, "add", "-A")
	return dir
}

// writeFile writes name under dir and creates the parent directory.
func writeFile(t *testing.T, dir, name, content string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create the parent directory of %s: %v", name, err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestScriptAcceptsARepositoryThatHoldsOnlyPlaceholders(t *testing.T) {
	dir := newRepo(t)
	code, out := runScript(t, dir)
	if code != 0 {
		t.Fatalf("the script exits %d, want 0\n%s", code, out)
	}
}

func TestScriptRejectsATrackedFileThatHoldsARealAuthKey(t *testing.T) {
	dir := newRepo(t)
	// The test builds the key from parts, because a literal key in this file would make
	// the script fail on this file.
	key := "tskey-" + "auth-" + strings.Repeat("k", 24)
	writeFile(t, dir, "internal/config/leaked.go", "const authKey = \""+key+"\"\n", 0o644)
	git(t, dir, "add", "-A")

	code, out := runScript(t, dir)
	if code == 0 {
		t.Fatalf("the script exits 0, want non-zero\n%s", out)
	}
	if !strings.Contains(out, "internal/config/leaked.go") {
		t.Errorf("the output names no file\n%s", out)
	}
	if !strings.Contains(out, "tskey-[a-z]+-[A-Za-z0-9]{22,}") {
		t.Errorf("the output names no pattern\n%s", out)
	}
	if strings.Contains(out, key) {
		t.Errorf("the output holds the value that matched\n%s", out)
	}
}

func TestScriptRejectsAnAwsAccessKeyIdAndAPrivateKeyBlock(t *testing.T) {
	// Each value comes from parts, because a whole literal in this file would make the
	// script fail on this file.
	cases := []struct {
		name    string
		file    string
		content string
		pattern string
	}{
		{
			name:    "an AWS access key id",
			file:    "deploy/keys.txt",
			content: "AKIA" + strings.Repeat("Q", 16) + "\n",
			pattern: "AKIA[0-9A-Z]{16}",
		},
		{
			name:    "a private key block",
			file:    "deploy/id_rsa",
			content: "-----BEGIN " + "OPENSSH PRIVATE KEY-----\n",
			pattern: "BEGIN (RSA |OPENSSH |EC )?PRIVATE KEY",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := newRepo(t)
			writeFile(t, dir, c.file, c.content, 0o644)
			git(t, dir, "add", "-A")

			code, out := runScript(t, dir)
			if code == 0 {
				t.Fatalf("the script exits 0, want non-zero\n%s", out)
			}
			if !strings.Contains(out, c.file) || !strings.Contains(out, c.pattern) {
				t.Errorf("the output names no file and no pattern\n%s", out)
			}
		})
	}
}

func TestScriptRejectsAPrivateDevelopmentNoteAndToolState(t *testing.T) {
	cases := []struct {
		file    string
		pattern string
	}{
		{file: "TODOS.md", pattern: "(^|/)(TODOS|HYPERPLAN|AGENTS)\\.md$"},
		{file: "docs/AGENTS.md", pattern: "(^|/)(TODOS|HYPERPLAN|AGENTS)\\.md$"},
		{file: ".gstack/state.json", pattern: "^\\.(gstack|omc|sisyphus|openagent)/"},
		{file: ".sisyphus/run.log", pattern: "^\\.(gstack|omc|sisyphus|openagent)/"},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			dir := newRepo(t)
			writeFile(t, dir, c.file, "a note\n", 0o644)
			git(t, dir, "add", "-A")

			code, out := runScript(t, dir)
			if code == 0 {
				t.Fatalf("the script exits 0, want non-zero\n%s", out)
			}
			if !strings.Contains(out, c.file) || !strings.Contains(out, c.pattern) {
				t.Errorf("the output names no file and no pattern\n%s", out)
			}
		})
	}
}

func TestScriptRejectsALargeFileOutsideTheBrandDirectory(t *testing.T) {
	large := strings.Repeat("x", 2*1024*1024+1)

	dir := newRepo(t)
	writeFile(t, dir, "assets/build.bin", large, 0o644)
	git(t, dir, "add", "-A")
	code, out := runScript(t, dir)
	if code == 0 {
		t.Fatalf("the script exits 0, want non-zero\n%s", out)
	}
	if !strings.Contains(out, "assets/build.bin") || !strings.Contains(out, "2 MB") {
		t.Errorf("the output names no file and no pattern\n%s", out)
	}

	// Issue #58 keeps the brand assets, so the size rule holds one exception.
	allowed := newRepo(t)
	writeFile(t, allowed, "internal/ui/static/brand/logo.png", large, 0o644)
	git(t, allowed, "add", "-A")
	if code, out := runScript(t, allowed); code != 0 {
		t.Fatalf("the script exits %d on a large brand asset, want 0\n%s", code, out)
	}
}

func TestScriptAcceptsThisRepository(t *testing.T) {
	code, out := runScript(t, "..")
	if code != 0 {
		t.Fatalf("the script exits %d on the repository, want 0\n%s", code, out)
	}
}
