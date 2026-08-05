package access

import (
	"strings"
	"testing"
)

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
			"-A HYDRASCALE-FWD -i vh0a0a0a0a0a0a ! -o vh+ -j ACCEPT",
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

	t.Run("accepts an empty rule set", func(t *testing.T) {
		if err := (RuleSet{}).Validate(nil); err != nil {
			t.Errorf("Validate returned %v, want no error", err)
		}
	})
}
