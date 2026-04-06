# Host Access Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enable transparent access to tailnet peers from the host machine — `ping havoc-mars` just works.

**Architecture:** New `internal/hostaccess/` package called by the reconciler each cycle. Parses peer data from existing `tailscale status --json` calls, manages host routes and `/etc/hosts` entries. Namespace-side iptables consolidated in `internal/namespaces/ns.go`.

**Tech Stack:** Go 1.24, os/exec for ip route/iptables, gopkg.in/yaml.v3 for config, godbus/dbus for resolved mode.

**Spec:** `docs/specs/2026-04-05-host-access-design.md`

**Branch:** `feat/host-access` off `main`

---

## File Map

| File | Action | Responsibility |
|------|--------|---------------|
| `internal/config/config.go` | Modify | Add `HostAccess`, `HostDNS` fields |
| `internal/config/config_test.go` | Modify | Test new config fields |
| `internal/daemon/daemon.go` | Modify | Add `GetStatus()` to Manager interface |
| `internal/hostaccess/peers.go` | Create | Parse tailscale status JSON → peer map |
| `internal/hostaccess/peers_test.go` | Create | 5 test cases |
| `internal/hostaccess/routes.go` | Create | Host route add/replace/delete |
| `internal/hostaccess/routes_test.go` | Create | 4 test cases |
| `internal/hostaccess/hosts.go` | Create | /etc/hosts managed block |
| `internal/hostaccess/hosts_test.go` | Create | 6 test cases |
| `internal/hostaccess/resolved.go` | Create | systemd-resolved D-Bus |
| `internal/hostaccess/resolved_test.go` | Create | 3 test cases |
| `internal/hostaccess/hostaccess.go` | Create | Manager, Sync(), Teardown() |
| `internal/hostaccess/hostaccess_test.go` | Create | 4 integration tests |
| `internal/namespaces/ns.go` | Modify | Host access iptables in SetupVeth |
| `internal/reconciler/reconciler.go` | Modify | Wire hostaccess into cycle |
| `internal/reconciler/reconciler_test.go` | Modify | Add GetStatus mock, test host access actions |
| `internal/api/server_test.go` | Modify | Add GetStatus to mock |

---

## Task 1: Config Schema

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write failing test for HostAccess config parsing**

Add to `internal/config/config_test.go`:

```go
func TestLoadConfigHostAccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
version: 2
host_access: true
tailnets:
  - id: havoc
    host_access: true
  - id: personal
host_dns:
  mode: hosts
reconciler:
  interval: 10s
`
	os.WriteFile(path, []byte(content), 0644)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.HostAccess {
		t.Error("expected global HostAccess to be true")
	}
	if cfg.HostDNS.Mode != "hosts" {
		t.Errorf("expected HostDNS.Mode=hosts, got %q", cfg.HostDNS.Mode)
	}
	if cfg.Tailnets[0].HostAccess == nil || !*cfg.Tailnets[0].HostAccess {
		t.Error("expected havoc HostAccess to be true")
	}
	if cfg.Tailnets[1].HostAccess != nil {
		t.Error("expected personal HostAccess to be nil (inherit global)")
	}
}

func TestLoadConfigHostAccessDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
version: 2
tailnets:
  - id: test
reconciler:
  interval: 10s
`
	os.WriteFile(path, []byte(content), 0644)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HostAccess {
		t.Error("expected global HostAccess to default to false")
	}
	if cfg.HostDNS.Mode != "" {
		t.Errorf("expected empty HostDNS.Mode when not set, got %q", cfg.HostDNS.Mode)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/e/development/Hydrascale && go test ./internal/config/ -run TestLoadConfigHostAccess -v`
Expected: FAIL — `HostAccess` field does not exist on Config/Tailnet structs.

- [ ] **Step 3: Add HostAccess and HostDNS fields to config structs**

In `internal/config/config.go`, modify the `Tailnet` struct:

```go
type Tailnet struct {
	ID         string `yaml:"id"`
	ExitNode   string `yaml:"exit_node,omitempty"`
	AuthKey    string `yaml:"auth_key,omitempty"`
	HostAccess *bool  `yaml:"host_access,omitempty"` // nil = inherit global
}
```

Add `HostDNSConfig` struct:

```go
// HostDNSConfig represents host DNS integration settings.
type HostDNSConfig struct {
	Mode string `yaml:"mode,omitempty"` // "hosts" (default) or "resolved"
}
```

Add fields to the `Config` struct:

```go
type Config struct {
	Version    int              `yaml:"version,omitempty"`
	HostAccess bool             `yaml:"host_access,omitempty"`
	Tailnets   []Tailnet        `yaml:"tailnets"`
	Resolver   ResolverConfig   `yaml:"resolver"`
	Reconciler ReconcilerConfig `yaml:"reconciler,omitempty"`
	Mesh       Mesh             `yaml:"mesh,omitempty"`
	EventLog   string           `yaml:"event_log,omitempty"`
	HostDNS    HostDNSConfig    `yaml:"host_dns,omitempty"`
}
```

Add a helper method:

```go
// TailnetHostAccess returns whether host access is enabled for a specific tailnet,
// resolving per-tailnet override against the global default.
func (c *Config) TailnetHostAccess(tailnetID string) bool {
	for _, tn := range c.Tailnets {
		if tn.ID == tailnetID {
			if tn.HostAccess != nil {
				return *tn.HostAccess
			}
			return c.HostAccess
		}
	}
	return c.HostAccess
}

// EffectiveHostDNSMode returns the DNS mode to use. Defaults to "hosts" if host_access
// is enabled for any tailnet and no mode is specified.
func (c *Config) EffectiveHostDNSMode() string {
	if c.HostDNS.Mode != "" {
		return c.HostDNS.Mode
	}
	// Default to "hosts" if any tailnet has host_access enabled
	for _, tn := range c.Tailnets {
		if c.TailnetHostAccess(tn.ID) {
			return "hosts"
		}
	}
	return ""
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/e/development/Hydrascale && go test ./internal/config/ -run TestLoadConfigHostAccess -v`
Expected: PASS

- [ ] **Step 5: Run all existing tests to check for regressions**

Run: `cd /home/e/development/Hydrascale && go test ./...`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add host_access and host_dns config schema"
```

---

## Task 2: GetStatus on Daemon Manager

**Files:**
- Modify: `internal/daemon/daemon.go`
- Modify: `internal/reconciler/reconciler_test.go`
- Modify: `internal/api/server_test.go`

- [ ] **Step 1: Add TailscaleStatus type and GetStatus to daemon.go**

In `internal/daemon/daemon.go`, add the status type after the existing imports:

```go
// TailscaleStatus represents parsed tailscale status --json output.
type TailscaleStatus struct {
	Self           StatusNode            `json:"Self"`
	Peer           map[string]StatusNode `json:"Peer"`
	MagicDNSSuffix string                `json:"MagicDNSSuffix"`
}

// StatusNode represents a node in tailscale status.
type StatusNode struct {
	HostName     string   `json:"HostName"`
	DNSName      string   `json:"DNSName"`
	TailscaleIPs []string `json:"TailscaleIPs"`
	Online       bool     `json:"Online"`
}
```

Add `GetStatus` to the `Manager` interface:

```go
type Manager interface {
	Start(tailnetID, nsName string) error
	Stop(nsName, tailnetID string) error
	CheckHealth(nsName, tailnetID string) (bool, error)
	GetSocketPath(tailnetID string) string
	AuthorizeDaemon(tailnetID, nsName, authKey string) error
	GetStatus(nsName, tailnetID string) (*TailscaleStatus, error)
}
```

Implement on `RealManager`:

```go
func (m *RealManager) GetStatus(nsName, tailnetID string) (*TailscaleStatus, error) {
	return GetStatus(nsName, tailnetID)
}
```

Add the standalone function (reuses the same `tailscale status --json` call as CheckHealth):

```go
// GetStatus returns parsed tailscale status for a tailnet.
func GetStatus(namespaceName string, tailnetID string) (*TailscaleStatus, error) {
	stateDir := filepath.Join(DefaultStateDir, tailnetID)
	socketPath := filepath.Join(stateDir, "tailscaled.sock")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ip", "netns", "exec", namespaceName,
		"tailscale", "--socket="+socketPath, "status", "--json")

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get tailscale status for %s: %w", tailnetID, err)
	}

	var status TailscaleStatus
	if err := json.Unmarshal(output, &status); err != nil {
		return nil, fmt.Errorf("failed to parse status JSON for %s: %w", tailnetID, err)
	}

	return &status, nil
}
```

Add `"encoding/json"` to the imports if not already present.

- [ ] **Step 2: Update mock in reconciler_test.go**

Add to `mockDaemon` in `internal/reconciler/reconciler_test.go`:

```go
func (m *mockDaemon) GetStatus(nsName, tailnetID string) (*daemon.TailscaleStatus, error) {
	return nil, nil
}
```

Add the `daemon` import if not present: `"hydrascale/internal/daemon"`

- [ ] **Step 3: Update mock in server_test.go**

Add to `mockDaemon` in `internal/api/server_test.go`:

```go
func (m *mockDaemon) GetStatus(nsName, tailnetID string) (*daemon.TailscaleStatus, error) {
	return nil, nil
}
```

Add the `daemon` import if not present: `"hydrascale/internal/daemon"`

- [ ] **Step 4: Run all tests**

Run: `cd /home/e/development/Hydrascale && go test ./...`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/daemon/daemon.go internal/reconciler/reconciler_test.go internal/api/server_test.go
git commit -m "feat: add GetStatus to daemon Manager interface"
```

---

## Task 3: Peer Map Parser

**Files:**
- Create: `internal/hostaccess/peers.go`
- Create: `internal/hostaccess/peers_test.go`

- [ ] **Step 1: Write all 5 peer parsing tests**

Create `internal/hostaccess/peers_test.go`:

```go
package hostaccess

import (
	"testing"

	"hydrascale/internal/daemon"
)

func TestParsePeers_ValidStatus(t *testing.T) {
	status := &daemon.TailscaleStatus{
		MagicDNSSuffix: "taildf854a.ts.net",
		Peer: map[string]daemon.StatusNode{
			"key1": {HostName: "mars", TailscaleIPs: []string{"100.98.107.70", "fd7a:115c:a1e0::1"}, Online: true},
			"key2": {HostName: "bigboy", TailscaleIPs: []string{"100.73.198.12"}, Online: true},
		},
	}
	peers := ParsePeers("havoc", status, "10.200.22.2", "vh5cde1b791fe1")
	if peers.TailnetID != "havoc" {
		t.Errorf("expected tailnetID=havoc, got %q", peers.TailnetID)
	}
	if peers.MagicDNSSuffix != "taildf854a.ts.net" {
		t.Errorf("expected MagicDNSSuffix, got %q", peers.MagicDNSSuffix)
	}
	if len(peers.Peers) != 2 {
		t.Fatalf("expected 2 peers, got %d", len(peers.Peers))
	}
	// Find mars
	var mars *Peer
	for i := range peers.Peers {
		if peers.Peers[i].Hostname == "mars" {
			mars = &peers.Peers[i]
			break
		}
	}
	if mars == nil {
		t.Fatal("mars peer not found")
	}
	if mars.IPv4 != "100.98.107.70" {
		t.Errorf("expected mars IPv4=100.98.107.70, got %q", mars.IPv4)
	}
	if mars.IPv6 != "fd7a:115c:a1e0::1" {
		t.Errorf("expected mars IPv6, got %q", mars.IPv6)
	}
	if !mars.Online {
		t.Error("expected mars to be online")
	}
}

func TestParsePeers_EmptyPeerList(t *testing.T) {
	status := &daemon.TailscaleStatus{
		MagicDNSSuffix: "tail123.ts.net",
		Peer:           map[string]daemon.StatusNode{},
	}
	peers := ParsePeers("test", status, "10.200.1.2", "vhtest")
	if len(peers.Peers) != 0 {
		t.Errorf("expected 0 peers, got %d", len(peers.Peers))
	}
}

func TestParsePeers_OfflinePeers(t *testing.T) {
	status := &daemon.TailscaleStatus{
		MagicDNSSuffix: "tail123.ts.net",
		Peer: map[string]daemon.StatusNode{
			"key1": {HostName: "offline-box", TailscaleIPs: []string{"100.1.2.3"}, Online: false},
		},
	}
	peers := ParsePeers("test", status, "10.200.1.2", "vhtest")
	if len(peers.Peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(peers.Peers))
	}
	if peers.Peers[0].Online {
		t.Error("expected peer to be offline")
	}
}

func TestParsePeers_NilStatus(t *testing.T) {
	peers := ParsePeers("test", nil, "10.200.1.2", "vhtest")
	if len(peers.Peers) != 0 {
		t.Errorf("expected 0 peers for nil status, got %d", len(peers.Peers))
	}
}

func TestParsePeers_MissingMagicDNSSuffix(t *testing.T) {
	status := &daemon.TailscaleStatus{
		MagicDNSSuffix: "",
		Peer: map[string]daemon.StatusNode{
			"key1": {HostName: "box", TailscaleIPs: []string{"100.1.2.3"}, Online: true},
		},
	}
	peers := ParsePeers("test", status, "10.200.1.2", "vhtest")
	if peers.MagicDNSSuffix != "" {
		t.Errorf("expected empty MagicDNSSuffix, got %q", peers.MagicDNSSuffix)
	}
	if len(peers.Peers) != 1 {
		t.Errorf("expected 1 peer, got %d", len(peers.Peers))
	}
}

func TestBuildDNSNames(t *testing.T) {
	peers := []Peer{
		{Hostname: "mars", IPv4: "100.98.107.70", IPv6: "fd7a:115c:a1e0::1"},
		{Hostname: "bigboy", IPv4: "100.73.198.12"},
	}
	records := BuildDNSNames("havoc", peers)
	if records["havoc-mars"] != "100.98.107.70" {
		t.Errorf("expected havoc-mars -> 100.98.107.70, got %q", records["havoc-mars"])
	}
	if records["havoc-bigboy"] != "100.73.198.12" {
		t.Errorf("expected havoc-bigboy -> 100.73.198.12, got %q", records["havoc-bigboy"])
	}
}

func TestBuildDNSNamesV6(t *testing.T) {
	peers := []Peer{
		{Hostname: "mars", IPv4: "100.98.107.70", IPv6: "fd7a:115c:a1e0::1"},
	}
	v4, v6 := BuildDNSRecords("havoc", peers)
	if v4["havoc-mars"] != "100.98.107.70" {
		t.Errorf("expected v4 havoc-mars, got %q", v4["havoc-mars"])
	}
	if v6["havoc-mars"] != "fd7a:115c:a1e0::1" {
		t.Errorf("expected v6 havoc-mars, got %q", v6["havoc-mars"])
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/e/development/Hydrascale && go test ./internal/hostaccess/ -v`
Expected: FAIL — package doesn't exist yet.

- [ ] **Step 3: Implement peers.go**

Create `internal/hostaccess/peers.go`:

```go
package hostaccess

import (
	"net"
	"strings"

	"hydrascale/internal/daemon"
)

// Peer represents a single tailscale peer with its addresses.
type Peer struct {
	Hostname string
	IPv4     string
	IPv6     string
	Online   bool
}

// TailnetPeers holds all peers for a single tailnet plus routing info.
type TailnetPeers struct {
	TailnetID      string
	MagicDNSSuffix string
	Peers          []Peer
	VethGateway    string
	VethHost       string
}

// ParsePeers extracts peer information from a TailscaleStatus.
func ParsePeers(tailnetID string, status *daemon.TailscaleStatus, vethGW, vethHost string) TailnetPeers {
	result := TailnetPeers{
		TailnetID:   tailnetID,
		VethGateway: vethGW,
		VethHost:    vethHost,
	}

	if status == nil {
		return result
	}

	result.MagicDNSSuffix = status.MagicDNSSuffix

	for _, node := range status.Peer {
		peer := Peer{
			Hostname: strings.ToLower(node.HostName),
			Online:   node.Online,
		}
		for _, ip := range node.TailscaleIPs {
			parsed := net.ParseIP(ip)
			if parsed == nil {
				continue
			}
			if parsed.To4() != nil {
				peer.IPv4 = ip
			} else {
				peer.IPv6 = ip
			}
		}
		if peer.IPv4 != "" || peer.IPv6 != "" {
			result.Peers = append(result.Peers, peer)
		}
	}

	return result
}

// BuildDNSNames returns a map of "<tailnetID>-<hostname>" -> IPv4 address.
func BuildDNSNames(tailnetID string, peers []Peer) map[string]string {
	records := make(map[string]string, len(peers))
	for _, p := range peers {
		if p.IPv4 != "" {
			records[tailnetID+"-"+p.Hostname] = p.IPv4
		}
	}
	return records
}

// BuildDNSRecords returns separate IPv4 and IPv6 maps of "<tailnetID>-<hostname>" -> IP.
func BuildDNSRecords(tailnetID string, peers []Peer) (v4 map[string]string, v6 map[string]string) {
	v4 = make(map[string]string, len(peers))
	v6 = make(map[string]string, len(peers))
	for _, p := range peers {
		name := tailnetID + "-" + p.Hostname
		if p.IPv4 != "" {
			v4[name] = p.IPv4
		}
		if p.IPv6 != "" {
			v6[name] = p.IPv6
		}
	}
	return v4, v6
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/e/development/Hydrascale && go test ./internal/hostaccess/ -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/hostaccess/peers.go internal/hostaccess/peers_test.go
git commit -m "feat: add peer map parser for host access"
```

---

## Task 4: Host Route Management

**Files:**
- Create: `internal/hostaccess/routes.go`
- Create: `internal/hostaccess/routes_test.go`

- [ ] **Step 1: Write route tests**

Create `internal/hostaccess/routes_test.go`:

```go
package hostaccess

import (
	"testing"
)

func TestParseHostRoutes(t *testing.T) {
	output := `100.98.107.70 via 10.200.22.2 dev vh5cde1b791fe1
100.73.198.12 via 10.200.22.2 dev vh5cde1b791fe1
default via 192.168.1.1 dev eth0
10.200.22.0/30 dev vh5cde1b791fe1 proto kernel scope link src 10.200.22.1
100.100.100.100 via 10.200.22.2 dev vh5cde1b791fe1
192.168.1.0/24 dev eth0 proto kernel scope link src 192.168.1.100
`
	routes := parseHostRoutes(output, "vh5cde1b791fe1")
	// Should include 100.98.107.70 and 100.73.198.12 (CGNAT range, correct dev)
	// Should NOT include 100.100.100.100 (infrastructure route)
	// Should NOT include 10.200.22.0/30 (veth infrastructure)
	// Should NOT include default or 192.168.1.0/24
	expected := map[string]bool{
		"100.98.107.70": true,
		"100.73.198.12": true,
	}
	if len(routes) != len(expected) {
		t.Fatalf("expected %d routes, got %d: %v", len(expected), len(routes), routes)
	}
	for _, r := range routes {
		if !expected[r] {
			t.Errorf("unexpected route: %s", r)
		}
	}
}

func TestDesiredRoutes(t *testing.T) {
	peers := TailnetPeers{
		Peers: []Peer{
			{Hostname: "mars", IPv4: "100.98.107.70", IPv6: "fd7a:115c:a1e0::1", Online: true},
			{Hostname: "offline", IPv4: "100.1.2.3", Online: false},
		},
	}
	v4, v6 := desiredRoutes(peers)
	if len(v4) != 2 {
		t.Errorf("expected 2 v4 routes (online+offline), got %d", len(v4))
	}
	if len(v6) != 1 {
		t.Errorf("expected 1 v6 route, got %d", len(v6))
	}
}

func TestRouteSync_DiffLogic(t *testing.T) {
	desired := []string{"100.1.1.1", "100.2.2.2", "100.3.3.3"}
	actual := []string{"100.1.1.1", "100.4.4.4"}

	toAdd, toRemove := diffRoutes(desired, actual)

	expectedAdd := map[string]bool{"100.2.2.2": true, "100.3.3.3": true}
	expectedRemove := map[string]bool{"100.4.4.4": true}

	if len(toAdd) != len(expectedAdd) {
		t.Fatalf("expected %d adds, got %d", len(expectedAdd), len(toAdd))
	}
	for _, r := range toAdd {
		if !expectedAdd[r] {
			t.Errorf("unexpected add: %s", r)
		}
	}
	if len(toRemove) != len(expectedRemove) {
		t.Fatalf("expected %d removes, got %d", len(expectedRemove), len(toRemove))
	}
	for _, r := range toRemove {
		if !expectedRemove[r] {
			t.Errorf("unexpected remove: %s", r)
		}
	}
}

func TestRouteSync_NoOp(t *testing.T) {
	desired := []string{"100.1.1.1", "100.2.2.2"}
	actual := []string{"100.1.1.1", "100.2.2.2"}

	toAdd, toRemove := diffRoutes(desired, actual)
	if len(toAdd) != 0 || len(toRemove) != 0 {
		t.Errorf("expected no-op, got add=%v remove=%v", toAdd, toRemove)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/e/development/Hydrascale && go test ./internal/hostaccess/ -run TestParseHostRoutes -v`
Expected: FAIL

- [ ] **Step 3: Implement routes.go**

Create `internal/hostaccess/routes.go`:

```go
package hostaccess

import (
	"fmt"
	"log"
	"net"
	"os/exec"
	"strings"
)

// isCGNAT returns true if the IP is in Tailscale's CGNAT range (100.64.0.0/10).
func isCGNAT(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	_, cgnat, _ := net.ParseCIDR("100.64.0.0/10")
	return cgnat.Contains(parsed)
}

// isTailscaleV6 returns true if the IP is in Tailscale's IPv6 range (fd7a:115c:a1e0::/48).
func isTailscaleV6(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	_, tsv6, _ := net.ParseCIDR("fd7a:115c:a1e0::/48")
	return tsv6.Contains(parsed)
}

// parseHostRoutes extracts Tailscale CGNAT peer IPs from ip route output,
// filtering to only routes via the specified veth device.
// Excludes 100.100.100.100 (MagicDNS infrastructure route).
func parseHostRoutes(output string, vethDev string) []string {
	var routes []string
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 1 {
			continue
		}
		dest := fields[0]
		// Skip non-IP destinations
		if dest == "default" || strings.Contains(dest, "/") {
			continue
		}
		ip := net.ParseIP(dest)
		if ip == nil {
			continue
		}
		// Must be via our veth device
		if !strings.Contains(line, "dev "+vethDev) {
			continue
		}
		// Must be in CGNAT range
		if !isCGNAT(dest) {
			continue
		}
		// Exclude MagicDNS infrastructure route
		if dest == "100.100.100.100" {
			continue
		}
		routes = append(routes, dest)
	}
	return routes
}

// parseHostRoutesV6 extracts Tailscale IPv6 peer IPs from ip -6 route output.
func parseHostRoutesV6(output string, vethDev string) []string {
	var routes []string
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 1 {
			continue
		}
		dest := fields[0]
		if dest == "default" || !strings.Contains(line, "dev "+vethDev) {
			continue
		}
		// Skip CIDR routes
		if strings.Contains(dest, "/") {
			continue
		}
		if isTailscaleV6(dest) {
			routes = append(routes, dest)
		}
	}
	return routes
}

// desiredRoutes extracts all peer IPs (including offline) as desired host routes.
func desiredRoutes(peers TailnetPeers) (v4 []string, v6 []string) {
	for _, p := range peers.Peers {
		if p.IPv4 != "" {
			v4 = append(v4, p.IPv4)
		}
		if p.IPv6 != "" {
			v6 = append(v6, p.IPv6)
		}
	}
	return v4, v6
}

// diffRoutes computes which routes to add and remove.
func diffRoutes(desired, actual []string) (toAdd, toRemove []string) {
	desiredSet := make(map[string]bool, len(desired))
	for _, r := range desired {
		desiredSet[r] = true
	}
	actualSet := make(map[string]bool, len(actual))
	for _, r := range actual {
		actualSet[r] = true
	}
	for _, r := range desired {
		if !actualSet[r] {
			toAdd = append(toAdd, r)
		}
	}
	for _, r := range actual {
		if !desiredSet[r] {
			toRemove = append(toRemove, r)
		}
	}
	return toAdd, toRemove
}

// SyncHostRoutes synchronizes host routes for a tailnet's peers.
func SyncHostRoutes(peers TailnetPeers) error {
	desiredV4, desiredV6 := desiredRoutes(peers)

	// Sync IPv4
	v4Output, err := exec.Command("ip", "route", "show", "dev", peers.VethHost).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to list v4 routes for %s: %w", peers.VethHost, err)
	}
	actualV4 := parseHostRoutes(string(v4Output), peers.VethHost)
	addV4, removeV4 := diffRoutes(desiredV4, actualV4)

	for _, ip := range addV4 {
		cmd := exec.Command("ip", "route", "replace", ip, "via", peers.VethGateway, "dev", peers.VethHost)
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("Failed to add host route %s via %s: %v (%s)", ip, peers.VethGateway, err, out)
		}
	}
	for _, ip := range removeV4 {
		cmd := exec.Command("ip", "route", "del", ip, "dev", peers.VethHost)
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("Failed to remove host route %s: %v (%s)", ip, err, out)
		}
	}

	// Sync IPv6
	v6Output, err := exec.Command("ip", "-6", "route", "show", "dev", peers.VethHost).CombinedOutput()
	if err != nil {
		// IPv6 may not be available, just log and continue
		log.Printf("Failed to list v6 routes for %s: %v", peers.VethHost, err)
		return nil
	}
	actualV6 := parseHostRoutesV6(string(v6Output), peers.VethHost)
	addV6, removeV6 := diffRoutes(desiredV6, actualV6)

	for _, ip := range addV6 {
		cmd := exec.Command("ip", "-6", "route", "replace", ip, "via", peers.VethGateway, "dev", peers.VethHost)
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("Failed to add v6 host route %s: %v (%s)", ip, err, out)
		}
	}
	for _, ip := range removeV6 {
		cmd := exec.Command("ip", "-6", "route", "del", ip, "dev", peers.VethHost)
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("Failed to remove v6 host route %s: %v (%s)", ip, err, out)
		}
	}

	return nil
}

// RemoveAllHostRoutes removes all host-access routes for a veth device.
func RemoveAllHostRoutes(vethDev string) {
	v4Output, err := exec.Command("ip", "route", "show", "dev", vethDev).CombinedOutput()
	if err == nil {
		for _, ip := range parseHostRoutes(string(v4Output), vethDev) {
			exec.Command("ip", "route", "del", ip, "dev", vethDev).Run()
		}
	}
	v6Output, err := exec.Command("ip", "-6", "route", "show", "dev", vethDev).CombinedOutput()
	if err == nil {
		for _, ip := range parseHostRoutesV6(string(v6Output), vethDev) {
			exec.Command("ip", "-6", "route", "del", ip, "dev", vethDev).Run()
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/e/development/Hydrascale && go test ./internal/hostaccess/ -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/hostaccess/routes.go internal/hostaccess/routes_test.go
git commit -m "feat: add host route management for host access"
```

---

## Task 5: /etc/hosts Managed Block

**Files:**
- Create: `internal/hostaccess/hosts.go`
- Create: `internal/hostaccess/hosts_test.go`

- [ ] **Step 1: Write all 6 hosts tests**

Create `internal/hostaccess/hosts_test.go`:

```go
package hostaccess

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const beginMarker = "# BEGIN HYDRASCALE MANAGED BLOCK - DO NOT EDIT"
const endMarker = "# END HYDRASCALE MANAGED BLOCK"

func TestInsertManagedBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts")
	os.WriteFile(path, []byte("127.0.0.1 localhost\n"), 0644)

	records := map[string]string{"havoc-mars": "100.98.107.70"}
	recordsV6 := map[string]string{"havoc-mars": "fd7a:115c:a1e0::1"}

	err := UpdateHostsFile(path, records, recordsV6)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, _ := os.ReadFile(path)
	content := string(data)
	if !strings.Contains(content, "127.0.0.1 localhost") {
		t.Error("original content missing")
	}
	if !strings.Contains(content, beginMarker) {
		t.Error("begin marker missing")
	}
	if !strings.Contains(content, "100.98.107.70  havoc-mars") {
		t.Error("IPv4 record missing")
	}
	if !strings.Contains(content, "fd7a:115c:a1e0::1  havoc-mars") {
		t.Error("IPv6 record missing")
	}
	if !strings.Contains(content, endMarker) {
		t.Error("end marker missing")
	}
}

func TestUpdateExistingBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts")
	existing := "127.0.0.1 localhost\n" + beginMarker + "\n100.1.1.1  old-entry\n" + endMarker + "\n"
	os.WriteFile(path, []byte(existing), 0644)

	records := map[string]string{"havoc-mars": "100.98.107.70"}
	err := UpdateHostsFile(path, records, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, _ := os.ReadFile(path)
	content := string(data)
	if strings.Contains(content, "old-entry") {
		t.Error("old entry should be replaced")
	}
	if !strings.Contains(content, "100.98.107.70  havoc-mars") {
		t.Error("new entry missing")
	}
}

func TestRemoveManagedBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts")
	existing := "127.0.0.1 localhost\n" + beginMarker + "\n100.1.1.1  old-entry\n" + endMarker + "\n"
	os.WriteFile(path, []byte(existing), 0644)

	err := UpdateHostsFile(path, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, _ := os.ReadFile(path)
	content := string(data)
	if strings.Contains(content, beginMarker) {
		t.Error("managed block should be removed")
	}
	if !strings.Contains(content, "127.0.0.1 localhost") {
		t.Error("non-managed content should be preserved")
	}
}

func TestPreserveNonManagedContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts")
	existing := "127.0.0.1 localhost\n::1 localhost\n# custom comment\n192.168.1.5 myserver\n"
	os.WriteFile(path, []byte(existing), 0644)

	records := map[string]string{"havoc-mars": "100.1.1.1"}
	err := UpdateHostsFile(path, records, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, _ := os.ReadFile(path)
	content := string(data)
	if !strings.Contains(content, "192.168.1.5 myserver") {
		t.Error("non-managed content should be preserved")
	}
	if !strings.Contains(content, "# custom comment") {
		t.Error("custom comments should be preserved")
	}
}

func TestSkipWriteWhenUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts")
	block := beginMarker + "\n100.1.1.1  havoc-mars\n" + endMarker + "\n"
	existing := "127.0.0.1 localhost\n" + block
	os.WriteFile(path, []byte(existing), 0644)
	info1, _ := os.Stat(path)

	records := map[string]string{"havoc-mars": "100.1.1.1"}
	err := UpdateHostsFile(path, records, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	info2, _ := os.Stat(path)
	// File should not be rewritten (same mod time on most systems, but size is sufficient check)
	if info1.Size() != info2.Size() {
		t.Error("file should not have been rewritten")
	}
}

func TestHostsFileNotExist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent", "hosts")

	records := map[string]string{"havoc-mars": "100.1.1.1"}
	err := UpdateHostsFile(path, records, nil)
	// Should handle gracefully — create or error
	if err == nil {
		// If it created the file, that's fine too
		data, _ := os.ReadFile(path)
		if !strings.Contains(string(data), "havoc-mars") {
			t.Error("expected record in new file")
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/e/development/Hydrascale && go test ./internal/hostaccess/ -run TestInsertManagedBlock -v`
Expected: FAIL

- [ ] **Step 3: Implement hosts.go**

Create `internal/hostaccess/hosts.go`:

```go
package hostaccess

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	hostsBeginMarker = "# BEGIN HYDRASCALE MANAGED BLOCK - DO NOT EDIT"
	hostsEndMarker   = "# END HYDRASCALE MANAGED BLOCK"
)

// buildManagedBlock generates the content between markers.
func buildManagedBlock(v4 map[string]string, v6 map[string]string) string {
	if len(v4) == 0 && len(v6) == 0 {
		return ""
	}

	// Sort names for deterministic output
	var names []string
	seen := make(map[string]bool)
	for name := range v4 {
		if !seen[name] {
			names = append(names, name)
			seen[name] = true
		}
	}
	for name := range v6 {
		if !seen[name] {
			names = append(names, name)
			seen[name] = true
		}
	}
	sort.Strings(names)

	var lines []string
	for _, name := range names {
		if ip, ok := v4[name]; ok {
			lines = append(lines, fmt.Sprintf("%s  %s", ip, name))
		}
		if ip, ok := v6[name]; ok {
			lines = append(lines, fmt.Sprintf("%s  %s", ip, name))
		}
	}

	return strings.Join(lines, "\n")
}

// UpdateHostsFile updates the managed block in /etc/hosts.
// If records are nil or empty, the managed block is removed.
// Skips write if the block content hasn't changed.
func UpdateHostsFile(path string, v4 map[string]string, v6 map[string]string) error {
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read %s: %w", path, err)
	}

	content := string(existing)
	newBlock := buildManagedBlock(v4, v6)

	// Extract existing managed block content (between markers)
	existingBlock := ""
	startIdx := strings.Index(content, hostsBeginMarker)
	endIdx := strings.Index(content, hostsEndMarker)
	if startIdx >= 0 && endIdx > startIdx {
		blockStart := startIdx + len(hostsBeginMarker) + 1 // +1 for newline
		if blockStart < endIdx {
			existingBlock = strings.TrimSpace(content[blockStart:endIdx])
		}
	}

	// Skip write if content hasn't changed
	if strings.TrimSpace(newBlock) == existingBlock {
		return nil
	}

	// Remove existing managed block if present
	if startIdx >= 0 && endIdx > startIdx {
		before := content[:startIdx]
		after := content[endIdx+len(hostsEndMarker):]
		// Trim trailing newline from the end marker line
		after = strings.TrimPrefix(after, "\n")
		content = before + after
	}

	// Remove trailing whitespace
	content = strings.TrimRight(content, "\n \t")

	// Append new block if we have records
	if newBlock != "" {
		if content != "" {
			content += "\n"
		}
		content += hostsBeginMarker + "\n" + newBlock + "\n" + hostsEndMarker + "\n"
	} else if content != "" {
		content += "\n"
	}

	// Atomic write: temp file + rename
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory for %s: %w", path, err)
	}

	tmp, err := os.CreateTemp(dir, ".hydrascale-hosts-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write temp hosts file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}

	// Preserve original file permissions
	if info, err := os.Stat(path); err == nil {
		os.Chmod(tmpPath, info.Mode())
	} else {
		os.Chmod(tmpPath, 0644)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename hosts file: %w", err)
	}

	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/e/development/Hydrascale && go test ./internal/hostaccess/ -run "Test.*Managed|Test.*Host|TestPreserve|TestSkip" -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/hostaccess/hosts.go internal/hostaccess/hosts_test.go
git commit -m "feat: add /etc/hosts managed block for host access DNS"
```

---

## Task 6: systemd-resolved D-Bus Integration

**Files:**
- Create: `internal/hostaccess/resolved.go`
- Create: `internal/hostaccess/resolved_test.go`

- [ ] **Step 1: Write resolved tests**

Create `internal/hostaccess/resolved_test.go`:

```go
package hostaccess

import (
	"testing"
)

func TestResolvedManager_NilWhenUnavailable(t *testing.T) {
	rm := NewResolvedManager()
	// On most test/CI environments, D-Bus may not be available.
	// The manager should handle this gracefully.
	err := rm.RegisterDomains([]string{"taildf854a.ts.net"})
	if err == nil {
		// If D-Bus is available (running on a desktop), deregister
		rm.DeregisterAll()
	}
	// Either way, no panic = pass
}

func TestResolvedManager_DeregisterIdempotent(t *testing.T) {
	rm := NewResolvedManager()
	// Should not panic or error even if never registered
	rm.DeregisterAll()
}

func TestResolvedManager_EmptyDomains(t *testing.T) {
	rm := NewResolvedManager()
	err := rm.RegisterDomains(nil)
	if err != nil {
		t.Errorf("registering nil domains should be a no-op, got: %v", err)
	}
}
```

- [ ] **Step 2: Implement resolved.go**

Create `internal/hostaccess/resolved.go`:

```go
package hostaccess

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
)

// ResolvedManager manages systemd-resolved DNS routing domains.
// Uses resolvectl CLI rather than D-Bus directly to avoid a heavy dependency.
type ResolvedManager struct {
	registered []string // domains currently registered
	ifIndex    string   // interface index for the DNS link
}

// NewResolvedManager creates a new ResolvedManager.
func NewResolvedManager() *ResolvedManager {
	return &ResolvedManager{}
}

// isAvailable checks if systemd-resolved is running.
func (rm *ResolvedManager) isAvailable() bool {
	err := exec.Command("systemctl", "is-active", "--quiet", "systemd-resolved").Run()
	return err == nil
}

// RegisterDomains registers routing domains with systemd-resolved.
// Uses resolvectl to set DNS routing domains on a dummy interface.
func (rm *ResolvedManager) RegisterDomains(domains []string) error {
	if len(domains) == 0 {
		return nil
	}

	if !rm.isAvailable() {
		return fmt.Errorf("systemd-resolved is not running")
	}

	// Use resolvectl to set routing domains
	// This tells resolved to route queries for these domains to our forwarder
	args := []string{"domain", "lo"} // use loopback as the link
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

	// Set our forwarder as the DNS server for this link
	cmd = exec.Command("resolvectl", "dns", "lo", "127.0.0.53:5354")
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("resolvectl dns failed: %v (%s)", err, out)
	}

	rm.registered = domains
	return nil
}

// DeregisterAll removes all registered routing domains.
func (rm *ResolvedManager) DeregisterAll() {
	if len(rm.registered) == 0 {
		return
	}
	// Reset the domain routing on loopback
	exec.Command("resolvectl", "revert", "lo").Run()
	rm.registered = nil
}
```

- [ ] **Step 3: Run tests**

Run: `cd /home/e/development/Hydrascale && go test ./internal/hostaccess/ -run TestResolved -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/hostaccess/resolved.go internal/hostaccess/resolved_test.go
git commit -m "feat: add systemd-resolved integration for host access DNS"
```

---

## Task 7: Host Access Manager (Sync & Teardown)

**Files:**
- Create: `internal/hostaccess/hostaccess.go`
- Create: `internal/hostaccess/hostaccess_test.go`

- [ ] **Step 1: Write integration tests**

Create `internal/hostaccess/hostaccess_test.go`:

```go
package hostaccess

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hydrascale/internal/daemon"
)

func TestSync_FullFlow(t *testing.T) {
	dir := t.TempDir()
	hostsPath := filepath.Join(dir, "hosts")
	os.WriteFile(hostsPath, []byte("127.0.0.1 localhost\n"), 0644)

	mgr := NewManager("hosts", hostsPath)

	status := &daemon.TailscaleStatus{
		MagicDNSSuffix: "taildf854a.ts.net",
		Peer: map[string]daemon.StatusNode{
			"key1": {HostName: "mars", TailscaleIPs: []string{"100.98.107.70"}, Online: true},
		},
	}

	// Sync should update hosts file (route sync will fail without root, that's OK)
	mgr.Sync("havoc", status, "10.200.22.2", "vhtest123")

	data, _ := os.ReadFile(hostsPath)
	content := string(data)
	if !strings.Contains(content, "havoc-mars") {
		t.Error("expected havoc-mars in hosts file")
	}
}

func TestSync_HostAccessDisabled(t *testing.T) {
	dir := t.TempDir()
	hostsPath := filepath.Join(dir, "hosts")
	os.WriteFile(hostsPath, []byte("127.0.0.1 localhost\n"), 0644)

	mgr := NewManager("hosts", hostsPath)

	// Sync with nil status should be a no-op
	mgr.Sync("havoc", nil, "10.200.22.2", "vhtest123")

	data, _ := os.ReadFile(hostsPath)
	if strings.Contains(string(data), "havoc") {
		t.Error("expected no havoc entries when status is nil")
	}
}

func TestSync_PartialFailure(t *testing.T) {
	dir := t.TempDir()
	hostsPath := filepath.Join(dir, "hosts")
	os.WriteFile(hostsPath, []byte("127.0.0.1 localhost\n"), 0644)

	mgr := NewManager("hosts", hostsPath)

	status := &daemon.TailscaleStatus{
		MagicDNSSuffix: "tail123.ts.net",
		Peer: map[string]daemon.StatusNode{
			"key1": {HostName: "box", TailscaleIPs: []string{"100.1.2.3"}, Online: true},
		},
	}

	// Routes will fail (no root), but hosts file should still update
	mgr.Sync("test", status, "10.200.1.2", "vhbaddev")

	data, _ := os.ReadFile(hostsPath)
	if !strings.Contains(string(data), "test-box") {
		t.Error("hosts file should update even if routes fail")
	}
}

func TestTeardown_Idempotent(t *testing.T) {
	dir := t.TempDir()
	hostsPath := filepath.Join(dir, "hosts")
	os.WriteFile(hostsPath, []byte("127.0.0.1 localhost\n"), 0644)

	mgr := NewManager("hosts", hostsPath)

	// Teardown with nothing set up should not panic
	mgr.Teardown("nonexistent")
}
```

- [ ] **Step 2: Implement hostaccess.go**

Create `internal/hostaccess/hostaccess.go`:

```go
package hostaccess

import (
	"log"
	"sync"

	"hydrascale/internal/daemon"
)

// Manager coordinates host access features: routes, DNS, and namespace setup.
type Manager struct {
	mu       sync.Mutex
	dnsMode  string // "hosts" or "resolved"
	hostsPath string
	resolved *ResolvedManager

	// Track which tailnets have been synced so teardown knows what to clean up
	activeTailnets map[string]TailnetPeers
}

// NewManager creates a new host access Manager.
func NewManager(dnsMode string, hostsPath string) *Manager {
	if hostsPath == "" {
		hostsPath = "/etc/hosts"
	}
	m := &Manager{
		dnsMode:        dnsMode,
		hostsPath:      hostsPath,
		activeTailnets: make(map[string]TailnetPeers),
	}
	if dnsMode == "resolved" {
		m.resolved = NewResolvedManager()
	}
	return m
}

// Sync updates host routes and DNS for a tailnet's peers.
func (m *Manager) Sync(tailnetID string, status *daemon.TailscaleStatus, vethGW, vethHost string) {
	peers := ParsePeers(tailnetID, status, vethGW, vethHost)

	if len(peers.Peers) == 0 && status == nil {
		return // no status available, skip
	}

	m.mu.Lock()
	m.activeTailnets[tailnetID] = peers
	m.mu.Unlock()

	// Sync host routes (best effort — requires root)
	if err := SyncHostRoutes(peers); err != nil {
		log.Printf("host-access: route sync failed for %s: %v", tailnetID, err)
	}

	// Sync DNS
	m.syncDNS()
}

// Teardown removes all host access state for a tailnet.
func (m *Manager) Teardown(tailnetID string) {
	m.mu.Lock()
	peers, exists := m.activeTailnets[tailnetID]
	if exists {
		delete(m.activeTailnets, tailnetID)
	}
	m.mu.Unlock()

	if exists {
		// Remove host routes
		RemoveAllHostRoutes(peers.VethHost)
	}

	// Re-sync DNS (will remove this tailnet's entries)
	m.syncDNS()
}

// TeardownAll removes all host access state. Called during shutdown.
func (m *Manager) TeardownAll() {
	m.mu.Lock()
	tailnets := make(map[string]TailnetPeers, len(m.activeTailnets))
	for k, v := range m.activeTailnets {
		tailnets[k] = v
	}
	m.activeTailnets = make(map[string]TailnetPeers)
	m.mu.Unlock()

	for _, peers := range tailnets {
		RemoveAllHostRoutes(peers.VethHost)
	}

	// Clear DNS
	m.syncDNS()

	if m.resolved != nil {
		m.resolved.DeregisterAll()
	}
}

// syncDNS updates DNS records based on all active tailnets.
func (m *Manager) syncDNS() {
	m.mu.Lock()
	allV4 := make(map[string]string)
	allV6 := make(map[string]string)
	var domains []string

	for _, peers := range m.activeTailnets {
		v4, v6 := BuildDNSRecords(peers.TailnetID, peers.Peers)
		for k, v := range v4 {
			allV4[k] = v
		}
		for k, v := range v6 {
			allV6[k] = v
		}
		if peers.MagicDNSSuffix != "" {
			domains = append(domains, peers.MagicDNSSuffix)
		}
	}
	m.mu.Unlock()

	switch m.dnsMode {
	case "hosts":
		if err := UpdateHostsFile(m.hostsPath, allV4, allV6); err != nil {
			log.Printf("host-access: failed to update hosts file: %v", err)
		}
	case "resolved":
		if m.resolved != nil {
			if err := m.resolved.RegisterDomains(domains); err != nil {
				log.Printf("host-access: resolved registration failed: %v", err)
			}
		}
	}
}
```

- [ ] **Step 3: Run tests**

Run: `cd /home/e/development/Hydrascale && go test ./internal/hostaccess/ -v`
Expected: All PASS

- [ ] **Step 4: Commit**

```bash
git add internal/hostaccess/hostaccess.go internal/hostaccess/hostaccess_test.go
git commit -m "feat: add host access manager with sync and teardown"
```

---

## Task 8: Namespace-Side iptables for Host Access

**Files:**
- Modify: `internal/namespaces/ns.go`

- [ ] **Step 1: Add host access iptables functions to ns.go**

In `internal/namespaces/ns.go`, add after the existing `SetupVeth` function:

```go
// SetupHostAccess adds namespace-side iptables rules for host access:
// - Masquerade on tailscale0 so host traffic is forwarded to peers
// - DNS DNAT on veth so MagicDNS queries from host reach 100.100.100.100
// - /etc/netns/NAME/resolv.conf for MagicDNS inside the namespace
// All rules are idempotent (check before insert).
func SetupHostAccess(nsName string, index int) error {
	_, nsVeth := vethNames(nsName)
	nsIP := fmt.Sprintf("10.200.%d.0/30", index)

	// Masquerade on tailscale0 — makes host traffic look like local tailscale traffic
	if exec.Command("ip", "netns", "exec", nsName, "iptables", "-t", "nat", "-C", "POSTROUTING", "-s", nsIP, "-o", "tailscale0", "-j", "MASQUERADE").Run() != nil {
		cmd := exec.Command("ip", "netns", "exec", nsName, "iptables", "-t", "nat", "-A", "POSTROUTING", "-s", nsIP, "-o", "tailscale0", "-j", "MASQUERADE")
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("host-access: failed to add tailscale0 masquerade in %s: %v (%s)", nsName, err, out)
		}
	}

	// DNS DNAT — forward DNS queries from veth to MagicDNS (100.100.100.100)
	if exec.Command("ip", "netns", "exec", nsName, "iptables", "-t", "nat", "-C", "PREROUTING", "-i", nsVeth, "-p", "udp", "--dport", "53", "-j", "DNAT", "--to-destination", "100.100.100.100:53").Run() != nil {
		cmd := exec.Command("ip", "netns", "exec", nsName, "iptables", "-t", "nat", "-A", "PREROUTING", "-i", nsVeth, "-p", "udp", "--dport", "53", "-j", "DNAT", "--to-destination", "100.100.100.100:53")
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("host-access: failed to add DNS DNAT (UDP) in %s: %v (%s)", nsName, err, out)
		}
	}
	if exec.Command("ip", "netns", "exec", nsName, "iptables", "-t", "nat", "-C", "PREROUTING", "-i", nsVeth, "-p", "tcp", "--dport", "53", "-j", "DNAT", "--to-destination", "100.100.100.100:53").Run() != nil {
		cmd := exec.Command("ip", "netns", "exec", nsName, "iptables", "-t", "nat", "-A", "PREROUTING", "-i", nsVeth, "-p", "tcp", "--dport", "53", "-j", "DNAT", "--to-destination", "100.100.100.100:53")
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("host-access: failed to add DNS DNAT (TCP) in %s: %v (%s)", nsName, err, out)
		}
	}

	// /etc/netns/NAME/resolv.conf — enables MagicDNS inside the namespace
	netnsDir := filepath.Join("/etc/netns", nsName)
	if err := os.MkdirAll(netnsDir, 0755); err != nil {
		log.Printf("host-access: failed to create %s: %v", netnsDir, err)
	} else {
		resolvPath := filepath.Join(netnsDir, "resolv.conf")
		if err := os.WriteFile(resolvPath, []byte("nameserver 100.100.100.100\n"), 0644); err != nil {
			log.Printf("host-access: failed to write %s: %v", resolvPath, err)
		}
	}

	log.Printf("Set up host access rules for namespace %s", nsName)
	return nil
}

// TeardownHostAccess removes namespace-side host access iptables rules and resolv.conf.
func TeardownHostAccess(nsName string, index int) {
	_, nsVeth := vethNames(nsName)
	nsIP := fmt.Sprintf("10.200.%d.0/30", index)

	// Remove masquerade
	exec.Command("ip", "netns", "exec", nsName, "iptables", "-t", "nat", "-D", "POSTROUTING", "-s", nsIP, "-o", "tailscale0", "-j", "MASQUERADE").Run()

	// Remove DNS DNAT
	exec.Command("ip", "netns", "exec", nsName, "iptables", "-t", "nat", "-D", "PREROUTING", "-i", nsVeth, "-p", "udp", "--dport", "53", "-j", "DNAT", "--to-destination", "100.100.100.100:53").Run()
	exec.Command("ip", "netns", "exec", nsName, "iptables", "-t", "nat", "-D", "PREROUTING", "-i", nsVeth, "-p", "tcp", "--dport", "53", "-j", "DNAT", "--to-destination", "100.100.100.100:53").Run()

	// Remove resolv.conf
	resolvPath := filepath.Join("/etc/netns", nsName, "resolv.conf")
	os.Remove(resolvPath)
	// Try to remove the directory (only succeeds if empty)
	os.Remove(filepath.Join("/etc/netns", nsName))
}
```

Add `"path/filepath"` to imports if not already present.

- [ ] **Step 2: Run all tests**

Run: `cd /home/e/development/Hydrascale && go test ./...`
Expected: All PASS

- [ ] **Step 3: Commit**

```bash
git add internal/namespaces/ns.go
git commit -m "feat: add namespace-side iptables and resolv.conf for host access"
```

---

## Task 9: Reconciler Integration

**Files:**
- Modify: `internal/reconciler/reconciler.go`
- Modify: `internal/reconciler/reconciler_test.go`

- [ ] **Step 1: Add hostaccess Manager to Reconciler**

In `internal/reconciler/reconciler.go`, add the import:

```go
"hydrascale/internal/hostaccess"
```

Add field to the `Reconciler` struct:

```go
ha *hostaccess.Manager
```

Update `New()` to accept and store a `*hostaccess.Manager` (can be nil if host access is not enabled):

```go
func New(configPath string, ns namespaces.Manager, dm daemon.Manager, rt routing.Manager, interval time.Duration, ha *hostaccess.Manager) *Reconciler {
	return &Reconciler{
		configPath:    configPath,
		ns:            ns,
		dm:            dm,
		rt:            rt,
		ha:            ha,
		interval:      interval,
		failureCounts: make(map[string]int),
		errorStates:   make(map[string]bool),
		pausedStates:  make(map[string]bool),
		lastErrors:    make(map[string]string),
	}
}
```

- [ ] **Step 2: Add host access sync to the reconcile cycle**

In `reconciler.go`, modify `executeAction` to handle the new `ActionSyncRoutes` case (after routes are synced and daemon is healthy). Add a new action type:

```go
ActionSyncHostAccess ActionType = "sync_host_access"
```

In the `Diff` method, after the `ActionSyncRoutes` action for healthy daemons with host_access enabled, add:

```go
// If host access is enabled for this tailnet, schedule host access sync
if r.ha != nil {
	cfg, _ := r.DesiredState()
	if tn, ok := cfg[id]; ok {
		fullCfg, _ := config.LoadConfig(r.configPath)
		if fullCfg != nil && fullCfg.TailnetHostAccess(tn.ID) {
			actions = append(actions, Action{Type: ActionSyncHostAccess, TailnetID: id, NsName: ns})
		}
	}
}
```

In `executeAction`, add the host access case:

```go
case ActionSyncHostAccess:
	if r.ha == nil {
		return nil
	}
	ns := r.ns.GetName(action.TailnetID)
	status, err := r.dm.GetStatus(ns, action.TailnetID)
	if err != nil {
		return fmt.Errorf("failed to get status for host access: %w", err)
	}
	vethHost, _ := namespaces.VethNames(ns)
	vethGW := fmt.Sprintf("10.200.%d.2", namespaces.VethIndex(ns))
	r.ha.Sync(action.TailnetID, status, vethGW, vethHost)
	return nil
```

- [ ] **Step 3: Add host access teardown to Shutdown**

In the `Shutdown` method, add before the final event emit:

```go
if r.ha != nil {
	r.ha.TeardownAll()
}
```

- [ ] **Step 4: Update test helper to pass nil hostaccess**

In `reconciler_test.go`, update `newTestReconciler`:

```go
func newTestReconciler(cfgPath string, ns *mockNS, dm *mockDaemon, rt *mockRouting) *Reconciler {
	return New(cfgPath, ns, dm, rt, 1*time.Second, nil)
}
```

- [ ] **Step 5: Update all callers of New() in the codebase**

Search for other callers of `reconciler.New()` and add the `nil` parameter for hostaccess:

Run: `grep -rn "reconciler.New(" /home/e/development/Hydrascale/ --include="*.go"`

Update each caller to pass the hostaccess Manager (or nil). The main caller will be in `cmd/hydrascale/main.go` — pass a real `hostaccess.Manager` when host access is configured.

- [ ] **Step 6: Run all tests**

Run: `cd /home/e/development/Hydrascale && go test ./...`
Expected: All PASS

- [ ] **Step 7: Commit**

```bash
git add internal/reconciler/reconciler.go internal/reconciler/reconciler_test.go cmd/hydrascale/main.go
git commit -m "feat: wire host access manager into reconciler cycle"
```

---

## Task 10: Main CLI Wiring

**Files:**
- Modify: `cmd/hydrascale/main.go`

- [ ] **Step 1: Import hostaccess and create manager in serve command**

In the `serve` command handler in `cmd/hydrascale/main.go`, after loading config:

```go
// Create host access manager if any tailnet has host_access enabled
var ha *hostaccess.Manager
dnsMode := cfg.EffectiveHostDNSMode()
if dnsMode != "" {
	ha = hostaccess.NewManager(dnsMode, "/etc/hosts")
}
```

Pass `ha` to `reconciler.New()`.

Add namespace host access setup in the reconciler's create_namespace action flow. When a namespace is created for a tailnet with host_access enabled, call `namespaces.SetupHostAccess()`.

- [ ] **Step 2: Run full test suite**

Run: `cd /home/e/development/Hydrascale && go test ./...`
Expected: All PASS

- [ ] **Step 3: Build and verify**

Run: `cd /home/e/development/Hydrascale && go build -o hydrascale ./cmd/hydrascale`
Expected: Compiles successfully

- [ ] **Step 4: Commit**

```bash
git add cmd/hydrascale/main.go
git commit -m "feat: wire host access into serve command"
```

---

## Task 11: Integration Test & Manual Verification

- [ ] **Step 1: Run all tests**

Run: `cd /home/e/development/Hydrascale && go test ./... -v`
Expected: All PASS, including all 25+ new test cases.

- [ ] **Step 2: Build final binary**

Run: `cd /home/e/development/Hydrascale && go build -o hydrascale ./cmd/hydrascale`

- [ ] **Step 3: Manual test with config**

Create a test config with host_access enabled:

```yaml
version: 2
host_access: true
tailnets:
  - id: havoc
  - id: personal
host_dns:
  mode: hosts
reconciler:
  interval: 30s
```

Install and test:

```bash
sudo install hydrascale /usr/local/bin/
sudo systemctl restart hydrascale
sleep 10
# Verify host routes exist
ip route | grep "100\."
# Verify /etc/hosts has managed block
grep HYDRASCALE /etc/hosts
# Verify TUI shows healthy
sudo hydrascale tui
# Test connectivity
ping -c2 havoc-bigboy
```

- [ ] **Step 4: Commit any fixes from manual testing**

- [ ] **Step 5: Final commit and push**

```bash
git push origin feat/host-access
```
