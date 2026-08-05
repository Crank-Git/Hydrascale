package access

import "testing"

func TestPreservingRuleSet(t *testing.T) {
	t.Run("holds one rule per tailnet to internet", func(t *testing.T) {
		set := PreservingRuleSet([]string{"alpha", "beta", "gamma"})
		if len(set.Rules) != 3 {
			t.Fatalf("len(Rules) = %d, want 3", len(set.Rules))
		}
		for i, want := range []string{"alpha", "beta", "gamma"} {
			if set.Rules[i].From != want {
				t.Errorf("Rules[%d].From = %q, want %q", i, set.Rules[i].From, want)
			}
			if set.Rules[i].To != Internet {
				t.Errorf("Rules[%d].To = %q, want %q", i, set.Rules[i].To, Internet)
			}
		}
	})

	t.Run("gives every rule an empty port list", func(t *testing.T) {
		set := PreservingRuleSet([]string{"alpha", "beta"})
		for i, rule := range set.Rules {
			if len(rule.Ports) != 0 {
				t.Errorf("Rules[%d].Ports = %v, want an empty list", i, rule.Ports)
			}
		}
	})

	t.Run("names no rule between two tailnets and no rule to the host", func(t *testing.T) {
		set := PreservingRuleSet([]string{"alpha", "beta"})
		for i, rule := range set.Rules {
			if rule.To != Internet {
				t.Errorf("Rules[%d] = %q, want a rule to %s only", i, rule, Internet)
			}
		}
	})

	t.Run("passes validation against the tailnets that it names", func(t *testing.T) {
		ids := []string{"alpha", "beta"}
		if err := PreservingRuleSet(ids).Validate(ids); err != nil {
			t.Fatalf("Validate: %v", err)
		}
	})

	t.Run("holds no rule when the configuration declares no tailnet", func(t *testing.T) {
		set := PreservingRuleSet(nil)
		if len(set.Rules) != 0 {
			t.Errorf("len(Rules) = %d, want 0", len(set.Rules))
		}
	})
}
