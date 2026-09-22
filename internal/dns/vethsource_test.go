package dns

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// The veth listener must answer a query that the host sends to the address it listens on.
// The handler reads the source of the query, therefore this test runs one real query over
// a real socket rather than call the handler with a written address.
func TestTheVethListenerAnswersTheHostOverARealSocket(t *testing.T) {
	port := useFreePort(t)

	f := aliasForwarder(t)
	t.Cleanup(func() { _ = f.stopAllListeners() })

	if _, err := f.SyncListeners([]string{"127.0.0.1"}); err != nil {
		t.Fatalf("SyncListeners: %v", err)
	}

	client := &dns.Client{Timeout: 2 * time.Second}
	req := new(dns.Msg)
	req.SetQuestion("laptop.mmo.ts.internal.", dns.TypeA)

	var reply *dns.Msg
	var err error
	for i := 0; i < 20; i++ {
		reply, _, err = client.Exchange(req, net.JoinHostPort("127.0.0.1", fmt.Sprint(port)))
		if err == nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("the listener answered no query: %v", err)
	}
	if reply.Rcode == dns.RcodeRefused {
		t.Fatal("the listener refused the host, whose source is the listen address")
	}
	if len(reply.Answer) != 1 {
		t.Fatalf("the answer holds %d records, want 1", len(reply.Answer))
	}
}
