package main

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"hydrascale/internal/config"
	"hydrascale/internal/daemon"
	"hydrascale/internal/namespaces"

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
	cmd.Flags().Bool("force", false, "Overwrite an existing config (backs up the old one to .bak), and turn the accept-dns preflight failure into a warning")
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

	if err := runPreflight(p, nativeTailscaleAcceptDNS(), force); err != nil {
		return err
	}

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

	promptGUIAccess(p, cfg)

	for _, d := range []string{"/etc/hydrascale", "/var/lib/hydrascale/state", "/var/log/hydrascale"} {
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

	return firstRunAndVerify(p, cfg, answers)
}

// firstRunAndVerify reconciles the new config to bring tailnets up, verifies
// each one authenticated (retrying auth-key logins that raced), prints the
// namespace cheat-sheet, and offers to install the boot service.
func firstRunAndVerify(p *prompter, cfg *config.Config, answers []tailnetAnswers) error {
	fmt.Println("\nBringing tailnets up...")
	r := newReconciler()
	r.ResetAllErrors()
	if err := r.Reconcile(); err != nil {
		fmt.Printf("  reconcile reported: %v\n", err)
	}

	fmt.Println("\nAuthentication:")
	for _, a := range answers {
		verifyAuth(a)
	}

	printCheatSheet()

	if p.yesNo("\nInstall as a systemd service so tailnets start on boot?", false) {
		self, err := os.Executable()
		if err != nil {
			fmt.Printf("  cannot locate own binary: %v\n", err)
			return nil
		}
		c := exec.Command(self, "install")
		c.Stdout, c.Stderr = os.Stdout, os.Stderr
		if err := c.Run(); err != nil {
			fmt.Printf("  install failed: %v\n", err)
		}
	}
	return nil
}

// stageAuthKey writes key to a new file under dir, with mode 0600, and returns the path
// of that file. The caller removes the file after the command ends.
// An argument of a command reaches /proc/<pid>/cmdline, and any local account reads that
// file. The daemon path already applies this form; see SA-6 and issue #31.
func stageAuthKey(dir, key string) (string, error) {
	f, err := os.CreateTemp(dir, "authkey-*")
	if err != nil {
		return "", fmt.Errorf("create the auth key file: %w", err)
	}
	path := f.Name()
	if err := f.Chmod(0600); err != nil {
		f.Close()
		os.Remove(path)
		return "", fmt.Errorf("set the mode of the auth key file: %w", err)
	}
	if _, err := f.WriteString(key); err != nil {
		f.Close()
		os.Remove(path)
		return "", fmt.Errorf("write the auth key file: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return "", fmt.Errorf("close the auth key file: %w", err)
	}
	return path, nil
}

// nsTailscaleUpArgs returns the arguments of the ip command that authenticates a tailnet
// inside its namespace. authKeyFile names the file that holds the auth key.
func nsTailscaleUpArgs(nsName, socketPath, authKeyFile string) []string {
	return []string{
		"netns", "exec", nsName,
		"tailscale", "--socket=" + socketPath,
		"up", "--accept-dns=true",
		"--auth-key=file:" + authKeyFile,
	}
}

// verifyAuth checks that a tailnet's namespace is authenticated. For auth-key
// tailnets it retries `up --authkey` once if the initial login raced; for
// browser tailnets it prints the login URL and polls until logged in.
func verifyAuth(a tailnetAnswers) {
	status, _ := nsTailscaleStatus(a.ID)
	if isLoggedIn(status) {
		fmt.Printf("  ✓ %s: authenticated\n", a.ID)
		return
	}

	if a.UseKey {
		if key := os.Getenv(config.AuthKeyEnvVar(a.ID)); key != "" {
			ns := namespaces.GetNamespaceName(a.ID)
			sock := daemon.SocketPath(a.ID)
			keyFile, err := stageAuthKey(filepath.Dir(sock), key)
			if err != nil {
				fmt.Printf("  ✗ %s: cannot stage the auth key: %v\n", a.ID, err)
				return
			}
			_ = exec.Command("ip", nsTailscaleUpArgs(ns, sock, keyFile)...).Run()
			if err := os.Remove(keyFile); err != nil {
				fmt.Printf("  ! %s: cannot remove the auth key file %s: %v\n", a.ID, keyFile, err)
			}
			if status, _ = nsTailscaleStatus(a.ID); isLoggedIn(status) {
				fmt.Printf("  ✓ %s: authenticated (after retry)\n", a.ID)
				return
			}
		}
		fmt.Printf("  ✗ %s: not authenticated — verify the auth key and re-run\n", a.ID)
		return
	}

	url := loginURL(status)
	if url == "" {
		fmt.Printf("  ✗ %s: no login URL available yet — check 'hydrascale tailscale %s -- status'\n", a.ID, a.ID)
		return
	}
	fmt.Printf("  → %s: open this URL to log in:\n      %s\n    waiting up to 2m...\n", a.ID, url)
	for i := 0; i < 40; i++ {
		time.Sleep(3 * time.Second)
		if status, _ = nsTailscaleStatus(a.ID); isLoggedIn(status) {
			fmt.Printf("  ✓ %s: authenticated\n", a.ID)
			return
		}
	}
	fmt.Printf("  ✗ %s: timed out waiting for browser login\n", a.ID)
}

// nsTailscaleStatus returns the output of `tailscale status` inside a tailnet's namespace.
func nsTailscaleStatus(id string) (string, error) {
	ns := namespaces.GetNamespaceName(id)
	sock := daemon.SocketPath(id)
	out, err := exec.Command("ip", "netns", "exec", ns, "tailscale", "--socket="+sock, "status").CombinedOutput()
	return string(out), err
}

// isLoggedIn reports whether a `tailscale status` output indicates an authenticated node.
func isLoggedIn(status string) bool {
	if strings.Contains(status, "Logged out") || strings.Contains(status, "Log in at") || strings.Contains(status, "NeedsLogin") {
		return false
	}
	return strings.TrimSpace(status) != ""
}

// loginURL extracts the interactive login URL from a `tailscale status` output.
func loginURL(status string) string {
	for _, f := range strings.Fields(status) {
		if strings.HasPrefix(f, "https://login.tailscale.com/") {
			return f
		}
	}
	return ""
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

// promptGUIAccess optionally grants non-root access to the control socket via a
// unix group. The Hydrascale desktop app (and any SSH-forwarded remote client)
// connects as a non-root user, so this is required for the GUI to work.
func promptGUIAccess(p *prompter, cfg *config.Config) {
	fmt.Fprintln(p.out, "\nGUI / remote access:")
	fmt.Fprintln(p.out, "  The daemon's control socket is root-only by default. The Hydrascale")
	fmt.Fprintln(p.out, "  desktop app — and any non-root or SSH-forwarded client — needs group")
	fmt.Fprintln(p.out, "  access to it. This is required for the GUI.")
	fmt.Fprintln(p.out, "  Warning: a member of this group can send a command to the daemon.")
	fmt.Fprintln(p.out, "  The daemon runs as root, so the member can create a namespace, write a")
	fmt.Fprintln(p.out, "  host route, and run a command as root.")
	fmt.Fprintln(p.out, "  Membership of this group is equivalent to root access on this host.")
	fmt.Fprintln(p.out, "  Name a group that holds only trusted operators.")
	if !p.yesNo("  Enable non-root access via a unix group?", true) {
		return
	}
	group := p.line("  Group name", "hydrascale")
	user := p.line("  User to add to the group", os.Getenv("SUDO_USER"))
	if out, err := exec.Command("groupadd", "-f", group).CombinedOutput(); err != nil {
		fmt.Printf("  groupadd failed: %v (%s)\n", err, strings.TrimSpace(string(out)))
		return
	}
	if user != "" {
		if out, err := exec.Command("usermod", "-aG", group, user).CombinedOutput(); err != nil {
			fmt.Printf("  usermod failed: %v (%s)\n", err, strings.TrimSpace(string(out)))
		} else {
			fmt.Printf("  ✓ added %s to group %q (log out/in for it to take effect)\n", user, group)
		}
	}
	cfg.SocketGroup = group
	fmt.Printf("  ✓ socket_group set to %q — the daemon will make the socket group-accessible\n", group)
}

// runPreflight reports environment readiness and offers to fix common problems.
// acceptDNS states whether the host tailscaled has accept-dns enabled. force turns the
// accept-dns failure into a warning. runPreflight returns an error when a check fails,
// and the caller stops the setup.
func runPreflight(p *prompter, acceptDNS, force bool) error {
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

	warning, err := checkHostAcceptDNS(acceptDNS, force)
	if warning != "" {
		fmt.Printf("  ! %s\n", warning)
	}
	if err != nil {
		fmt.Printf("  ✗ %v\n", err)
		return err
	}
	fmt.Println("  ✓ the host tailscaled does not take over the host DNS")
	return nil
}

// acceptDNSFix is the command that turns the host accept-dns preference off.
const acceptDNSFix = "sudo tailscale set --accept-dns=false"

// checkHostAcceptDNS returns the preflight result for the host accept-dns preference.
// acceptDNS states whether the host tailscaled has accept-dns enabled. force turns the
// failure into a warning. checkHostAcceptDNS returns an error when acceptDNS is true and
// force is false, and it returns a warning string when acceptDNS is true and force is
// true.
func checkHostAcceptDNS(acceptDNS, force bool) (string, error) {
	if !acceptDNS {
		return "", nil
	}
	const cause = "the host tailscaled has accept-dns enabled. It takes over /etc/resolv.conf and it defeats the hydrascale DNS forwarder."
	if force {
		return fmt.Sprintf("%s --force continues. To fix it, run: %s", cause, acceptDNSFix), nil
	}
	return "", fmt.Errorf("%s To fix it, run: %s, then run hydrascale init again", cause, acceptDNSFix)
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
