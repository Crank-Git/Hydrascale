package reconciler

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"hydrascale/internal/config"
	"hydrascale/internal/namespaces"
)

// publishCall holds the arguments that the reconciler gave the host access setup.
type publishCall struct {
	nsName  string
	publish []string
	ipv6    bool
}

// writePublishConfig writes a configuration file that holds the tailnet corp with host
// access, the published port tcp/22, and the local rule that the port needs.
func writePublishConfig(t *testing.T) string {
	t.Helper()
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")

	on := true
	cfg := config.DefaultConfig()
	cfg.Tailnets = append(cfg.Tailnets, config.Tailnet{ID: "corp", HostAccess: &on, Publish: []string{"tcp/22"}})
	if err := config.SaveConfig(cfgPath, cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	existing, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	body := "access:\n  rules:\n    - from: corp\n      to: host\n"
	if err := os.WriteFile(cfgPath, append(existing, []byte(body)...), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return cfgPath
}

// newPublishReconciler returns a Reconciler with a host access manager that runs no
// command, and a setup function that records each call and returns setupErr.
func newPublishReconciler(t *testing.T, setupErr error) (*Reconciler, *[]publishCall) {
	t.Helper()
	dm := newMockDaemon()
	dm.statusResult = statusWithPeer("corp.ts.net", "peer", "100.64.0.2")
	r := New(writePublishConfig(t), newMockNS(), dm, newMockRouting(), time.Second, newHostAccessManager(t), "10.200.0.0/16")

	var calls []publishCall
	r.setupHostAccess = func(nsName string, index int, infraSubnet string, publish []string, ipv6 bool) error {
		calls = append(calls, publishCall{nsName: nsName, publish: publish, ipv6: ipv6})
		return setupErr
	}
	return r, &calls
}

func TestTheHostAccessSyncPassesThePublishListOfTheTailnet(t *testing.T) {
	r, calls := newPublishReconciler(t, nil)

	if err := r.executeAction(Action{Type: ActionSyncHostAccess, TailnetID: "corp"}); err != nil {
		t.Fatalf("executeAction: %v", err)
	}

	if len(*calls) != 1 || !slices.Equal((*calls)[0].publish, []string{"tcp/22"}) {
		t.Errorf("the setup calls are %+v, want one call with [tcp/22]", *calls)
	}
}

func TestTheHostAccessSyncPassesTheIPv6StateOfTheHost(t *testing.T) {
	for _, tc := range []struct {
		state string
		want  bool
	}{
		{"", false},
		{"off: the kernel holds no force_forwarding", false},
		{"on: " + ipv6ForceForwarding, true},
	} {
		r, calls := newPublishReconciler(t, nil)
		r.ipv6State = tc.state

		if err := r.executeAction(Action{Type: ActionSyncHostAccess, TailnetID: "corp"}); err != nil {
			t.Fatalf("executeAction: %v", err)
		}
		if len(*calls) != 1 || (*calls)[0].ipv6 != tc.want {
			t.Errorf("IPv6 state %q: the setup calls are %+v, want ipv6 %v", tc.state, *calls, tc.want)
		}
	}
}

func TestTheHostAccessSyncRecordsAccessWriteFailedWhenAPublishedPortRuleFails(t *testing.T) {
	failure := fmt.Errorf("%w in ns-corp: ip6tables: No such file or directory", namespaces.ErrPublishedPorts)
	r, _ := newPublishReconciler(t, failure)

	if err := r.executeAction(Action{Type: ActionSyncHostAccess, TailnetID: "corp"}); err != nil {
		t.Fatalf("executeAction returned %v; a published port failure stops no other step", err)
	}

	found := false
	for _, e := range r.Events() {
		if e.Type == "access.write_failed" && e.TailnetID == "corp" {
			found = true
		}
	}
	if !found {
		t.Errorf("the event list holds no access.write_failed for corp: %v", r.Events())
	}
	if !r.hostAccessRules["corp"] {
		t.Error("the reconciler did not mark the host access rules of corp as present")
	}
}

func TestTheHostAccessSyncReturnsAnErrorThatIsNotAPublishedPortFailure(t *testing.T) {
	r, _ := newPublishReconciler(t, errors.New("host-access: enable forwarding in ns-corp: exit status 1"))

	if err := r.executeAction(Action{Type: ActionSyncHostAccess, TailnetID: "corp"}); err == nil {
		t.Fatal("executeAction returned no error for a failed forwarding write")
	}
	if hasEvent(r, "access.write_failed", "") {
		t.Errorf("the reconciler recorded access.write_failed for a failure that is not a published port: %v", r.Events())
	}
}

func TestTheNamespaceCreateRecordsAccessWriteFailedWhenAPublishedPortRuleFails(t *testing.T) {
	failure := fmt.Errorf("%w in ns-corp: iptables: Permission denied", namespaces.ErrPublishedPorts)
	r, calls := newPublishReconciler(t, failure)

	if err := r.executeAction(Action{Type: ActionCreateNS, TailnetID: "corp"}); err != nil {
		t.Fatalf("executeAction: %v", err)
	}

	if len(*calls) != 1 || !slices.Equal((*calls)[0].publish, []string{"tcp/22"}) {
		t.Errorf("the setup calls are %+v, want one call with [tcp/22]", *calls)
	}
	if !hasEvent(r, "access.write_failed", "Permission denied") {
		t.Errorf("the event list holds no access.write_failed: %v", r.Events())
	}
}
