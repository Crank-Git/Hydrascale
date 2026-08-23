// Package access holds the local rule model and the compiler that turns a rule set into
// iptables arguments. The package writes no rule to the host and it runs no command.
package access

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Host is the literal that names the Linux machine that runs the daemon.
const Host = "host"

// Internet is the literal that names every address outside the host and the namespaces.
const Internet = "internet"

// ModeEnforce drops a packet that no rule allows.
const ModeEnforce = "enforce"

// ModeObserve logs a packet that no rule allows and drops nothing.
const ModeObserve = "observe"

// Rule allows traffic from one endpoint to another endpoint on a list of ports.
// An empty port list allows every port and every protocol. The model holds no deny
// rule, because deny is the default.
type Rule struct {
	From  string   `yaml:"from" json:"from"`
	To    string   `yaml:"to" json:"to"`
	Ports []string `yaml:"ports,omitempty" json:"ports"`
}

// RuleSet holds the local rules of one host and the mode that the daemon applies them
// in. An absent mode means ModeEnforce.
type RuleSet struct {
	Mode  string `yaml:"mode,omitempty" json:"mode"`
	Rules []Rule `yaml:"rules,omitempty" json:"rules"`
}

// EffectiveMode returns the mode that the daemon applies, which is ModeEnforce when the
// rule set names no mode.
func (s RuleSet) EffectiveMode() string {
	if s.Mode == "" {
		return ModeEnforce
	}
	return s.Mode
}

// portPattern matches one port entry: a protocol, one port, and an optional second port.
var portPattern = regexp.MustCompile(`^(tcp|udp)/([0-9]{1,5})(?:-([0-9]{1,5}))?$`)

// port holds one parsed port entry.
type port struct {
	protocol string
	low      int
	high     int
}

// parsePort returns the parsed port entry.
// entry is one item of the port list of a rule, in the form tcp/22 or udp/1-1024.
// parsePort returns an error when the protocol is not tcp or udp, when a number is
// outside 1 to 65535, or when the second number is lower than the first.
func parsePort(entry string) (port, error) {
	match := portPattern.FindStringSubmatch(entry)
	if match == nil {
		return port{}, fmt.Errorf("invalid port %q: the form is tcp/<n>, udp/<n>, tcp/<n>-<m>, or udp/<n>-<m>, for example tcp/22", entry)
	}
	low, err := strconv.Atoi(match[2])
	if err != nil || low < 1 || low > 65535 {
		return port{}, fmt.Errorf("invalid port %q: a port number is between 1 and 65535", entry)
	}
	high := low
	if match[3] != "" {
		high, err = strconv.Atoi(match[3])
		if err != nil || high < 1 || high > 65535 {
			return port{}, fmt.Errorf("invalid port %q: a port number is between 1 and 65535", entry)
		}
		if high < low {
			return port{}, fmt.Errorf("invalid port %q: the second number is lower than the first", entry)
		}
	}
	return port{protocol: match[1], low: low, high: high}, nil
}

// match returns the iptables arguments that select this port entry.
func (p port) match() []string {
	dport := strconv.Itoa(p.low)
	if p.high != p.low {
		dport = dport + ":" + strconv.Itoa(p.high)
	}
	return []string{"-p", p.protocol, "--dport", dport}
}

// Validate returns an error when the rule set names an unknown tailnet, an unknown mode,
// or an invalid port.
// tailnets holds every tailnet identifier that the configuration declares.
// Validate checks every rule and it reports every failure together, so that a caller
// never applies a rule set in part.
func (s RuleSet) Validate(tailnets []string) error {
	var failures []error

	switch s.Mode {
	case "", ModeEnforce, ModeObserve:
	default:
		failures = append(failures, fmt.Errorf("invalid mode %q: the daemon runs the modes %s and %s", s.Mode, ModeEnforce, ModeObserve))
	}

	declared := make(map[string]bool, len(tailnets))
	for _, id := range tailnets {
		declared[id] = true
	}

	for i, rule := range s.Rules {
		if err := rule.validate(declared); err != nil {
			failures = append(failures, fmt.Errorf("rule %d: %w", i+1, err))
		}
	}
	if len(failures) == 0 {
		return nil
	}
	return errors.Join(failures...)
}

// validate returns an error when the rule names an unknown endpoint or an invalid port.
// declared holds every tailnet identifier that the configuration declares.
func (r Rule) validate(declared map[string]bool) error {
	var failures []error

	switch {
	case r.From == "":
		failures = append(failures, errors.New("from is empty"))
	case r.From == Internet:
		failures = append(failures, fmt.Errorf("from is %q: a source is a tailnet or the literal %s", Internet, Host))
	case r.From != Host && !declared[r.From]:
		failures = append(failures, fmt.Errorf("from names the tailnet %q, which the configuration does not declare", r.From))
	}

	switch {
	case r.To == "":
		failures = append(failures, errors.New("to is empty"))
	case r.To != Host && r.To != Internet && !declared[r.To]:
		failures = append(failures, fmt.Errorf("to names the tailnet %q, which the configuration does not declare", r.To))
	}

	if r.From != "" && r.From == r.To {
		failures = append(failures, fmt.Errorf("from equals to (%q)", r.From))
	}

	for _, entry := range r.Ports {
		if _, err := parsePort(entry); err != nil {
			failures = append(failures, err)
		}
	}

	if len(failures) == 0 {
		return nil
	}
	return errors.Join(failures...)
}

// String returns the rule in the form that an error message and a log line use.
func (r Rule) String() string {
	if len(r.Ports) == 0 {
		return r.From + " -> " + r.To
	}
	return r.From + " -> " + r.To + " " + strings.Join(r.Ports, ",")
}
