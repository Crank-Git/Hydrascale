package hostaccess

import (
	"context"
	"fmt"
	"log"
	"strings"

	"hydrascale/internal/execx"
)

type ResolvedManager struct {
	// Runner runs every command that the ResolvedManager sends to the host. A test
	// replaces Runner with an execx.Recorder and asserts the exact argument list.
	Runner execx.Runner

	registered []string
}

func NewResolvedManager() *ResolvedManager {
	return &ResolvedManager{Runner: execx.OSRunner{}}
}

// runner returns the command runner. A ResolvedManager with no Runner runs on the host.
func (rm *ResolvedManager) runner() execx.Runner {
	if rm.Runner == nil {
		return execx.OSRunner{}
	}
	return rm.Runner
}

// validDNSName reports whether d is a DNS name. A trailing dot is permitted.
//
// The control server supplies the MagicDNS suffix through `tailscale status --json`. The
// suffix becomes one argument of `resolvectl domain lo`, and resolvectl reads an argument
// that starts with a hyphen as an option. See SA-19.
func validDNSName(d string) bool {
	d = strings.TrimSuffix(d, ".")
	if d == "" || len(d) > 253 {
		return false
	}
	for _, label := range strings.Split(d, ".") {
		if label == "" || len(label) > 63 {
			return false
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			switch {
			case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			case c == '-' && i != 0 && i != len(label)-1:
			default:
				return false
			}
		}
	}
	return true
}

func (rm *ResolvedManager) isAvailable(ctx context.Context) bool {
	_, err := rm.runner().Run(ctx, "systemctl", "is-active", "--quiet", "systemd-resolved")
	return err == nil
}

// RegisterDomains gives every domain to systemd-resolved on the loopback interface.
// RegisterDomains validates every domain as a DNS name first. If one domain is not a DNS
// name, RegisterDomains rejects the whole set and runs no command.
func (rm *ResolvedManager) RegisterDomains(domains []string) error {
	if len(domains) == 0 {
		return nil
	}

	args := []string{"domain", "lo"}
	for _, d := range domains {
		name := strings.TrimPrefix(d, "~")
		if !validDNSName(name) {
			return fmt.Errorf("the MagicDNS suffix %q is not a DNS name", d)
		}
		args = append(args, "~"+name)
	}

	ctx, cancel := context.WithTimeout(context.Background(), hostCommandTimeout)
	defer cancel()

	if !rm.isAvailable(ctx) {
		return fmt.Errorf("systemd-resolved is not running")
	}

	if out, err := rm.runner().Run(ctx, "resolvectl", args...); err != nil {
		return fmt.Errorf("resolvectl domain failed: %v (%s)", err, out)
	}
	if out, err := rm.runner().Run(ctx, "resolvectl", "dns", "lo", "127.0.0.53:5354"); err != nil {
		log.Printf("resolvectl dns failed: %v (%s)", err, out)
	}
	rm.registered = domains
	return nil
}

// DeregisterAll removes every domain that RegisterDomains gave to systemd-resolved.
// DeregisterAll returns the failure of the revert command, because a domain that stays
// registered sends a query to a resolver that no longer answers it.
func (rm *ResolvedManager) DeregisterAll() error {
	if len(rm.registered) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), hostCommandTimeout)
	defer cancel()

	if out, err := rm.runner().Run(ctx, "resolvectl", "revert", "lo"); err != nil {
		return fmt.Errorf("resolvectl revert lo: %w (%s)", err, out)
	}
	rm.registered = nil
	return nil
}
