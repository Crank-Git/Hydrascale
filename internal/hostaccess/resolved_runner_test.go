package hostaccess

import (
	"strings"
	"testing"

	"hydrascale/internal/execx"
)

func TestRegisterDomainsRunsTheFullCommandListInOrder(t *testing.T) {
	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{}, "systemctl", "is-active", "--quiet", "systemd-resolved")
	rec.Script(execx.Result{}, "resolvectl", "domain", "lo", "~corp.ts.net")
	rec.Script(execx.Result{}, "resolvectl", "dns", "lo", "127.0.0.53:5354")

	rm := NewResolvedManager()
	rm.Runner = rec

	if err := rm.RegisterDomains([]string{"corp.ts.net"}); err != nil {
		t.Fatalf("RegisterDomains: %v", err)
	}

	want := []execx.Call{
		{Name: "systemctl", Args: []string{"is-active", "--quiet", "systemd-resolved"}},
		{Name: "resolvectl", Args: []string{"domain", "lo", "~corp.ts.net"}},
		{Name: "resolvectl", Args: []string{"dns", "lo", "127.0.0.53:5354"}},
	}
	got := rec.Calls()
	if len(got) != len(want) {
		t.Fatalf("RegisterDomains ran %d commands, want %d:\n%s", len(got), len(want), callList(got))
	}
	for i := range want {
		if got[i].String() != want[i].String() {
			t.Errorf("command %d = %q, want %q", i, got[i].String(), want[i].String())
		}
	}
}

func TestAMagicDNSSuffixThatIsNotADNSNameReachesNoCommand(t *testing.T) {
	const hostile = "-interface=eth0"

	rec := execx.NewRecorder(t)
	rm := NewResolvedManager()
	rm.Runner = rec

	err := rm.RegisterDomains([]string{hostile})
	if err == nil {
		t.Fatal("RegisterDomains returned no error for a suffix that is not a DNS name")
	}
	if !strings.Contains(err.Error(), hostile) {
		t.Errorf("the error names no rejected suffix: %v", err)
	}
	if calls := rec.Calls(); len(calls) != 0 {
		t.Errorf("RegisterDomains ran a command for a rejected suffix:\n%s", callList(calls))
	}
}

func TestValidDNSNameAcceptsASuffixAndRejectsOtherText(t *testing.T) {
	valid := []string{"corp.ts.net", "tail1234.ts.net", "example.com.", "a-b.example"}
	for _, d := range valid {
		if !validDNSName(d) {
			t.Errorf("validDNSName(%q) = false, want true", d)
		}
	}
	invalid := []string{"", ".", "-lo", "corp..ts.net", "-interface=eth0", "corp.ts.net/x", "a b.net"}
	for _, d := range invalid {
		if validDNSName(d) {
			t.Errorf("validDNSName(%q) = true, want false", d)
		}
	}
}
