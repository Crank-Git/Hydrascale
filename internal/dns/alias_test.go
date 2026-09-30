package dns

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// freePort returns a port that is free for UDP and for TCP on the loopback address.
// A test binds a fixed port at its own risk: another test run of the same host holds it
// and the test then fails for a reason that has nothing to do with the forwarder.
func freePort(t *testing.T) int {
	t.Helper()
	packet, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("open a free UDP port: %v", err)
	}
	port := packet.LocalAddr().(*net.UDPAddr).Port
	if err := packet.Close(); err != nil {
		t.Fatalf("close the free UDP port: %v", err)
	}
	stream, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", fmt.Sprint(port)))
	if err != nil {
		t.Skipf("the port %d is free for UDP and not for TCP: %v", port, err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("close the free TCP port: %v", err)
	}
	return port
}

// useFreePort points the alias listener at a free port for the duration of the test.
func useFreePort(t *testing.T) int {
	t.Helper()
	previous := aliasListenPort
	port := freePort(t)
	aliasListenPort = port
	t.Cleanup(func() { aliasListenPort = previous })
	return port
}

func testZones() map[string]AliasZone {
	return map[string]AliasZone{
		"mmo.ts.internal": {
			V4: map[string]string{"laptop": "100.64.0.1", "a.b": "100.64.0.3"},
			V6: map[string]string{"laptop": "fd7a:115c::1", "phone": "fd7a:115c::2"},
		},
		"psm.ts.internal": {
			V4: map[string]string{"server": "100.65.0.1"},
			V6: map[string]string{},
		},
	}
}

func TestAliasZoneName_joins_the_alias_and_the_reserved_suffix(t *testing.T) {
	if got, want := AliasZoneName("MMO"), "mmo.ts.internal"; got != want {
		t.Errorf("AliasZoneName = %q, want %q", got, want)
	}
}

func TestMatchAliasZone_reads_the_host_name_below_the_zone(t *testing.T) {
	zones := testZones()
	for _, tc := range []struct {
		qname    string
		wantHost string
		wantOK   bool
	}{
		{"laptop.mmo.ts.internal.", "laptop", true},
		{"LAPTOP.MMO.TS.INTERNAL.", "laptop", true},
		{"laptop.mmo.ts.internal", "laptop", true},
		{"server.psm.ts.internal.", "server", true},
		{"unknown.mmo.ts.internal.", "unknown", true},
		// The apex names no peer.
		{"mmo.ts.internal.", "", true},
		// A MagicDNS name holds more than one label below the suffix on some control
		// servers, and the alias zone keeps that shape.
		{"a.b.mmo.ts.internal.", "a.b", true},
		// A name outside every zone belongs to the forwarding path.
		{"laptop.tail1234.ts.net.", "", false},
		{"ts.internal.", "", false},
		{"example.com.", "", false},
	} {
		_, host, ok := matchAliasZone(zones, tc.qname)
		if ok != tc.wantOK || host != tc.wantHost {
			t.Errorf("matchAliasZone(%q) = (%q, %v), want (%q, %v)", tc.qname, host, ok, tc.wantHost, tc.wantOK)
		}
	}
}

// answerWriter records the message that answerAlias writes.
type answerWriter struct {
	dns.ResponseWriter
	msg    *dns.Msg
	remote net.Addr
}

func (w *answerWriter) RemoteAddr() net.Addr { return w.remote }

func (w *answerWriter) WriteMsg(m *dns.Msg) error {
	w.msg = m
	return nil
}

// ask returns the answer that the forwarder writes for one question.
func ask(t *testing.T, f *Forwarder, qname string, qtype uint16) *dns.Msg {
	t.Helper()
	req := new(dns.Msg)
	req.SetQuestion(dns.Fqdn(qname), qtype)
	w := &answerWriter{}
	f.handleDNSRequest(w, req)
	if w.msg == nil {
		t.Fatalf("the forwarder wrote no answer for %s", qname)
	}
	return w.msg
}

func aliasForwarder(t *testing.T) *Forwarder {
	t.Helper()
	f, err := NewForwarder([]string{"192.0.2.1"}, time.Second, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f.SetAliasZones(testZones())
	return f
}

func TestTheForwarderAnswersAnAliasNameWithItsAddress(t *testing.T) {
	f := aliasForwarder(t)

	msg := ask(t, f, "laptop.mmo.ts.internal", dns.TypeA)
	if msg.Rcode != dns.RcodeSuccess {
		t.Fatalf("rcode = %s, want NOERROR", dns.RcodeToString[msg.Rcode])
	}
	if !msg.Authoritative {
		t.Error("the answer is not authoritative, and the forwarder owns the zone")
	}
	if len(msg.Answer) != 1 {
		t.Fatalf("the answer holds %d records, want 1", len(msg.Answer))
	}
	a, ok := msg.Answer[0].(*dns.A)
	if !ok {
		t.Fatalf("the record is %T, want *dns.A", msg.Answer[0])
	}
	if !a.A.Equal(net.ParseIP("100.64.0.1")) {
		t.Errorf("the address is %s, want 100.64.0.1", a.A)
	}
	if a.Hdr.Name != "laptop.mmo.ts.internal." {
		t.Errorf("the owner name is %q, want the name of the question", a.Hdr.Name)
	}
}

func TestTheForwarderAnswersAnAliasNameWithItsIPv6Address(t *testing.T) {
	f := aliasForwarder(t)
	msg := ask(t, f, "phone.mmo.ts.internal", dns.TypeAAAA)
	if len(msg.Answer) != 1 {
		t.Fatalf("the answer holds %d records, want 1", len(msg.Answer))
	}
	if _, ok := msg.Answer[0].(*dns.AAAA); !ok {
		t.Fatalf("the record is %T, want *dns.AAAA", msg.Answer[0])
	}
}

func TestTheForwarderReportsThatAnUnknownAliasNameDoesNotExist(t *testing.T) {
	f := aliasForwarder(t)
	msg := ask(t, f, "absent.mmo.ts.internal", dns.TypeA)
	if msg.Rcode != dns.RcodeNameError {
		t.Errorf("rcode = %s, want NXDOMAIN", dns.RcodeToString[msg.Rcode])
	}
}

func TestTheForwarderAnswersNoRecordForAKnownNameOfAnotherType(t *testing.T) {
	f := aliasForwarder(t)
	// The zone holds no IPv4 address for phone.
	msg := ask(t, f, "phone.mmo.ts.internal", dns.TypeA)
	if msg.Rcode != dns.RcodeSuccess {
		t.Errorf("rcode = %s, want NOERROR", dns.RcodeToString[msg.Rcode])
	}
	if len(msg.Answer) != 0 {
		t.Errorf("the answer holds %d records, want 0", len(msg.Answer))
	}
}

func TestTheForwarderAnswersNoRecordForTheApexOfAZone(t *testing.T) {
	f := aliasForwarder(t)
	msg := ask(t, f, "mmo.ts.internal", dns.TypeA)
	if msg.Rcode != dns.RcodeSuccess {
		t.Errorf("rcode = %s, want NOERROR", dns.RcodeToString[msg.Rcode])
	}
	if len(msg.Answer) != 0 {
		t.Errorf("the answer holds %d records, want 0", len(msg.Answer))
	}
}

// A name below the zone that the zone does not hold must not reach an upstream server.
// The upstream address is in the range that RFC 5737 reserves for documentation, therefore
// a query that leaves the host fails after the client timeout rather than answer.
// NXDOMAIN inside the timeout proves that the forwarder answered the name itself.
func TestTheForwarderSendsNoAliasQueryToAnUpstreamServer(t *testing.T) {
	f := aliasForwarder(t)
	done := make(chan *dns.Msg, 1)
	go func() { done <- ask(t, f, "absent.mmo.ts.internal", dns.TypeA) }()
	select {
	case msg := <-done:
		if msg.Rcode != dns.RcodeNameError {
			t.Errorf("rcode = %s, want NXDOMAIN", dns.RcodeToString[msg.Rcode])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the forwarder sent the alias query to the upstream server")
	}
}

// A MagicDNS name of several labels answers under the alias zone with the same shape.
func TestTheForwarderAnswersANestedAliasName(t *testing.T) {
	f := aliasForwarder(t)
	msg := ask(t, f, "a.b.mmo.ts.internal", dns.TypeA)
	if msg.Rcode != dns.RcodeSuccess {
		t.Fatalf("rcode = %s, want NOERROR", dns.RcodeToString[msg.Rcode])
	}
	if len(msg.Answer) != 1 {
		t.Fatalf("the answer holds %d records, want 1", len(msg.Answer))
	}
	a, ok := msg.Answer[0].(*dns.A)
	if !ok {
		t.Fatalf("the record is %T, want *dns.A", msg.Answer[0])
	}
	if !a.A.Equal(net.ParseIP("100.64.0.3")) {
		t.Errorf("the address is %s, want 100.64.0.3", a.A)
	}
}

// A name below the zone that names no peer does not exist, whatever its label count.
func TestTheForwarderReportsThatAnUnknownNestedNameDoesNotExist(t *testing.T) {
	f := aliasForwarder(t)
	msg := ask(t, f, "x.y.mmo.ts.internal", dns.TypeA)
	if msg.Rcode != dns.RcodeNameError {
		t.Errorf("rcode = %s, want NXDOMAIN", dns.RcodeToString[msg.Rcode])
	}
}

func TestSyncListenersStartsAndStopsAListener(t *testing.T) {
	port := useFreePort(t)
	bind := net.JoinHostPort("127.0.0.1", fmt.Sprint(port))

	f := aliasForwarder(t)
	t.Cleanup(func() { _ = f.stopAllListeners() })

	if _, err := f.SyncListeners([]string{"127.0.0.1"}); err != nil {
		t.Fatalf("SyncListeners returned %v", err)
	}
	if len(f.listeners) != 1 {
		t.Fatalf("the forwarder holds %d listeners, want 1", len(f.listeners))
	}

	// The listener answers the alias zone on its own address.
	client := &dns.Client{Timeout: 2 * time.Second}
	req := new(dns.Msg)
	req.SetQuestion("laptop.mmo.ts.internal.", dns.TypeA)
	var reply *dns.Msg
	var err error
	for i := 0; i < 20; i++ {
		reply, _, err = client.Exchange(req, bind)
		if err == nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("the listener answered no query: %v", err)
	}
	if len(reply.Answer) != 1 {
		t.Fatalf("the listener answered %d records, want 1", len(reply.Answer))
	}

	// A second call with the same address changes nothing.
	if _, err := f.SyncListeners([]string{"127.0.0.1"}); err != nil {
		t.Fatalf("the second SyncListeners returned %v", err)
	}
	if len(f.listeners) != 1 {
		t.Errorf("the forwarder holds %d listeners after a repeat, want 1", len(f.listeners))
	}

	// An empty list stops the listener.
	if _, err := f.SyncListeners(nil); err != nil {
		t.Fatalf("SyncListeners(nil) returned %v", err)
	}
	if len(f.listeners) != 0 {
		t.Errorf("the forwarder holds %d listeners after the stop, want 0", len(f.listeners))
	}
}

func TestSyncListenersRejectsAnAddressThatIsNotAnIPAddress(t *testing.T) {
	f := aliasForwarder(t)
	if _, err := f.SyncListeners([]string{"not-an-address"}); err == nil {
		t.Fatal("SyncListeners returned no error for a name that is not an IP address")
	}
}

// A listener that cannot bind must not enter the listener table, because the caller reads
// that table to decide whether the link of the tailnet names the host side address.
// SyncListeners must retry such an address on the next call.
func TestSyncListenersReportsAnAddressThatCannotBind(t *testing.T) {
	port := useFreePort(t)

	// Another process of the host already holds the port.
	blocker, err := net.ListenPacket("udp", net.JoinHostPort("127.0.0.1", fmt.Sprint(port)))
	if err != nil {
		t.Skipf("the test cannot hold the port %d: %v", port, err)
	}

	f := aliasForwarder(t)
	t.Cleanup(func() { _ = f.stopAllListeners() })

	live, err := f.SyncListeners([]string{"127.0.0.1"})
	if err == nil {
		t.Error("SyncListeners reported no error, and the address was already in use")
	}
	if live["127.0.0.1"] {
		t.Error("SyncListeners reports the address as live, and nothing bound it")
	}
	if len(f.listeners) != 0 {
		t.Errorf("the forwarder holds %d listeners, want 0 after a failed bind", len(f.listeners))
	}

	// The port frees, and the next call binds it, therefore the failure is not permanent.
	if err := blocker.Close(); err != nil {
		t.Fatal(err)
	}
	live, err = f.SyncListeners([]string{"127.0.0.1"})
	if err != nil {
		t.Fatalf("the retry returned %v", err)
	}
	if !live["127.0.0.1"] {
		t.Error("the retry did not bind the address that is free now")
	}
}

// The veth address carries the traffic of the namespace, therefore the listener answers
// the host alone. The host holds the veth address; the namespace holds the other end.
func TestTheVethListenerRefusesAQueryFromTheNamespace(t *testing.T) {
	f := aliasForwarder(t)
	held, err := readHostAddrs()
	if err != nil {
		t.Fatalf("readHostAddrs: %v", err)
	}
	f.hostAddrs = held
	handler := f.vethHandler("127.0.0.1")

	req := new(dns.Msg)
	req.SetQuestion("laptop.mmo.ts.internal.", dns.TypeA)

	// 203.0.113.9 belongs to the range that RFC 5737 reserves for documentation, so no
	// interface of a test host holds it. It stands for the address inside a namespace.
	from := &answerWriter{remote: &net.UDPAddr{IP: net.ParseIP("203.0.113.9"), Port: 5353}}
	handler(from, req)
	if from.msg == nil {
		t.Fatal("the handler wrote no answer")
	}
	if from.msg.Rcode != dns.RcodeRefused {
		t.Errorf("rcode = %s for a query of another host, want REFUSED",
			dns.RcodeToString[from.msg.Rcode])
	}

	host := &answerWriter{remote: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5353}}
	handler(host, req)
	if host.msg == nil {
		t.Fatal("the handler wrote no answer for the host")
	}
	if len(host.msg.Answer) != 1 {
		t.Errorf("the answer to the host holds %d records, want 1", len(host.msg.Answer))
	}
}

// The host holds one end of every veth pair, therefore a query from a veth address is a
// query of the host. This is the address that systemd-resolved sends from.
func TestSourceIsHostAcceptsEveryAddressOfTheHost(t *testing.T) {
	f := aliasForwarder(t)
	held, err := readHostAddrs()
	if err != nil {
		t.Fatalf("readHostAddrs: %v", err)
	}
	f.hostAddrs = held
	if len(held) == 0 {
		t.Skip("the test host reports no address")
	}

	for addr := range held {
		ip := net.ParseIP(addr)
		if ip == nil || ip.IsLinkLocalUnicast() {
			continue
		}
		if !f.sourceIsHost(&net.UDPAddr{IP: ip, Port: 5353}) {
			t.Errorf("sourceIsHost refused %s, which the host holds", addr)
		}
	}

	if f.sourceIsHost(&net.UDPAddr{IP: net.ParseIP("203.0.113.9"), Port: 53}) {
		t.Error("sourceIsHost accepted 203.0.113.9, which no interface of the host holds")
	}
	if f.sourceIsHost(nil) {
		t.Error("sourceIsHost accepted a query with no source address")
	}
	if !f.sourceIsHost(&net.UDPAddr{IP: net.ParseIP("127.0.0.53"), Port: 53}) {
		t.Error("sourceIsHost refused a loopback address, which is the host")
	}
}
