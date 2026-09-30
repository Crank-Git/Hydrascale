package hostaccess

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
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

// Link names the registration of one tailnet. Device is the host side veth device of that
// tailnet, Address is the server that answers a query on it, and Domains holds every
// domain that the device routes.
// A link of systemd-resolved carries one list of servers for every domain that it holds,
// therefore each domain of Domains reaches Address. See RegisterDomains.
type Link struct {
	Device  string
	Address string
	Domains []string
}

// validDeviceName reports whether d is a device name that resolvectl reads as a link.
// A name that starts with a hyphen reads as an option, which is the finding SA-19.
var validDeviceName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,14}$`)

// RegisterDomains gives the domains of each tailnet to systemd-resolved, on the veth
// device of that tailnet.
// systemd-resolved refuses a per-link domain on the loopback device, and it answers
// "Link lo is loopback device". The registration therefore names the veth device of the
// tailnet.
// A link carries one list of servers for every domain that it holds, therefore every
// domain of one link reaches one server. The caller names that server in Link.Address.
// With resolver.resolve_aliases unset, the server is the namespace side address, and a
// DNAT rule inside the namespace sends the query to 100.100.100.100, which is the
// MagicDNS address of that tailnet. See SetupHostAccess in internal/namespaces/ns.go.
// With the key set, the server is the host side address, on which the DNS forwarder
// answers the alias zone and routes the MagicDNS suffix onward.
// RegisterDomains sets the server before the domains. A link that holds a domain and no
// server sends every query of that domain to a resolver that does not exist.
// RegisterDomains writes every domain of a link in one command, because `resolvectl
// domain` replaces the list of the link rather than add to it.
// RegisterDomains validates every link first. If one link fails the check, RegisterDomains
// rejects the whole set and runs no command.
func (rm *ResolvedManager) RegisterDomains(links []Link) error {
	if len(links) == 0 {
		return nil
	}

	for _, l := range links {
		if !validDeviceName.MatchString(l.Device) {
			return fmt.Errorf("the device %q is not a device name", l.Device)
		}
		if net.ParseIP(l.Address) == nil {
			return fmt.Errorf("the address %q of the device %q is not an IP address", l.Address, l.Device)
		}
		if len(l.Domains) == 0 {
			return fmt.Errorf("the device %q carries no domain", l.Device)
		}
		for _, d := range l.Domains {
			if !validDNSName(strings.TrimPrefix(d, "~")) {
				return fmt.Errorf("the domain %q of the device %q is not a DNS name", d, l.Device)
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), hostCommandTimeout)
	defer cancel()

	if !rm.isAvailable(ctx) {
		return fmt.Errorf("systemd-resolved is not running")
	}

	var errs []error
	var registered []string
	for _, l := range links {
		if out, err := rm.runner().Run(ctx, "resolvectl", "dns", l.Device, l.Address); err != nil {
			errs = append(errs, fmt.Errorf("resolvectl dns %s %s: %v (%s)", l.Device, l.Address, err, out))
			continue
		}
		args := []string{"domain", l.Device}
		for _, d := range l.Domains {
			args = append(args, "~"+strings.TrimPrefix(d, "~"))
		}
		if out, err := rm.runner().Run(ctx, "resolvectl", args...); err != nil {
			errs = append(errs, fmt.Errorf("resolvectl %s: %v (%s)", strings.Join(args, " "), err, out))
			continue
		}
		registered = append(registered, l.Device)
	}
	rm.registered = registered
	return errors.Join(errs...)
}

// DeregisterAll reverts every device that RegisterDomains registered.
// DeregisterAll reverts the remaining devices when one revert fails, and it returns the
// failures together. A device that keeps its domain sends a query to a resolver that no
// longer answers it.
func (rm *ResolvedManager) DeregisterAll() error {
	if len(rm.registered) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), hostCommandTimeout)
	defer cancel()

	var errs []error
	for _, device := range rm.registered {
		if out, err := rm.runner().Run(ctx, "resolvectl", "revert", device); err != nil {
			errs = append(errs, fmt.Errorf("resolvectl revert %s: %w (%s)", device, err, out))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	rm.registered = nil
	return nil
}
