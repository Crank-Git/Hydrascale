package main

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"hydrascale/internal/config"

	"github.com/spf13/cobra"
)

func initCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Interactive first-run setup wizard",
		Long: `init walks you through preflight checks, generates a config, sets up
authentication (auth key or browser login), brings your first tailnet up, and
prints how to use it. Run as root.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			force, _ := cmd.Flags().GetBool("force")
			return runInit(force)
		},
	}
	cmd.Flags().Bool("force", false, "Overwrite an existing config (backs up the old one to .bak)")
	return cmd
}

func runInit(force bool) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("init must run as root (it creates namespaces and system directories); try: sudo hydrascale init")
	}
	p := newPrompter(os.Stdin, os.Stdout)

	fmt.Println("Hydrascale setup")
	fmt.Println("================")

	// Existing-config guard.
	path := configPath()
	var existing *config.Config
	if _, err := os.Stat(path); err == nil && !force {
		fmt.Printf("\nA config already exists at %s\n", path)
		switch strings.ToLower(p.line("[a]dd a tailnet, [r]ecreate (backs up the old one), or [q]uit?", "a")) {
		case "q":
			return nil
		case "r":
			if err := backupFile(path); err != nil {
				return fmt.Errorf("back up existing config: %w", err)
			}
			fmt.Printf("Backed up to %s.bak\n", path)
		default:
			cfg, err := config.LoadConfig(path)
			if err != nil {
				return fmt.Errorf("load existing config: %w", err)
			}
			existing = cfg
		}
	}

	runPreflight(p)

	// Prompt one or more tailnets. Keys are collected separately and never
	// written to the config; they are exported as HYDRASCALE_AUTHKEY_<ID>.
	var answers []tailnetAnswers
	keys := map[string]string{}
	for {
		a, key := promptTailnet(p)
		answers = append(answers, a)
		if a.UseKey && key != "" {
			keys[a.ID] = key
		}
		if !p.yesNo("Add another tailnet?", false) {
			break
		}
	}

	// Build (or merge into existing) config. Global host_access stays false;
	// tailnets that want it get an explicit per-tailnet override.
	var cfg *config.Config
	if existing != nil {
		cfg = existing
		cfg.Tailnets = append(cfg.Tailnets, buildConfig(answers, existing.HostAccess).Tailnets...)
	} else {
		cfg = buildConfig(answers, false)
	}

	for _, d := range []string{"/var/lib/hydrascale/state", "/var/log/hydrascale"} {
		if err := os.MkdirAll(d, 0750); err != nil {
			return fmt.Errorf("create %s: %w", d, err)
		}
	}
	if err := config.SaveConfig(path, cfg); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	fmt.Printf("\nWrote config to %s\n", path)

	// Export keys for the in-process first run and show how to persist them.
	for id, key := range keys {
		os.Setenv(config.AuthKeyEnvVar(id), key)
		fmt.Printf("  Persist for restarts: export %s=<your-key>\n", config.AuthKeyEnvVar(id))
	}

	printCheatSheet()
	fmt.Println("\nNext: run 'sudo hydrascale apply' to bring your tailnets up.")
	return nil
}

// promptTailnet collects answers for one tailnet, returning the answers and the
// raw auth key (empty for browser auth).
func promptTailnet(p *prompter) (tailnetAnswers, string) {
	var a tailnetAnswers
	for {
		a.ID = p.line("Tailnet id (e.g. personal, work)", "")
		if config.IsValidID(a.ID) {
			break
		}
		fmt.Println("  invalid id — letters/digits/dots/hyphens/underscores, start alphanumeric, max 63 chars")
	}
	a.HostAccess = p.yesNo("Enable host access (reach this tailnet's peers directly from the host)?", false)
	a.ExitNode = p.line("Exit node hostname (optional, blank for none)", "")

	if strings.HasPrefix(strings.ToLower(p.line("Authenticate with an auth [k]ey or [b]rowser login?", "k")), "k") {
		a.UseKey = true
		key, err := p.secret("Paste auth key (tskey-auth-...)")
		if err != nil {
			fmt.Printf("  could not read key (%v); falling back to browser login\n", err)
			a.UseKey = false
			return a, ""
		}
		return a, key
	}
	return a, ""
}

// runPreflight reports environment readiness and offers to fix common problems.
func runPreflight(p *prompter) {
	fmt.Println("\nPreflight:")
	for _, bin := range []string{"tailscale", "tailscaled", "iptables", "ip"} {
		if _, err := exec.LookPath(bin); err != nil {
			fmt.Printf("  ✗ %s not found in PATH\n", bin)
		} else {
			fmt.Printf("  ✓ %s found\n", bin)
		}
	}

	if v, _ := os.ReadFile("/proc/sys/net/ipv4/ip_forward"); strings.TrimSpace(string(v)) != "1" {
		fmt.Println("  ✗ net.ipv4.ip_forward is disabled")
		if p.yesNo("    Enable IP forwarding now (and persist)?", true) {
			_ = exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1").Run()
			_ = os.WriteFile("/etc/sysctl.d/99-hydrascale.conf", []byte("net.ipv4.ip_forward=1\n"), 0644)
			fmt.Println("    enabled")
		}
	} else {
		fmt.Println("  ✓ IP forwarding enabled")
	}

	if hasPolicyRouting() {
		fmt.Println("  ✓ kernel policy routing (multiple tables) available")
	} else {
		fmt.Println("  ! CONFIG_IP_MULTIPLE_TABLES not detected — host route propagation for accepted subnet routes will not work on this kernel")
	}

	if nativeTailscaleAcceptDNS() {
		fmt.Println("  ! the host's own tailscaled has accept-dns enabled — it hijacks /etc/resolv.conf and fights hydrascale's DNS")
		if p.yesNo("    Disable it now (tailscale set --accept-dns=false)?", true) {
			_ = exec.Command("tailscale", "set", "--accept-dns=false").Run()
			fmt.Println("    disabled")
		}
	}
}

// hasPolicyRouting reports whether the kernel was built with multiple routing
// tables (needed for host-route propagation). Returns true when undetectable so
// we don't raise a false alarm.
func hasPolicyRouting() bool {
	if data, ok := readMaybeGzip("/proc/config.gz"); ok {
		return strings.Contains(data, "CONFIG_IP_MULTIPLE_TABLES=y")
	}
	rel, err := exec.Command("uname", "-r").Output()
	if err != nil {
		return true
	}
	data, err := os.ReadFile("/boot/config-" + strings.TrimSpace(string(rel)))
	if err != nil {
		return true
	}
	return strings.Contains(string(data), "CONFIG_IP_MULTIPLE_TABLES=y")
}

// readMaybeGzip reads a gzipped file, returning its decompressed contents.
func readMaybeGzip(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", false
	}
	defer gz.Close()
	data, err := io.ReadAll(gz)
	if err != nil {
		return "", false
	}
	return string(data), true
}

// nativeTailscaleAcceptDNS reports whether a host-level tailscaled has accept-dns on.
func nativeTailscaleAcceptDNS() bool {
	out, err := exec.Command("tailscale", "debug", "prefs").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "\"CorpDNS\": true")
}

func backupFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path+".bak", data, 0640)
}

// printCheatSheet shows how to run commands inside tailnet namespaces.
func printCheatSheet() {
	fmt.Println("\nHow to use your tailnets:")
	fmt.Println("  Run a command:            sudo hydrascale exec <id> -- <cmd>")
	fmt.Println("  Quick tools:              hydrascale ssh|ping|tailscale <id> ...")
	fmt.Println("  Default THIS shell:       eval \"$(hydrascale env <id>)\"")
	fmt.Println("  Set a persistent default: hydrascale switch <id>")
	fmt.Println("  Run a service in-ns:      hydrascale wrap <service> <id>")
}
