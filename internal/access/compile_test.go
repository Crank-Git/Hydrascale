package access

import (
	"net"
	"strings"
	"testing"
)

// wantExclusions holds the matches that keep an internet rule off the private ranges and
// off the host local network. SA-9 is the finding that these matches close.
const wantExclusions = "-m iprange ! --dst-range 10.0.0.0-10.255.255.255 " +
	"-m iprange ! --dst-range 172.16.0.0-172.31.255.255 " +
	"-m iprange ! --dst-range 192.168.0.0-192.168.255.255 " +
	"-m iprange ! --dst-range 169.254.0.0-169.254.255.255 " +
	"-m iprange ! --dst-range 127.0.0.0-127.255.255.255"

// destinationMatches reports whether a packet to addr passes every destination match of
// one compiled rule. The test reads the compiled argument list, because this issue runs
// no host command.
func destinationMatches(rule []string, addr string) bool {
	target := net.ParseIP(addr).To4()
	for i := 0; i+3 < len(rule); i++ {
		if rule[i] != "-m" || rule[i+1] != "iprange" || rule[i+2] != "!" || rule[i+3] != "--dst-range" {
			continue
		}
		bounds := strings.SplitN(rule[i+4], "-", 2)
		low := net.ParseIP(bounds[0]).To4()
		high := net.ParseIP(bounds[1]).To4()
		if bytesCompare(target, low) >= 0 && bytesCompare(target, high) <= 0 {
			return false
		}
	}
	return true
}

// bytesCompare returns -1, 0, or 1 for two IPv4 addresses in network order.
func bytesCompare(a, b net.IP) int {
	for i := range a {
		switch {
		case a[i] < b[i]:
			return -1
		case a[i] > b[i]:
			return 1
		}
	}
	return 0
}

// testTopology returns two tailnets and the default DNS forwarder bind address.
func testTopology() Topology {
	return Topology{
		Devices: map[string]string{
			"alpha": "vh0a0a0a0a0a0a",
			"beta":  "vh0b0b0b0b0b0b",
		},
		DNSAddress: "127.0.0.53:5354",
	}
}

// lines returns one string per compiled rule, so a failure prints the rule.
func lines(rules [][]string) []string {
	out := make([]string, 0, len(rules))
	for _, r := range rules {
		out = append(out, strings.Join(r, " "))
	}
	return out
}

func equalLines(t *testing.T, got [][]string, want []string) {
	t.Helper()
	gotLines := lines(got)
	if len(gotLines) != len(want) {
		t.Fatalf("got %d rules, want %d\ngot:\n%s\nwant:\n%s",
			len(gotLines), len(want), strings.Join(gotLines, "\n"), strings.Join(want, "\n"))
	}
	for i := range want {
		if gotLines[i] != want[i] {
			t.Errorf("rule %d = %q, want %q", i, gotLines[i], want[i])
		}
	}
}

func TestCompile(t *testing.T) {
	t.Run("produces the exact chain for a fixed rule set", func(t *testing.T) {
		set := RuleSet{
			Mode: ModeEnforce,
			Rules: []Rule{
				{From: "alpha", To: "beta", Ports: []string{"tcp/22"}},
				{From: "alpha", To: Internet},
				{From: "beta", To: Host, Ports: []string{"udp/53", "tcp/8000-8080"}},
				{From: Host, To: "alpha"},
			},
		}

		compiled, err := Compile(set, testTopology(), EnforceTail)
		if err != nil {
			t.Fatalf("Compile returned %v, want no error", err)
		}

		equalLines(t, compiled.Forward, []string{
			"-A HYDRASCALE-FWD -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT",
			"-A HYDRASCALE-FWD -i vh0a0a0a0a0a0a -o vh0b0b0b0b0b0b -p tcp --dport 22 -j ACCEPT",
			"-A HYDRASCALE-FWD -i vh0a0a0a0a0a0a ! -o vh+ " + wantExclusions + " -j ACCEPT",
			"-A HYDRASCALE-FWD -j DROP",
		})
		equalLines(t, compiled.Out, []string{
			"-A HYDRASCALE-OUT -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT",
			"-A HYDRASCALE-OUT -i vh0a0a0a0a0a0a -d 127.0.0.53 -p udp --dport 5354 -j ACCEPT",
			"-A HYDRASCALE-OUT -i vh0a0a0a0a0a0a -d 127.0.0.53 -p tcp --dport 5354 -j ACCEPT",
			"-A HYDRASCALE-OUT -i vh0b0b0b0b0b0b -d 127.0.0.53 -p udp --dport 5354 -j ACCEPT",
			"-A HYDRASCALE-OUT -i vh0b0b0b0b0b0b -d 127.0.0.53 -p tcp --dport 5354 -j ACCEPT",
			"-A HYDRASCALE-OUT -i vh0b0b0b0b0b0b -p udp --dport 53 -j ACCEPT",
			"-A HYDRASCALE-OUT -i vh0b0b0b0b0b0b -p tcp --dport 8000:8080 -j ACCEPT",
			"-A HYDRASCALE-OUT -i vh0a0a0a0a0a0a -j DROP",
			"-A HYDRASCALE-OUT -i vh0b0b0b0b0b0b -j DROP",
		})
	})

	t.Run("opens each chain with the established-connection rule", func(t *testing.T) {
		compiled, err := Compile(RuleSet{}, testTopology(), EnforceTail)
		if err != nil {
			t.Fatalf("Compile returned %v, want no error", err)
		}
		want := "-m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT"
		if first := strings.Join(compiled.Forward[0], " "); first != "-A "+ChainForward+" "+want {
			t.Errorf("first forward rule = %q, want the established-connection rule", first)
		}
		if first := strings.Join(compiled.Out[0], " "); first != "-A "+ChainOut+" "+want {
			t.Errorf("first out rule = %q, want the established-connection rule", first)
		}
	})

	t.Run("holds an accept rule for the DNS forwarder bind address", func(t *testing.T) {
		compiled, err := Compile(RuleSet{}, testTopology(), EnforceTail)
		if err != nil {
			t.Fatalf("Compile returned %v, want no error", err)
		}
		want := "-A HYDRASCALE-OUT -i vh0a0a0a0a0a0a -d 127.0.0.53 -p udp --dport 5354 -j ACCEPT"
		found := false
		for _, line := range lines(compiled.Out) {
			if line == want {
				found = true
			}
		}
		if !found {
			t.Errorf("the out chain holds no rule %q\ngot:\n%s", want, strings.Join(lines(compiled.Out), "\n"))
		}
	})

	t.Run("produces a rule with no protocol match for an empty port list", func(t *testing.T) {
		set := RuleSet{Rules: []Rule{{From: "alpha", To: "beta"}}}
		compiled, err := Compile(set, testTopology(), EnforceTail)
		if err != nil {
			t.Fatalf("Compile returned %v, want no error", err)
		}
		want := "-A HYDRASCALE-FWD -i vh0a0a0a0a0a0a -o vh0b0b0b0b0b0b -j ACCEPT"
		if got := strings.Join(compiled.Forward[1], " "); got != want {
			t.Errorf("rule = %q, want %q", got, want)
		}
		for _, arg := range compiled.Forward[1] {
			if arg == "-p" {
				t.Errorf("rule %q holds a -p match, want none", want)
			}
		}
	})

	t.Run("excludes every private range from an internet rule", func(t *testing.T) {
		set := RuleSet{Rules: []Rule{{From: "alpha", To: Internet}}}
		compiled, err := Compile(set, testTopology(), EnforceTail)
		if err != nil {
			t.Fatalf("Compile returned %v, want no error", err)
		}
		want := "-A HYDRASCALE-FWD -i vh0a0a0a0a0a0a ! -o vh+ " + wantExclusions + " -j ACCEPT"
		if got := strings.Join(compiled.Forward[1], " "); got != want {
			t.Errorf("rule = %q, want %q", got, want)
		}
	})

	t.Run("matches a public address and not the host local network", func(t *testing.T) {
		set := RuleSet{Rules: []Rule{{From: "alpha", To: Internet}}}
		compiled, err := Compile(set, testTopology(), EnforceTail)
		if err != nil {
			t.Fatalf("Compile returned %v, want no error", err)
		}
		rule := compiled.Forward[1]
		for _, addr := range []string{"93.184.216.34", "1.1.1.1", "100.64.0.1"} {
			if !destinationMatches(rule, addr) {
				t.Errorf("the internet rule excludes %s, want a match", addr)
			}
		}
		for _, addr := range []string{"192.168.1.215", "10.0.0.1", "172.16.0.1", "169.254.1.1", "127.0.0.1"} {
			if destinationMatches(rule, addr) {
				t.Errorf("the internet rule matches %s, want an exclusion", addr)
			}
		}
	})

	t.Run("returns the same output for the same input", func(t *testing.T) {
		set := RuleSet{Rules: []Rule{
			{From: "alpha", To: "beta", Ports: []string{"tcp/22"}},
			{From: "beta", To: Internet},
			{From: "alpha", To: Host, Ports: []string{"udp/53"}},
		}}
		first, err := Compile(set, testTopology(), EnforceTail)
		if err != nil {
			t.Fatalf("Compile returned %v, want no error", err)
		}
		second, err := Compile(set, testTopology(), EnforceTail)
		if err != nil {
			t.Fatalf("Compile returned %v, want no error", err)
		}
		equalLines(t, second.Forward, lines(first.Forward))
		equalLines(t, second.Out, lines(first.Out))
	})

	t.Run("rejects a rule where from equals to", func(t *testing.T) {
		set := RuleSet{Rules: []Rule{{From: "alpha", To: "alpha"}}}
		_, err := Compile(set, testTopology(), EnforceTail)
		if err == nil {
			t.Fatal("Compile returned no error, want an error")
		}
		if !strings.Contains(err.Error(), "alpha") {
			t.Errorf("error = %q, want the identifier alpha in the message", err)
		}
	})

	t.Run("rejects a rule that names a tailnet that the configuration does not declare", func(t *testing.T) {
		set := RuleSet{Rules: []Rule{{From: "alpha", To: "gamma"}}}
		_, err := Compile(set, testTopology(), EnforceTail)
		if err == nil {
			t.Fatal("Compile returned no error, want an error")
		}
		if !strings.Contains(err.Error(), "gamma") {
			t.Errorf("error = %q, want the identifier gamma in the message", err)
		}
	})

	t.Run("rejects the literal internet as a source", func(t *testing.T) {
		set := RuleSet{Rules: []Rule{{From: Internet, To: "alpha"}}}
		if _, err := Compile(set, testTopology(), EnforceTail); err == nil {
			t.Fatal("Compile returned no error, want an error")
		}
	})

	t.Run("rejects an invalid port entry", func(t *testing.T) {
		for _, port := range []string{"tcp/0", "tcp/65536", "tcp/22-21", "sctp/22", "22", "tcp/", "tcp/a"} {
			set := RuleSet{Rules: []Rule{{From: "alpha", To: "beta", Ports: []string{port}}}}
			if _, err := Compile(set, testTopology(), EnforceTail); err == nil {
				t.Errorf("Compile accepted the port %q, want an error", port)
			}
		}
	})

	t.Run("accepts a valid port entry", func(t *testing.T) {
		for _, port := range []string{"tcp/1", "tcp/65535", "udp/53", "tcp/22-23", "udp/1-65535"} {
			set := RuleSet{Rules: []Rule{{From: "alpha", To: "beta", Ports: []string{port}}}}
			if _, err := Compile(set, testTopology(), EnforceTail); err != nil {
				t.Errorf("Compile rejected the port %q with %v, want no error", port, err)
			}
		}
	})

	t.Run("rejects a mode that the daemon does not run", func(t *testing.T) {
		set := RuleSet{Mode: "audit"}
		if _, err := Compile(set, testTopology(), EnforceTail); err == nil {
			t.Fatal("Compile returned no error, want an error")
		}
	})

	t.Run("changes nothing when one rule of many fails validation", func(t *testing.T) {
		set := RuleSet{Rules: []Rule{
			{From: "alpha", To: "beta"},
			{From: "beta", To: "gamma"},
		}}
		compiled, err := Compile(set, testTopology(), EnforceTail)
		if err == nil {
			t.Fatal("Compile returned no error, want an error")
		}
		if compiled.Forward != nil || compiled.Out != nil {
			t.Errorf("Compile returned %v rules, want none", compiled)
		}
	})
}

func TestObserveTail(t *testing.T) {
	// observeSet holds one rule, so that the chain carries an allow rule and a tail.
	observeSet := RuleSet{Mode: ModeObserve, Rules: []Rule{{From: "alpha", To: "beta"}}}

	t.Run("ends the forward chain with a log rule and a return rule", func(t *testing.T) {
		compiled, err := Compile(observeSet, testTopology(), ObserveTail)
		if err != nil {
			t.Fatalf("Compile returned %v, want no error", err)
		}
		got := lines(compiled.Forward)
		want := []string{
			"-A HYDRASCALE-FWD -m limit --limit 60/minute -j LOG --log-prefix hydrascale-would-deny: ",
			"-A HYDRASCALE-FWD -j RETURN",
		}
		if len(got) < 2 {
			t.Fatalf("the forward chain holds %d rules, want at least 2", len(got))
		}
		equalLines(t, compiled.Forward[len(compiled.Forward)-2:], want)
	})

	t.Run("ends the out chain with a log rule and a return rule for each namespace device", func(t *testing.T) {
		compiled, err := Compile(observeSet, testTopology(), ObserveTail)
		if err != nil {
			t.Fatalf("Compile returned %v, want no error", err)
		}
		want := []string{
			"-A HYDRASCALE-OUT -i vh0a0a0a0a0a0a -m limit --limit 60/minute -j LOG --log-prefix hydrascale-would-deny: ",
			"-A HYDRASCALE-OUT -i vh0a0a0a0a0a0a -j RETURN",
			"-A HYDRASCALE-OUT -i vh0b0b0b0b0b0b -m limit --limit 60/minute -j LOG --log-prefix hydrascale-would-deny: ",
			"-A HYDRASCALE-OUT -i vh0b0b0b0b0b0b -j RETURN",
		}
		if len(compiled.Out) < 4 {
			t.Fatalf("the out chain holds %d rules, want at least 4", len(compiled.Out))
		}
		equalLines(t, compiled.Out[len(compiled.Out)-4:], want)
	})

	t.Run("holds no drop rule in either chain", func(t *testing.T) {
		compiled, err := Compile(observeSet, testTopology(), ObserveTail)
		if err != nil {
			t.Fatalf("Compile returned %v, want no error", err)
		}
		for _, line := range append(lines(compiled.Forward), lines(compiled.Out)...) {
			if strings.Contains(line, "DROP") {
				t.Errorf("rule %q holds DROP, want none in the mode %s", line, ModeObserve)
			}
		}
	})

	t.Run("carries the prefix hydrascale-would-deny on the log rule", func(t *testing.T) {
		compiled, err := Compile(observeSet, testTopology(), ObserveTail)
		if err != nil {
			t.Fatalf("Compile returned %v, want no error", err)
		}
		rule := compiled.Forward[len(compiled.Forward)-2]
		prefix, ok := argValue(rule, "--log-prefix")
		if !ok {
			t.Fatalf("rule %q holds no --log-prefix option", strings.Join(rule, " "))
		}
		if prefix != "hydrascale-would-deny: " {
			t.Errorf("--log-prefix = %q, want %q", prefix, "hydrascale-would-deny: ")
		}
	})

	t.Run("carries the rate limit of 60 packets each minute on the log rule", func(t *testing.T) {
		compiled, err := Compile(observeSet, testTopology(), ObserveTail)
		if err != nil {
			t.Fatalf("Compile returned %v, want no error", err)
		}
		rule := strings.Join(compiled.Forward[len(compiled.Forward)-2], " ")
		if !strings.Contains(rule, "-m limit --limit 60/minute") {
			t.Errorf("rule = %q, want the match -m limit --limit 60/minute", rule)
		}
	})

	t.Run("ends the forward chain with a drop rule in the mode enforce", func(t *testing.T) {
		set := RuleSet{Mode: ModeEnforce, Rules: []Rule{{From: "alpha", To: "beta"}}}
		compiled, err := Compile(set, testTopology(), EnforceTail)
		if err != nil {
			t.Fatalf("Compile returned %v, want no error", err)
		}
		last := strings.Join(compiled.Forward[len(compiled.Forward)-1], " ")
		if last != "-A HYDRASCALE-FWD -j DROP" {
			t.Errorf("the last forward rule = %q, want %q", last, "-A HYDRASCALE-FWD -j DROP")
		}
	})
}

func TestTailForMode(t *testing.T) {
	t.Run("returns the enforce tail for an absent mode", func(t *testing.T) {
		tail, err := TailForMode("")
		if err != nil {
			t.Fatalf("TailForMode returned %v, want no error", err)
		}
		equalLines(t, tail, lines(EnforceTail))
	})

	t.Run("returns the observe tail for the mode observe", func(t *testing.T) {
		tail, err := TailForMode(ModeObserve)
		if err != nil {
			t.Fatalf("TailForMode returned %v, want no error", err)
		}
		equalLines(t, tail, lines(ObserveTail))
	})

	t.Run("rejects a mode that the daemon does not run and names both accepted values", func(t *testing.T) {
		_, err := TailForMode("audit")
		if err == nil {
			t.Fatal("TailForMode returned no error, want an error")
		}
		for _, want := range []string{"audit", ModeEnforce, ModeObserve} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error = %q, want the value %q in the message", err, want)
			}
		}
	})
}

// argValue returns the value that follows the named option in one argument list.
func argValue(rule []string, option string) (string, bool) {
	for i := 0; i+1 < len(rule); i++ {
		if rule[i] == option {
			return rule[i+1], true
		}
	}
	return "", false
}

func TestValidate(t *testing.T) {
	t.Run("reports every rule that fails", func(t *testing.T) {
		set := RuleSet{Rules: []Rule{
			{From: "alpha", To: "alpha"},
			{From: "alpha", To: "beta", Ports: []string{"tcp/0"}},
		}}
		err := set.Validate([]string{"alpha", "beta"})
		if err == nil {
			t.Fatal("Validate returned no error, want an error")
		}
		if !strings.Contains(err.Error(), "rule 1") || !strings.Contains(err.Error(), "rule 2") {
			t.Errorf("error = %q, want both rule 1 and rule 2 in the message", err)
		}
	})

	t.Run("names both accepted values when it rejects a mode", func(t *testing.T) {
		err := RuleSet{Mode: "audit"}.Validate(nil)
		if err == nil {
			t.Fatal("Validate returned no error, want an error")
		}
		for _, want := range []string{"audit", ModeEnforce, ModeObserve} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error = %q, want the value %q in the message", err, want)
			}
		}
	})

	t.Run("accepts an empty rule set", func(t *testing.T) {
		if err := (RuleSet{}).Validate(nil); err != nil {
			t.Errorf("Validate returned %v, want no error", err)
		}
	})
}
