package hostaccess

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
)

type ResolvedManager struct {
	registered []string
}

func NewResolvedManager() *ResolvedManager {
	return &ResolvedManager{}
}

func (rm *ResolvedManager) isAvailable() bool {
	return exec.Command("systemctl", "is-active", "--quiet", "systemd-resolved").Run() == nil
}

func (rm *ResolvedManager) RegisterDomains(domains []string) error {
	if len(domains) == 0 {
		return nil
	}
	if !rm.isAvailable() {
		return fmt.Errorf("systemd-resolved is not running")
	}
	args := []string{"domain", "lo"}
	for _, d := range domains {
		if !strings.HasPrefix(d, "~") {
			d = "~" + d
		}
		args = append(args, d)
	}
	cmd := exec.Command("resolvectl", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("resolvectl domain failed: %v (%s)", err, out)
	}
	cmd = exec.Command("resolvectl", "dns", "lo", "127.0.0.53:5354")
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("resolvectl dns failed: %v (%s)", err, out)
	}
	rm.registered = domains
	return nil
}

func (rm *ResolvedManager) DeregisterAll() {
	if len(rm.registered) == 0 {
		return
	}
	exec.Command("resolvectl", "revert", "lo").Run()
	rm.registered = nil
}
