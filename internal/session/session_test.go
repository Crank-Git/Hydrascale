package session

import (
	"context"
	"errors"
	"testing"

	"hydrascale/internal/execx"
)

// ssOutput is the output of `ss -H -tna` on the test host at 192.168.1.221, with two rows
// added: one session from a peer of the tailnet jbones, and one session that the host
// opened to a peer of the same tailnet.
const ssOutput = `LISTEN 0      4096                    127.0.0.54:53            0.0.0.0:*
LISTEN 0      4096                       0.0.0.0:22            0.0.0.0:*
LISTEN 0      4096                     127.0.0.1:631           0.0.0.0:*
LISTEN 0      4096                 127.0.0.53%lo:53            0.0.0.0:*
ESTAB  0      0                    192.168.1.221:22      192.168.1.133:59934
ESTAB  0      0                    192.168.1.221:22       100.98.107.70:51402
ESTAB  0      0                    192.168.1.221:44210     100.82.3.90:22
LISTEN 0      4096                          [::]:22               [::]:*
`

// routeJSON is the output of `ip -json route get 100.98.107.70` on the test host.
const routeJSON = `[{"dst":"100.98.107.70","gateway":"10.200.0.86","dev":"vh5cde1b791fe1","prefsrc":"10.200.0.85","flags":[],"uid":1000,"cache":[]}]`

// lanRouteJSON is the output of `ip -json route get 192.168.1.133` on the test host.
const lanRouteJSON = `[{"dst":"192.168.1.133","dev":"enp1s0f0","prefsrc":"192.168.1.221","flags":[],"uid":1000,"cache":[]}]`

// devices maps the one tailnet of the test host to its host side veth device.
func devices() map[string]string {
	return map[string]string{"jbones": "vh5cde1b791fe1"}
}

func TestTheReaderRunsTheStatedCommands(t *testing.T) {
	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{Output: []byte(ssOutput)}, "ss", "-H", "-tna")
	rec.Script(execx.Result{Output: []byte(lanRouteJSON)}, "ip", "-json", "route", "get", "192.168.1.133")
	rec.Script(execx.Result{Output: []byte(routeJSON)}, "ip", "-json", "route", "get", "100.98.107.70")

	if _, err := NewReaderWith(rec).Paths(context.Background(), devices()); err != nil {
		t.Fatalf("Paths returned the error %v", err)
	}

	// The reader asks the kernel for the device of an inbound session only. The session on
	// the local port 44210 leaves the host, therefore the address 100.82.3.90 is absent.
	want := []execx.Call{
		{Name: "ss", Args: []string{"-H", "-tna"}},
		{Name: "ip", Args: []string{"-json", "route", "get", "192.168.1.133"}},
		{Name: "ip", Args: []string{"-json", "route", "get", "100.98.107.70"}},
	}
	got := rec.Calls()
	if len(got) != len(want) {
		t.Fatalf("the reader ran %d commands, want %d: %v", len(got), len(want), got)
	}
	for at, call := range got {
		if call.String() != want[at].String() {
			t.Errorf("command %d is %q, want %q", at, call.String(), want[at].String())
		}
	}
}

func TestTheReaderReportsThePathOfAnInboundSession(t *testing.T) {
	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{Output: []byte(ssOutput)}, "ss", "-H", "-tna")
	rec.Script(execx.Result{Output: []byte(lanRouteJSON)}, "ip", "-json", "route", "get", "192.168.1.133")
	rec.Script(execx.Result{Output: []byte(routeJSON)}, "ip", "-json", "route", "get", "100.98.107.70")

	paths, err := NewReaderWith(rec).Paths(context.Background(), devices())
	if err != nil {
		t.Fatalf("Paths returned the error %v", err)
	}

	// The session on the local port 22 arrives on a listening port, therefore the host
	// accepted it and the path is jbones to host. The session on the local port 44210
	// leaves the host, and the reader reports no path for it.
	want := []Path{{From: "jbones", To: "host"}}
	if len(paths) != len(want) || paths[0] != want[0] {
		t.Fatalf("Paths returned %v, want %v", paths, want)
	}
}

func TestTheReaderReportsNoPathWhenNoSessionArrivesOnATailnetDevice(t *testing.T) {
	const lanOnly = `LISTEN 0      4096                       0.0.0.0:22            0.0.0.0:*
ESTAB  0      0                    192.168.1.221:22      192.168.1.133:59934
`
	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{Output: []byte(lanOnly)}, "ss", "-H", "-tna")
	rec.Script(execx.Result{Output: []byte(lanRouteJSON)}, "ip", "-json", "route", "get", "192.168.1.133")

	paths, err := NewReaderWith(rec).Paths(context.Background(), devices())
	if err != nil {
		t.Fatalf("Paths returned the error %v", err)
	}
	if len(paths) != 0 {
		t.Fatalf("Paths returned %v, want no path", paths)
	}
}

func TestTheReaderReportsOnePathForTwoSessionsOnOneTailnet(t *testing.T) {
	const two = `LISTEN 0      4096                       0.0.0.0:22            0.0.0.0:*
ESTAB  0      0                    192.168.1.221:22       100.98.107.70:51402
ESTAB  0      0                    192.168.1.221:22       100.98.107.70:51403
`
	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{Output: []byte(two)}, "ss", "-H", "-tna")
	rec.Script(execx.Result{Output: []byte(routeJSON)}, "ip", "-json", "route", "get", "100.98.107.70")

	paths, err := NewReaderWith(rec).Paths(context.Background(), devices())
	if err != nil {
		t.Fatalf("Paths returned the error %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("Paths returned %v, want one path", paths)
	}
	// The reader asks the kernel once for one address, because a second question returns
	// the same answer.
	if got := len(rec.Calls()); got != 2 {
		t.Fatalf("the reader ran %d commands, want 2: %v", got, rec.Calls())
	}
}

func TestTheReaderReadsAListeningPortOnEveryLocalAddress(t *testing.T) {
	// The listening socket holds the address 100.72.254.115 and the session holds the
	// address 10.200.0.85, therefore a reader that matches the address reports no path.
	const other = `LISTEN 0      4096                100.72.254.115:59871         0.0.0.0:*
ESTAB  0      0                     10.200.0.85:59871      100.98.107.70:51402
`
	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{Output: []byte(other)}, "ss", "-H", "-tna")
	rec.Script(execx.Result{Output: []byte(routeJSON)}, "ip", "-json", "route", "get", "100.98.107.70")

	paths, err := NewReaderWith(rec).Paths(context.Background(), devices())
	if err != nil {
		t.Fatalf("Paths returned the error %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("Paths returned %v, want one path", paths)
	}
}

func TestTheReaderReturnsTheErrorOfTheSessionCommand(t *testing.T) {
	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{Output: []byte("ss: command not found"), Err: errors.New("exit status 127")}, "ss", "-H", "-tna")

	if _, err := NewReaderWith(rec).Paths(context.Background(), devices()); err == nil {
		t.Fatal("Paths returned no error for a command that failed")
	}
}

func TestTheReaderReturnsTheErrorOfTheRouteCommand(t *testing.T) {
	const one = `LISTEN 0      4096                       0.0.0.0:22            0.0.0.0:*
ESTAB  0      0                    192.168.1.221:22       100.98.107.70:51402
`
	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{Output: []byte(one)}, "ss", "-H", "-tna")
	rec.Script(execx.Result{Output: []byte("RTNETLINK answers: Network is unreachable"), Err: errors.New("exit status 2")},
		"ip", "-json", "route", "get", "100.98.107.70")

	if _, err := NewReaderWith(rec).Paths(context.Background(), devices()); err == nil {
		t.Fatal("Paths returned no error for a command that failed")
	}
}

// TestTheReaderReadsTheOutputOfTheTestHost holds the output that the test host at
// 192.168.1.221 wrote on 2026-08-05, word for word. A session inside the namespace
// ns-havoc reached the host on the veth device vh5cde1b791fe1, and the reader reports the
// path from the tailnet havoc to the host.
func TestTheReaderReadsTheOutputOfTheTestHost(t *testing.T) {
	const hostSS = `LISTEN 0      4096                       0.0.0.0:22            0.0.0.0:*    
ESTAB  0      0                      10.200.0.85:22        10.200.0.86:44060
`
	const hostRoute = `[{"dst":"10.200.0.86","dev":"vh5cde1b791fe1","prefsrc":"10.200.0.85","flags":[],"uid":1000,"cache":[]}]`

	rec := execx.NewRecorder(t)
	rec.Script(execx.Result{Output: []byte(hostSS)}, "ss", "-H", "-tna")
	rec.Script(execx.Result{Output: []byte(hostRoute)}, "ip", "-json", "route", "get", "10.200.0.86")

	paths, err := NewReaderWith(rec).Paths(context.Background(), map[string]string{"havoc": "vh5cde1b791fe1"})
	if err != nil {
		t.Fatalf("Paths returned the error %v", err)
	}
	want := Path{From: "havoc", To: "host"}
	if len(paths) != 1 || paths[0] != want {
		t.Fatalf("Paths returned %v, want %v", paths, want)
	}
}
