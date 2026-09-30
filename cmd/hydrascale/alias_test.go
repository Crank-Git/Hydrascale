package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hydrascale/internal/config"
	"hydrascale/internal/reconciler"
)

// withConfig writes body to a temporary configuration file and points the CLI at it.
// withConfig restores the previous path when the test ends.
func withConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	previous := cfgFile
	cfgFile = path
	t.Cleanup(func() { cfgFile = previous })
	return path
}

const configWithAlias = `version: 2
tailnets:
  - id: Tk4738291056CNTRL
    alias: alias1
`

func TestResolveRef_returns_the_id_of_an_alias(t *testing.T) {
	withConfig(t, configWithAlias)

	got, err := resolveRef("alias1")
	if err != nil {
		t.Fatalf("resolveRef: %v", err)
	}
	if got != "Tk4738291056CNTRL" {
		t.Errorf("resolveRef(alias1) = %q, want %q", got, "Tk4738291056CNTRL")
	}
}

func TestResolveRef_returns_a_name_that_no_tailnet_holds(t *testing.T) {
	// A command that routes work into a namespace read no configuration file before the
	// alias. It must still reach a namespace that the configuration no longer declares.
	withConfig(t, configWithAlias)

	got, err := resolveRef("absent")
	if err != nil {
		t.Fatalf("resolveRef: %v", err)
	}
	if got != "absent" {
		t.Errorf("resolveRef(absent) = %q, want it unchanged", got)
	}
}

func TestResolveRef_reports_a_configuration_file_that_it_cannot_parse(t *testing.T) {
	// Without this error the operator reads "no such namespace" and never learns that the
	// configuration file holds a syntax error.
	withConfig(t, "version: 2\ntailnets:\n  - id: [unclosed\n")

	if _, err := resolveRef("alias1"); err == nil {
		t.Fatal("resolveRef returned no error, want the parse failure")
	}
}

func TestResolveRef_returns_the_name_when_no_configuration_file_exists(t *testing.T) {
	previous := cfgFile
	cfgFile = filepath.Join(t.TempDir(), "absent.yaml")
	t.Cleanup(func() { cfgFile = previous })

	got, err := resolveRef("alias1")
	if err != nil {
		t.Fatalf("resolveRef: %v", err)
	}
	if got != "alias1" {
		t.Errorf("resolveRef = %q, want it unchanged", got)
	}
}

func TestAddCmd_refuses_an_id_that_is_the_alias_of_a_tailnet(t *testing.T) {
	// `add alias1` stored a second tailnet whose ID was the alias of the first one. The
	// loader then refused the file, and every later command failed, including `remove`.
	path := withConfig(t, configWithAlias)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	cmd := rootCommand()
	cmd.SetArgs([]string{"add", "alias1"})
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})

	if err := cmd.Execute(); err == nil {
		t.Fatal("add returned no error, want a refusal")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(after) != string(before) {
		t.Errorf("add changed the configuration file, want no change:\n%s", after)
	}
}

// captureStdout returns what fn writes to the standard output stream.
// printStatusTable writes to os.Stdout directly, so a test reads it this way.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	previous := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var b strings.Builder
		io.Copy(&b, r)
		done <- b.String()
	}()
	fn()
	w.Close()
	os.Stdout = previous
	return <-done
}

func statusRow(t *testing.T, out, id string) []string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 1 && fields[0] == id {
			return fields
		}
	}
	t.Fatalf("no row for %s in:\n%s", id, out)
	return nil
}

func TestPrintStatusTable_prints_the_alias_in_the_second_column(t *testing.T) {
	desired := map[string]config.Tailnet{
		"Tk4738291056CNTRL": {ID: "Tk4738291056CNTRL", Alias: "alias1"},
		"Tq8162094375CNTRL": {ID: "Tq8162094375CNTRL"},
	}

	out := captureStdout(t, func() {
		printStatusTable(desired, map[string]*reconciler.TailnetState{}, nil, nil, nil)
	})

	header := statusRow(t, out, "ID")
	if header[1] != "ALIAS" {
		t.Errorf("the second heading is %q, want ALIAS", header[1])
	}
	if got := statusRow(t, out, "Tk4738291056CNTRL")[1]; got != "alias1" {
		t.Errorf("the alias column holds %q, want %q", got, "alias1")
	}
}

func TestPrintStatusTable_prints_a_dash_for_a_tailnet_that_holds_no_alias(t *testing.T) {
	desired := map[string]config.Tailnet{"Tq8162094375CNTRL": {ID: "Tq8162094375CNTRL"}}

	out := captureStdout(t, func() {
		printStatusTable(desired, map[string]*reconciler.TailnetState{}, nil, nil, nil)
	})

	if got := statusRow(t, out, "Tq8162094375CNTRL")[1]; got != "-" {
		t.Errorf("the alias column holds %q, want a dash", got)
	}
}

func TestPrintStatusTable_keeps_the_namespace_in_the_third_column(t *testing.T) {
	// The alias column moved every later column, so the row must still line up with the
	// heading that names it.
	desired := map[string]config.Tailnet{"Tk4738291056CNTRL": {ID: "Tk4738291056CNTRL", Alias: "alias1"}}

	out := captureStdout(t, func() {
		printStatusTable(desired, map[string]*reconciler.TailnetState{}, nil, nil, nil)
	})

	header := statusRow(t, out, "ID")
	if header[2] != "NAMESPACE" {
		t.Errorf("the third heading is %q, want NAMESPACE", header[2])
	}
	if got := statusRow(t, out, "Tk4738291056CNTRL")[2]; got != "ns-Tk4738291056CNTRL" {
		t.Errorf("the namespace column holds %q, want the namespace name", got)
	}
}

func TestResolveRef_names_the_need_for_read_access_when_the_file_is_unreadable(t *testing.T) {
	// The configuration directory of a host is readable by root alone, and `hydrascale env`
	// runs as the operator. The error must name the cause, not only "permission denied".
	if os.Geteuid() == 0 {
		t.Skip("root reads a file that has no permission bits")
	}
	path := withConfig(t, configWithAlias)
	if err := os.Chmod(path, 0); err != nil {
		t.Fatalf("Chmod: %v", err)
	}

	_, err := resolveRef("alias1")
	if err == nil {
		t.Fatal("resolveRef returned no error, want the read failure")
	}
	for _, want := range []string{path, "sudo"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to contain %q", err, want)
		}
	}
}
