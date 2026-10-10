package access

import (
	"strings"
	"testing"
)

func TestParsePublishPort(t *testing.T) {
	t.Run("reads the protocol and the number of one port", func(t *testing.T) {
		got, err := ParsePublishPort("udp/53")
		if err != nil {
			t.Fatalf("ParsePublishPort: %v", err)
		}
		if got.Protocol != "udp" || got.Number != 53 {
			t.Errorf("ParsePublishPort(udp/53) = %+v, want udp and 53", got)
		}
	})

	t.Run("refuses a range", func(t *testing.T) {
		_, err := ParsePublishPort("tcp/22-23")
		if err == nil {
			t.Fatal("ParsePublishPort(tcp/22-23) returned no error, want a refusal of the range")
		}
		if !strings.Contains(err.Error(), "range") {
			t.Errorf("error = %q, want a message that names the range", err)
		}
	})

	t.Run("refuses an entry that is not of the form tcp/<n> or udp/<n>", func(t *testing.T) {
		for _, entry := range []string{"", "22", "icmp/1", "tcp/0", "tcp/65536", "tcp/ 22"} {
			if _, err := ParsePublishPort(entry); err == nil {
				t.Errorf("ParsePublishPort(%q) returned no error", entry)
			}
		}
	})
}

func TestRuleSetCoversHost(t *testing.T) {
	cases := []struct {
		name     string
		rules    []Rule
		protocol string
		number   int
		want     bool
	}{
		{"covers every port when the rule holds an empty port list",
			[]Rule{{From: "alpha", To: Host}}, "udp", 5353, true},
		{"covers a port that the rule names",
			[]Rule{{From: "alpha", To: Host, Ports: []string{"tcp/22"}}}, "tcp", 22, true},
		{"covers a port inside a range of the rule",
			[]Rule{{From: "alpha", To: Host, Ports: []string{"tcp/20-30"}}}, "tcp", 22, true},
		{"does not cover a port outside the ports of the rule",
			[]Rule{{From: "alpha", To: Host, Ports: []string{"tcp/20-21", "tcp/23"}}}, "tcp", 22, false},
		{"does not cover a port of another protocol",
			[]Rule{{From: "alpha", To: Host, Ports: []string{"udp/22"}}}, "tcp", 22, false},
		{"does not cover a port when the rule goes to another target",
			[]Rule{{From: "alpha", To: Internet}}, "tcp", 22, false},
		{"does not cover a port when the rule comes from another tailnet",
			[]Rule{{From: "beta", To: Host}}, "tcp", 22, false},
		{"does not cover a port when the rule set holds no rule",
			nil, "tcp", 22, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			set := RuleSet{Rules: tc.rules}
			if got := set.CoversHost("alpha", tc.protocol, tc.number); got != tc.want {
				t.Errorf("CoversHost(alpha, %s, %d) = %v, want %v", tc.protocol, tc.number, got, tc.want)
			}
		})
	}
}
