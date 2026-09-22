package hostaccess

import (
	"strings"
	"testing"

	"hydrascale/internal/execx"
)

func TestRegisterDomainsRunsTheFullCommandListInOrder(t *testing.T) {
	// The server comes before the domain. A link that holds a domain and no server sends
	// every query of that domain to a resolver that does not exist.
	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{}, "systemctl", "is-active", "--quiet", "systemd-resolved")
	rec.Script(execx.Result{}, "resolvectl", "dns", "vhcorp", "10.200.1.46")
	rec.Script(execx.Result{}, "resolvectl", "domain", "vhcorp", "~corp.ts.net")

	rm := NewResolvedManager()
	rm.Runner = rec

	links := []Link{{Device: "vhcorp", Address: "10.200.1.46", Domain: "corp.ts.net"}}
	if err := rm.RegisterDomains(links); err != nil {
		t.Fatalf("RegisterDomains: %v", err)
	}

	want := []execx.Call{
		{Name: "systemctl", Args: []string{"is-active", "--quiet", "systemd-resolved"}},
		{Name: "resolvectl", Args: []string{"dns", "vhcorp", "10.200.1.46"}},
		{Name: "resolvectl", Args: []string{"domain", "vhcorp", "~corp.ts.net"}},
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

	err := rm.RegisterDomains([]Link{{Device: "vhcorp", Address: "10.200.1.46", Domain: hostile}})
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

func TestRegisterDomainsRegistersEachTailnetOnItsOwnDevice(t *testing.T) {
	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{}, "systemctl", "is-active", "--quiet", "systemd-resolved")
	rec.Script(execx.Result{}, "resolvectl", "dns", "vhone", "10.200.1.46")
	rec.Script(execx.Result{}, "resolvectl", "domain", "vhone", "~one.ts.net")
	rec.Script(execx.Result{}, "resolvectl", "dns", "vhtwo", "10.200.2.202")
	rec.Script(execx.Result{}, "resolvectl", "domain", "vhtwo", "~two.ts.net")

	rm := NewResolvedManager()
	rm.Runner = rec

	links := []Link{
		{Device: "vhone", Address: "10.200.1.46", Domain: "one.ts.net"},
		{Device: "vhtwo", Address: "10.200.2.202", Domain: "two.ts.net"},
	}
	if err := rm.RegisterDomains(links); err != nil {
		t.Fatalf("RegisterDomains: %v", err)
	}
	if calls := rec.Calls(); len(calls) != 5 {
		t.Fatalf("RegisterDomains ran %d commands, want 5:\n%s", len(calls), callList(calls))
	}
}

func TestRegisterDomainsRejectsADeviceThatResolvectlReadsAsAnOption(t *testing.T) {
	rec := execx.NewRecorder(t)
	rm := NewResolvedManager()
	rm.Runner = rec

	links := []Link{{Device: "-interface=eth0", Address: "10.200.1.46", Domain: "corp.ts.net"}}
	if err := rm.RegisterDomains(links); err == nil {
		t.Fatal("RegisterDomains returned no error for a device that is not a device name")
	}
	if calls := rec.Calls(); len(calls) != 0 {
		t.Errorf("RegisterDomains ran a command for a rejected device:\n%s", callList(calls))
	}
}

func TestRegisterDomainsRejectsAnAddressThatIsNotAnIPAddress(t *testing.T) {
	rec := execx.NewRecorder(t)
	rm := NewResolvedManager()
	rm.Runner = rec

	links := []Link{{Device: "vhcorp", Address: "127.0.0.53:5354", Domain: "corp.ts.net"}}
	if err := rm.RegisterDomains(links); err == nil {
		t.Fatal("RegisterDomains returned no error for an address that holds a port")
	}
	if calls := rec.Calls(); len(calls) != 0 {
		t.Errorf("RegisterDomains ran a command for a rejected address:\n%s", callList(calls))
	}
}

func TestDeregisterAllRevertsEveryDeviceThatItRegistered(t *testing.T) {
	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{}, "systemctl", "is-active", "--quiet", "systemd-resolved")
	rec.Script(execx.Result{}, "resolvectl", "dns", "vhone", "10.200.1.46")
	rec.Script(execx.Result{}, "resolvectl", "domain", "vhone", "~one.ts.net")
	rec.Script(execx.Result{}, "resolvectl", "dns", "vhtwo", "10.200.2.202")
	rec.Script(execx.Result{}, "resolvectl", "domain", "vhtwo", "~two.ts.net")
	rec.Script(execx.Result{}, "resolvectl", "revert", "vhone")
	rec.Script(execx.Result{}, "resolvectl", "revert", "vhtwo")

	rm := NewResolvedManager()
	rm.Runner = rec

	links := []Link{
		{Device: "vhone", Address: "10.200.1.46", Domain: "one.ts.net"},
		{Device: "vhtwo", Address: "10.200.2.202", Domain: "two.ts.net"},
	}
	if err := rm.RegisterDomains(links); err != nil {
		t.Fatalf("RegisterDomains: %v", err)
	}
	if err := rm.DeregisterAll(); err != nil {
		t.Fatalf("DeregisterAll: %v", err)
	}

	calls := rec.Calls()
	last := calls[len(calls)-2:]
	if last[0].String() != "resolvectl revert vhone" || last[1].String() != "resolvectl revert vhtwo" {
		t.Errorf("the revert commands are %q and %q, want one for each device", last[0].String(), last[1].String())
	}
}
