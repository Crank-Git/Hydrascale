package access

// PreservingRuleSet returns the rule set that preserves the reachability of version 0.9.
// tailnetIDs holds every tailnet identifier that the configuration declares, in the order
// of the configuration file.
// The rule set holds one rule per tailnet, from that tailnet to Internet, with an empty
// port list. It holds no rule between two tailnets, so no tailnet reaches another tailnet.
// PreservingRuleSet is pure: the same identifiers always produce the same rule set.
// PreservingRuleSet names no mode, so the daemon applies ModeEnforce.
func PreservingRuleSet(tailnetIDs []string) RuleSet {
	set := RuleSet{}
	for _, id := range tailnetIDs {
		set.Rules = append(set.Rules, Rule{From: id, To: Internet})
	}
	return set
}
