// Package reconciler implements the declarative reconciliation loop for Hydrascale.
// It compares desired state (from config) with actual state (from the system)
// and executes actions to converge them.
package reconciler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"hydrascale/internal/access"
	"hydrascale/internal/config"
	"hydrascale/internal/daemon"
	"hydrascale/internal/dns"
	"hydrascale/internal/hostaccess"
	"hydrascale/internal/namespaces"
	"hydrascale/internal/reach"
	"hydrascale/internal/routing"
)

// ActionType represents the type of reconciliation action.
type ActionType string

const (
	ActionCreateNS           ActionType = "create_namespace"
	ActionDeleteNS           ActionType = "delete_namespace"
	ActionStartDaemon        ActionType = "start_daemon"
	ActionStopDaemon         ActionType = "stop_daemon"
	ActionSyncRoutes         ActionType = "sync_routes"
	ActionSyncHostAccess     ActionType = "sync_host_access"
	ActionTeardownHostAccess ActionType = "teardown_host_access"
	ActionAuthDaemon         ActionType = "auth_daemon"
	ActionRefreshDNS         ActionType = "refresh_dns"
)

// MaxFailures is the number of consecutive failures before a tailnet enters error state.
const MaxFailures = 3

// Action represents a single reconciliation action to be executed.
type Action struct {
	Type       ActionType
	TailnetID  string
	NsName     string
	AuthKey    string // Used by ActionAuthDaemon
	ControlURL string // Used by ActionAuthDaemon for custom control servers (e.g. Headscale)
}

func (a Action) String() string {
	return fmt.Sprintf("%s %s (%s)", a.Type, a.TailnetID, a.NsName)
}

// ErrTailnetNotFound is returned by GetTailscaleStatus when the tailnet ID is not in config.
var ErrTailnetNotFound = errors.New("tailnet not found")

// TailnetState represents the observed state of a single tailnet.
type TailnetState struct {
	ID            string
	NsName        string
	NsExists      bool
	DaemonHealthy bool
	Routes        []routing.Route
	// MeasuredReachability holds the last measurement that the daemon made of the
	// reachability of the namespace. DaemonHealthy states that the tailscaled process
	// runs; this field states what one packet did. The two answer different questions,
	// therefore a namespace reports both.
	MeasuredReachability *reach.Result `json:"measured_reachability"`
}

// Event represents a structured reconciler event for debugging and future API use.
type Event struct {
	Time      time.Time
	Type      string
	TailnetID string
	Message   string
}

// Reconciler drives actual state toward desired state.
type Reconciler struct {
	configPath  string
	ns          namespaces.Manager
	dm          daemon.Manager
	rt          routing.Manager
	ha          *hostaccess.Manager
	interval    time.Duration
	infraSubnet string

	mu            sync.Mutex
	failureCounts map[string]int    // tailnetID -> consecutive failure count
	errorStates   map[string]bool   // tailnetID -> true if in error state
	pausedStates  map[string]bool   // tailnetID -> true if manually disconnected
	lastErrors    map[string]string // tailnetID -> last error message
	unprotected   map[string]string // tailnetID -> the reported overlay mount error
	events        []Event

	// jumpPositions holds the position of the jump rule of the daemon in each parent
	// chain, as the last tick measured it. A parent chain that holds no jump rule gets
	// the position 0. The reconciler records an event for a change of position only, so
	// a host that keeps its chain adds no event on the next tick.
	jumpPositions map[string]int

	// dnsRefreshPending holds true for a tailnet that waits for the DNS refresh, and
	// dnsRefreshReported holds true for a tailnet whose wait the reconciler recorded once.
	// The restart path plans the refresh, and the refresh runs only against a tailscaled in
	// the Running state, therefore the reconciler keeps the action planned across ticks.
	dnsRefreshPending  map[string]bool
	dnsRefreshReported map[string]bool

	// hostAccessRules holds true for a tailnet whose namespace carries the host access
	// rules, and false for a tailnet whose rules the reconciler removed. A tailnet with no
	// entry is a tailnet the reconciler has not seen since it started, so the first cycle
	// tears the rules down once when the operator set host_access to false.
	hostAccessRules map[string]bool

	// teardownHostAccess removes the namespace-side host access rules. A test replaces it.
	teardownHostAccess func(nsName string, index int, infraSubnet string) error

	// reaper removes each host rule that names a veth device that is gone. New sets it
	// when the namespace manager carries that ability.
	reaper StaleRuleReaper

	// access writes the compiled local rule set into the chains that the daemon owns.
	// A test replaces it.
	access ChainWriter

	// path writes the host rules of the forward path of a namespace that the host does
	// not hold. New sets it when the namespace manager carries that ability.
	path ForwardPathWriter

	// prober measures the reachability of a namespace with one packet. A test replaces
	// it. reachability holds the last measurement of each tailnet, because the status
	// route reads the state between two ticks and never sends a packet of its own.
	prober       NamespaceProber
	reachability map[string]reach.Result

	hostFile *dns.HostFileMonitor

	eventLogPath string
	eventFile    *os.File
}

// StaleRuleReaper removes the host rules that no version of the daemon writes any more.
// namespaces.RealManager carries the ability. A test double does not have to.
type StaleRuleReaper interface {
	// ReapStaleRules removes each host rule that names a veth device that no longer
	// exists.
	ReapStaleRules() (int, error)
	// RemoveLegacyForwardRules removes each FORWARD rule that version 0.9 wrote for a
	// host veth device.
	RemoveLegacyForwardRules() (int, error)
	// NamespaceForwarding returns the value of net.ipv4.ip_forward inside the namespace.
	NamespaceForwarding(nsName string) (string, error)
}

// ForwardPathWriter writes the host rules of the forward path of one namespace.
// namespaces.RealManager carries the ability. A test double does not have to.
type ForwardPathWriter interface {
	// EnsureForwardPath writes each host rule of the forward path of the namespace that
	// the host does not hold, and it returns the rules that it wrote.
	EnsureForwardPath(nsName string, index int, infraSubnet string) ([]string, error)
}

// ChainWriter writes the compiled local rule set into the chains that the daemon owns.
// access.Writer carries the ability.
type ChainWriter interface {
	// Apply makes the chains hold the compiled rule set. It returns the write state and
	// the placement of each jump rule that the daemon owns.
	Apply(ctx context.Context, c access.Compiled) (access.Result, error)
	// Teardown removes both chains and both jump rules.
	Teardown(ctx context.Context) error
}

// NamespaceProber measures the reachability of one namespace with one packet.
// reach.Prober carries the ability. A test double does not have to.
type NamespaceProber interface {
	// Probe sends one packet from nsName to target and reports what came back. An empty
	// target selects reach.DefaultTarget.
	Probe(ctx context.Context, nsName, target string) reach.Result
}

// SetProber replaces the prober that measures the reachability of a namespace. A test
// uses it to give a measurement without a packet on the host.
func (r *Reconciler) SetProber(p NamespaceProber) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.prober = p
}

// probeBudget bounds the whole measurement step of one tick. The reconciler probes every
// namespace at the same time, therefore the step costs one budget for a host with one
// namespace and for a host with eight.
const probeBudget = 400 * time.Millisecond

// probeReachability measures the reachability of each namespace and stores the result, so
// that the status route reports what one packet did and never what a rule says.
//
// The step sends one packet from each namespace at the same time, and probeBudget bounds
// the whole step. A probe that gets no answer inside the budget reports unreachable, which
// is the answer that the operator needs: a path that carries nothing inside 400 ms carries
// nothing that the operator can use.
//
// The daemon sends no packet from the namespace of a tailnet that the operator stopped, and
// none from a namespace that does not exist. Both report not_probed with the reason,
// therefore an operator who stops a tailnet on purpose reads no fault.
func (r *Reconciler) probeReachability(desired map[string]config.Tailnet, actual map[string]*TailnetState) {
	r.mu.Lock()
	prober := r.prober
	r.mu.Unlock()
	if prober == nil {
		return
	}

	// A configuration file that the daemon cannot read leaves the target empty, which
	// makes the prober measure against reach.DefaultTarget.
	var target string
	if cfg, err := config.LoadConfig(r.configPath); err == nil {
		target = cfg.ProbeTarget
	}

	ctx, cancel := context.WithTimeout(context.Background(), probeBudget)
	defer cancel()

	// mu guards results, because a goroutine writes the measurement of each namespace
	// while the loop still writes the result of a namespace that it does not probe.
	results := make(map[string]reach.Result, len(desired))
	var mu sync.Mutex
	var wg sync.WaitGroup

	record := func(id string, res reach.Result) {
		mu.Lock()
		results[id] = res
		mu.Unlock()
	}

	for id := range desired {
		r.mu.Lock()
		paused := r.pausedStates[id]
		r.mu.Unlock()

		state, ok := actual[id]
		switch {
		case paused:
			record(id, reach.Result{
				State:     reach.StateNotProbed,
				CheckedAt: time.Now(),
				Detail:    "the operator stopped this tailnet",
			})
			continue
		case !ok || !state.NsExists:
			record(id, reach.Result{
				State:     reach.StateNotProbed,
				CheckedAt: time.Now(),
				Detail:    "the namespace does not exist",
			})
			continue
		}

		wg.Add(1)
		go func(id, nsName string) {
			defer wg.Done()
			record(id, prober.Probe(ctx, nsName, target))
		}(id, state.NsName)
	}
	wg.Wait()

	r.mu.Lock()
	for id, res := range results {
		r.reachability[id] = res
	}
	r.mu.Unlock()

	for id, res := range results {
		if state, ok := actual[id]; ok {
			value := res
			state.MeasuredReachability = &value
		}
	}
}

// SetChainWriter replaces the writer of the local rule set. A test uses it to observe the
// compiled rule set without a command on the host.
func (r *Reconciler) SetChainWriter(w ChainWriter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.access = w
}

// New creates a new Reconciler with the given dependencies.
func New(configPath string, ns namespaces.Manager, dm daemon.Manager, rt routing.Manager, interval time.Duration, ha *hostaccess.Manager, infraSubnet string) *Reconciler {
	if infraSubnet == "" {
		infraSubnet = "10.200.0.0/16"
	}
	r := &Reconciler{
		configPath:         configPath,
		ns:                 ns,
		dm:                 dm,
		rt:                 rt,
		ha:                 ha,
		interval:           interval,
		infraSubnet:        infraSubnet,
		failureCounts:      make(map[string]int),
		errorStates:        make(map[string]bool),
		pausedStates:       make(map[string]bool),
		lastErrors:         make(map[string]string),
		unprotected:        make(map[string]string),
		jumpPositions:      make(map[string]int),
		dnsRefreshPending:  make(map[string]bool),
		dnsRefreshReported: make(map[string]bool),
		hostAccessRules:    make(map[string]bool),
		reachability:       make(map[string]reach.Result),
		teardownHostAccess: namespaces.TeardownHostAccess,
		hostFile:           dns.NewHostFileMonitor(dns.DefaultHostResolvConf),
	}
	if reaper, ok := ns.(StaleRuleReaper); ok {
		r.reaper = reaper
	}
	if path, ok := ns.(ForwardPathWriter); ok {
		r.path = path
	}
	// Only a Reconciler that drives the live host writes the chains. A test that passes a
	// namespace manager double gets no writer, and it sets one with SetChainWriter.
	if _, ok := ns.(*namespaces.RealManager); ok {
		r.access = access.NewWriter()
		// Only a Reconciler that drives the live host sends a packet. A test that passes
		// a namespace manager double gets no prober, and it sets one with SetProber.
		r.prober = reach.New()
	}
	return r
}

// SetHostFileMonitor replaces the monitor of the host resolv.conf file.
// A test uses SetHostFileMonitor to observe a temporary file.
func (r *Reconciler) SetHostFileMonitor(m *dns.HostFileMonitor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hostFile = m
}

// HostFileMonitor returns the monitor of the host resolv.conf file.
func (r *Reconciler) HostFileMonitor() *dns.HostFileMonitor {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.hostFile
}

// DesiredState reads the config and returns the desired set of tailnets.
func (r *Reconciler) DesiredState() (map[string]config.Tailnet, error) {
	cfg, err := config.LoadConfig(r.configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	desired := make(map[string]config.Tailnet, len(cfg.Tailnets))
	for _, tn := range cfg.Tailnets {
		desired[tn.ID] = tn
	}
	return desired, nil
}

// ActualState queries the system for the current state of all Hydrascale namespaces.
func (r *Reconciler) ActualState() (map[string]*TailnetState, error) {
	nsList, err := r.ns.List()
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}

	actual := make(map[string]*TailnetState)
	for _, nsName := range nsList {
		// Only consider Hydrascale namespaces (ns-* prefix)
		tailnetID := r.ns.GetTailnetID(nsName)
		if tailnetID == "" {
			continue
		}

		state := &TailnetState{
			ID:                   tailnetID,
			NsName:               nsName,
			NsExists:             true,
			MeasuredReachability: r.lastReachability(tailnetID),
		}

		// Check daemon health with timeout
		healthy, _ := r.dm.CheckHealth(nsName, tailnetID)
		state.DaemonHealthy = healthy

		// Get routes if daemon is healthy
		if healthy {
			socketPath := r.dm.GetSocketPath(tailnetID)
			routes, err := r.rt.PollStatus(nsName, socketPath)
			if err == nil {
				state.Routes = routes
			}
		}

		actual[tailnetID] = state
	}

	return actual, nil
}

// lastReachability returns the measurement that the last tick made for the tailnet.
// The status route reads the state between two ticks, therefore it reads the stored
// measurement and it sends no packet of its own. A tailnet with no measurement yet gets
// not_probed with the reason, so the field is present for every namespace.
func (r *Reconciler) lastReachability(tailnetID string) *reach.Result {
	r.mu.Lock()
	res, ok := r.reachability[tailnetID]
	r.mu.Unlock()
	if !ok {
		return &reach.Result{
			State:  reach.StateNotProbed,
			Detail: "the daemon made no measurement since it started",
		}
	}
	return &res
}

// Diff computes the actions needed to move from actual to desired state.
func (r *Reconciler) Diff(desired map[string]config.Tailnet, actual map[string]*TailnetState) []Action {
	var actions []Action

	cfg, cfgErr := config.LoadConfig(r.configPath)

	// Create/start tailnets that should exist but don't
	for id := range desired {
		r.mu.Lock()
		inError := r.errorStates[id]
		isPaused := r.pausedStates[id]
		r.mu.Unlock()

		if inError {
			continue // skip tailnets in error state
		}
		if isPaused {
			continue // skip manually disconnected tailnets
		}

		ns := r.ns.GetName(id)
		state, exists := actual[id]

		if !exists || !state.NsExists {
			// Two-phase reconcile: new tailnets only get create+start on first cycle.
			// Route sync happens on the next cycle once the daemon is healthy.
			// This avoids a race where SyncRoutes fails because the daemon socket
			// doesn't exist yet (StartDaemon is non-blocking via cmd.Start()).
			actions = append(actions, Action{Type: ActionCreateNS, TailnetID: id, NsName: ns})
			actions = append(actions, Action{Type: ActionStartDaemon, TailnetID: id, NsName: ns})
			// If an auth key is available, schedule authorization after daemon start
			authKey := config.ResolveAuthKey(id, desired[id].AuthKey)
			if authKey != "" {
				var controlURL string
				if cfgErr == nil {
					controlURL = config.ResolveControlURL(desired[id].ControlURL, cfg.ControlURL)
				}
				actions = append(actions, Action{Type: ActionAuthDaemon, TailnetID: id, NsName: ns, AuthKey: authKey, ControlURL: controlURL})
			}
		} else if !state.DaemonHealthy {
			// Restart path: tailscaled loads state from disk but doesn't re-read
			// resolv.conf, so its MagicDNS resolver chain can come up wedged.
			// Follow the start with a DNS refresh (accept-dns flip-flop) to
			// force tailscaled to rebuild the chain from the current namespace
			// resolv.conf. Without this, dig @100.100.100.100 returns SERVFAIL
			// for every query until someone manually runs `tailscale set`.
			actions = append(actions, Action{Type: ActionStartDaemon, TailnetID: id, NsName: ns})
			r.setDNSRefreshPending(id, true)
			actions = append(actions, Action{Type: ActionRefreshDNS, TailnetID: id, NsName: ns})
		} else {
			// The refresh runs only against a tailscaled in the Running state, and the
			// process reports healthy before it reaches that state. A tailnet that still
			// waits therefore keeps the action on this branch, until the refresh runs.
			if r.dnsRefreshWaits(id) {
				actions = append(actions, Action{Type: ActionRefreshDNS, TailnetID: id, NsName: ns})
			}
			// Daemon healthy, sync routes
			actions = append(actions, Action{Type: ActionSyncRoutes, TailnetID: id, NsName: ns})
			// Sync host access if enabled for this tailnet
			if cfgErr == nil && cfg.TailnetHostAccess(id) {
				actions = append(actions, Action{Type: ActionSyncHostAccess, TailnetID: id, NsName: ns})
			} else if cfgErr == nil && r.hostAccessRulesPresent(id) {
				// The operator set host_access to false, so remove the rules that the
				// earlier value wrote. Without this action the namespace keeps the
				// masquerade rule and the two DNS rules, and reachability does not change.
				actions = append(actions, Action{Type: ActionTeardownHostAccess, TailnetID: id, NsName: ns})
			}
		}
	}

	// Stop/delete tailnets that exist but shouldn't
	for id, state := range actual {
		if _, wanted := desired[id]; !wanted {
			actions = append(actions, Action{Type: ActionStopDaemon, TailnetID: id, NsName: state.NsName})
			actions = append(actions, Action{Type: ActionDeleteNS, TailnetID: id, NsName: state.NsName})
		}
	}

	return actions
}

// Apply executes a list of actions. Each action runs independently.
// Failure counting is per-tailnet per-cycle: only clear on a full cycle
// with zero failures for that tailnet.
func (r *Reconciler) Apply(actions []Action) {
	// Track which tailnets had failures in this cycle
	cycleFailures := make(map[string]bool)

	// Track error messages for LastError reporting
	cycleErrors := make(map[string]string)

	for _, action := range actions {
		err := r.executeAction(action)
		if err != nil {
			errMsg := fmt.Sprintf("%s: %v", action.Type, err)
			r.emit("action_failed", action.TailnetID, errMsg)
			cycleFailures[action.TailnetID] = true
			cycleErrors[action.TailnetID] = errMsg
		} else {
			r.emit("action_ok", action.TailnetID, string(action.Type))
		}
	}

	// Update failure counts: increment for tailnets with failures,
	// clear only for tailnets where ALL actions succeeded this cycle.
	tailnetsSeen := make(map[string]bool)
	for _, action := range actions {
		tailnetsSeen[action.TailnetID] = true
	}
	for id := range tailnetsSeen {
		if cycleFailures[id] {
			r.recordFailure(id, cycleErrors[id])
		} else {
			r.clearFailure(id)
		}
	}
}

// safeStateDir returns the state directory for a tailnet id and whether it is
// safe to operate on. It guards the root-level state-dir RemoveAll (#32) against
// path traversal: the id must be a valid tailnet id AND resolve to a direct
// child of the state dir, so a crafted id (e.g. "..", "a/b") can never escape.
func safeStateDir(tailnetID string) (string, bool) {
	if !config.IsValidID(tailnetID) {
		return "", false
	}
	dir := filepath.Join(daemon.DefaultStateDir, tailnetID)
	if filepath.Dir(dir) != filepath.Clean(daemon.DefaultStateDir) {
		return "", false
	}
	return dir, true
}

// hostAccessRulesPresent reports whether the namespace of the tailnet can still carry the
// host access rules. A tailnet with no record is a tailnet that the reconciler has not
// seen since it started, so the answer is true and the first cycle removes the rules once.
func (r *Reconciler) hostAccessRulesPresent(tailnetID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	present, seen := r.hostAccessRules[tailnetID]
	return !seen || present
}

// dnsRefreshWaits reports whether the DNS refresh of the tailnet waits for the Running
// state of tailscaled.
func (r *Reconciler) dnsRefreshWaits(tailnetID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.dnsRefreshPending[tailnetID]
}

// setDNSRefreshPending records whether the DNS refresh of the tailnet waits for the Running
// state of tailscaled.
// A change of the state clears the report state, therefore the reconciler records one event
// for one wait and a repeated tick adds no event.
func (r *Reconciler) setDNSRefreshPending(tailnetID string, pending bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.dnsRefreshPending[tailnetID] != pending {
		delete(r.dnsRefreshReported, tailnetID)
	}
	if pending {
		r.dnsRefreshPending[tailnetID] = true
		return
	}
	delete(r.dnsRefreshPending, tailnetID)
}

// reportDNSRefreshWait records the event dns.refresh_waits one time for one wait, so that
// the operator reads that the refresh of the tailnet is still open.
func (r *Reconciler) reportDNSRefreshWait(tailnetID string) {
	r.mu.Lock()
	reported := r.dnsRefreshReported[tailnetID]
	r.dnsRefreshReported[tailnetID] = true
	r.mu.Unlock()
	if reported {
		return
	}
	r.emit("dns.refresh_waits", tailnetID,
		"tailscaled is not in the Running state, therefore the refresh runs on a later tick")
}

// setHostAccessRules records whether the namespace of the tailnet carries the host access
// rules.
func (r *Reconciler) setHostAccessRules(tailnetID string, present bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hostAccessRules[tailnetID] = present
}

// reportTeardown records teardown.failed with the text of each failed step and returns the
// failures together. reportTeardown returns nil when every step succeeded.
func (r *Reconciler) reportTeardown(tailnetID string, errs []error) error {
	err := errors.Join(errs...)
	if err == nil {
		return nil
	}
	r.emit("teardown.failed", tailnetID, err.Error())
	return err
}

func (r *Reconciler) executeAction(action Action) error {
	switch action.Type {
	case ActionCreateNS:
		if err := r.ns.Create(action.TailnetID, r.infraSubnet); err != nil {
			return err
		}
		nsName := r.ns.GetName(action.TailnetID)
		// Always materialise /etc/netns/<ns>/resolv.conf so `ip netns exec`
		// bind-mounts it over /etc/resolv.conf — otherwise tailscaled
		// --accept-dns=true rewrites the host's /etc/resolv.conf to point
		// at 100.100.100.100 and breaks host DNS. See issue #22.
		if err := namespaces.WriteNamespaceResolvConf(nsName); err != nil {
			log.Printf("namespace: failed to write resolv.conf for %s: %v", nsName, err)
		}
		// Set up host access iptables if enabled for this tailnet
		if cfg, err := config.LoadConfig(r.configPath); err == nil && cfg.TailnetHostAccess(action.TailnetID) {
			index := namespaces.VethIndex(nsName)
			if err := namespaces.SetupHostAccess(nsName, index, r.infraSubnet); err != nil {
				log.Printf("host-access: setup failed for %s: %v", nsName, err)
			} else {
				r.setHostAccessRules(action.TailnetID, true)
			}
		}
		return nil
	case ActionDeleteNS:
		// A step that fails does not stop the remaining steps. Every failure reaches the
		// caller together, and teardown.failed names each one.
		var errs []error
		if err := r.ns.Delete(action.NsName, r.infraSubnet); err != nil {
			errs = append(errs, fmt.Errorf("delete the namespace %s: %w", action.NsName, err))
		}
		r.setHostAccessRules(action.TailnetID, false)
		r.setDNSRefreshPending(action.TailnetID, false)
		// The host access manager keeps the names of the tailnet in the /etc/hosts block
		// until it removes the tailnet, so the host resolves a name on a tailnet that it
		// no longer joins. See issue #69.
		if r.ha != nil {
			if err := r.ha.Teardown(action.TailnetID); err != nil {
				errs = append(errs, fmt.Errorf("remove the host access state of %s: %w", action.TailnetID, err))
			}
		}
		// Remove the tailnet's state dir (tailscaled state/socket + the #28
		// overlay upper/work dirs) so `remove` leaves nothing behind — otherwise
		// stale per-tailnet dirs accumulate under /var/lib/hydrascale/state. The
		// directory holds the node private key, so a failure reaches the operator.
		//
		// The id is derived from a live namespace name (not the IsValidID-
		// validated config), so validate the path before this root-level
		// RemoveAll — never let a crafted id (e.g. "..") traverse out.
		if stateDir, ok := safeStateDir(action.TailnetID); ok {
			if err := os.RemoveAll(stateDir); err != nil {
				errs = append(errs, fmt.Errorf("remove the state directory %s: %w", stateDir, err))
			}
		}
		return r.reportTeardown(action.TailnetID, errs)
	case ActionTeardownHostAccess:
		nsName := r.ns.GetName(action.TailnetID)
		index := namespaces.VethIndex(nsName)
		if err := r.teardownHostAccess(nsName, index, r.infraSubnet); err != nil {
			return r.reportTeardown(action.TailnetID, []error{
				fmt.Errorf("remove the host access rules of %s: %w", nsName, err)})
		}
		r.setHostAccessRules(action.TailnetID, false)
		return nil
	case ActionStartDaemon:
		// Belt-and-braces: ensure the namespace's resolv.conf bind-mount
		// override exists before tailscaled launches. ActionCreateNS already
		// writes it on first creation, but on a daemon restart the namespace
		// may already exist and we'd otherwise miss this. Idempotent. See #22.
		if err := namespaces.WriteNamespaceResolvConf(action.NsName); err != nil {
			log.Printf("namespace: failed to write resolv.conf for %s: %v", action.NsName, err)
		}
		allowUnprotected := false
		if cfg, err := config.LoadConfig(r.configPath); err == nil {
			allowUnprotected = cfg.DNS.AllowUnprotected
		}
		return r.dm.Start(action.TailnetID, action.NsName, allowUnprotected)
	case ActionStopDaemon:
		return r.dm.Stop(action.NsName, action.TailnetID)
	case ActionAuthDaemon:
		return r.dm.AuthorizeDaemon(action.TailnetID, action.NsName, action.AuthKey, action.ControlURL)
	case ActionRefreshDNS:
		// The call reads the state one time and it waits for no deadline, therefore one
		// tailnet holds no other tailnet. See issue #223.
		refreshed, err := r.dm.RefreshDNSConfigIfReady(action.TailnetID, action.NsName)
		if err != nil {
			return err
		}
		if !refreshed {
			r.reportDNSRefreshWait(action.TailnetID)
			return nil
		}
		r.setDNSRefreshPending(action.TailnetID, false)
		return nil
	case ActionSyncRoutes:
		socketPath := r.dm.GetSocketPath(action.TailnetID)
		routes, err := r.rt.PollStatus(action.NsName, socketPath)
		if err != nil {
			return err
		}
		return r.rt.SyncRoutes(action.NsName, routes, r.infraSubnet)
	case ActionSyncHostAccess:
		if r.ha == nil {
			return nil
		}
		nsName := r.ns.GetName(action.TailnetID)
		index := namespaces.VethIndex(nsName)
		// Ensure namespace-side iptables are set up (idempotent — safe every cycle)
		if err := namespaces.SetupHostAccess(nsName, index, r.infraSubnet); err != nil {
			return fmt.Errorf("host-access: setup failed for %s: %w", nsName, err)
		}
		r.setHostAccessRules(action.TailnetID, true)
		status, err := r.dm.GetStatus(context.Background(), nsName, action.TailnetID)
		if err != nil {
			return fmt.Errorf("host-access: failed to get status for %s: %w", action.TailnetID, err)
		}
		_, _, _, vethGW, err := namespaces.VethIPs(r.infraSubnet, index)
		if err != nil {
			return fmt.Errorf("host-access: failed to get veth IPs: %w", err)
		}
		vethHost, _ := namespaces.VethNames(nsName)
		r.ha.Sync(action.TailnetID, status, vethGW, vethHost, nsName)
		return nil
	default:
		return fmt.Errorf("unknown action type: %s", action.Type)
	}
}

// Reconcile runs a single reconciliation cycle.
// It acquires a file lock, loads config, computes diff, and applies actions.
func (r *Reconciler) Reconcile() error {
	lockPath := lockPathFor(r.configPath)
	unlock, err := acquireLock(lockPath)
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer unlock()

	r.emit("reconcile_start", "", "")

	r.checkHostFile()

	desired, err := r.DesiredState()
	if err != nil {
		return err
	}

	actual, err := r.ActualState()
	if err != nil {
		return err
	}

	// Runs before the repair steps below, because the measurement must report the host
	// that this tick found. A tick that repairs the forward path and then measures reports
	// no loss, and the operator never learns that a rule went away.
	r.probeReachability(desired, actual)

	// Runs before Apply, because the shutdown removes both chains and the policy of the
	// FORWARD chain is DROP. A namespace that holds no chain reaches no control server,
	// therefore tailscaled never reaches the Running state and the DNS refresh of the tick
	// finds no daemon to refresh. Issue #223 names the defect. The compiler reads the
	// configuration file and not the live host, so a namespace that this tick creates gets
	// its rules on this tick.
	r.applyAccess()
	r.ensureForwardPath(desired, actual)

	actions := r.Diff(desired, actual)
	if len(actions) > 0 {
		r.emit("reconcile_apply", "", fmt.Sprintf("%d actions", len(actions)))
		r.Apply(actions)
	}

	// Runs after Apply, because Apply clears the last error of a tailnet whose actions
	// all succeeded, and an unprotected namespace must keep its error.
	r.checkDNSProtection(desired)

	r.checkNamespaceForwarding(desired)

	if len(actions) == 0 {
		r.emit("reconcile_complete", "", "no changes needed")
		return nil
	}
	r.emit("reconcile_complete", "", fmt.Sprintf("applied %d actions", len(actions)))
	return nil
}

// accessTimeout bounds every command that one chain write runs. The specification states
// that the reconciler tick does not grow longer than 1 second because of the rule engine.
const accessTimeout = time.Second

// applyAccess compiles the local rule set of the configuration file and writes it into the
// chains that the daemon owns.
// applyAccess records access.written when it wrote the chains, and access.write_failed when
// a command failed. It records one event and it returns; a failed write leaves the previous
// chains in place, therefore the host keeps the rules of the last cycle.
func (r *Reconciler) applyAccess() {
	r.mu.Lock()
	writer := r.access
	r.mu.Unlock()
	if writer == nil {
		return
	}

	cfg, err := config.LoadConfig(r.configPath)
	if err != nil {
		r.emit("access.write_failed", "", fmt.Sprintf("load the configuration file: %v", err))
		return
	}

	devices := make(map[string]string, len(cfg.Tailnets))
	for _, tn := range cfg.Tailnets {
		hostVeth, _ := namespaces.VethNames(namespaces.GetNamespaceName(tn.ID))
		devices[tn.ID] = hostVeth
	}

	bindAddress := cfg.Resolver.BindAddress
	if bindAddress == "" {
		bindAddress = dns.DefaultBindAddress
	}

	set := access.RuleSet{Mode: cfg.AccessMode()}
	if cfg.Access != nil {
		set.Rules = r.declaredRules(cfg.Access.Rules, devices)
	}

	tail, err := access.TailForMode(set.EffectiveMode())
	if err != nil {
		r.emit("access.write_failed", "", err.Error())
		return
	}

	compiled, err := access.Compile(set, access.Topology{Devices: devices, DNSAddress: bindAddress}, tail)
	if err != nil {
		r.emit("access.write_failed", "", fmt.Sprintf("compile the local rule set: %v", err))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), accessTimeout)
	defer cancel()

	res, err := writer.Apply(ctx, compiled)
	if err != nil {
		r.emit("access.write_failed", "", err.Error())
		return
	}
	if res.Wrote {
		r.emit("access.written", "", fmt.Sprintf("mode %s, %d forward rules, %d out rules",
			set.EffectiveMode(), len(compiled.Forward), len(compiled.Out)))
	}
	r.reportJumps(res.Jumps)
	// The chain now holds the rules of the operator, therefore the version 0.9 rules are
	// no longer the path that accepts the traffic of a namespace.
	if set.EffectiveMode() == access.ModeEnforce {
		r.removeLegacyRules()
	}
}

// declaredRules returns the rules whose endpoints the configuration file still declares.
// devices holds one entry for each declared tailnet.
// declaredRules records access.rule_dropped for each rule that names a tailnet that the
// configuration file no longer declares, because the daemon does not refuse to start for
// a tailnet that the operator removed.
func (r *Reconciler) declaredRules(rules []access.Rule, devices map[string]string) []access.Rule {
	kept := make([]access.Rule, 0, len(rules))
	for _, rule := range rules {
		if !endpointDeclared(rule.From, devices) || !endpointDeclared(rule.To, devices) {
			r.emit("access.rule_dropped", "", fmt.Sprintf("%s: the configuration file declares no such tailnet", rule))
			continue
		}
		kept = append(kept, rule)
	}
	return kept
}

// endpointDeclared reports whether the endpoint is the host, the internet, or a tailnet
// that the configuration file declares.
func endpointDeclared(endpoint string, devices map[string]string) bool {
	if endpoint == access.Host || endpoint == access.Internet {
		return true
	}
	_, ok := devices[endpoint]
	return ok
}

// checkDNSProtection records the event dns.unprotected for each tailnet whose overlay
// mount on /etc failed, and places that tailnet in an error state. A tailnet that
// dns.allow_unprotected covers keeps its event and stays out of the error state, because
// the operator chose to run it without the overlay mount. checkDNSProtection records one
// event for one failure, so a repeated cycle adds no event. See issue #76.
func (r *Reconciler) checkDNSProtection(desired map[string]config.Tailnet) {
	for id := range desired {
		record, unprotected := r.dm.UnprotectedDNS(id)

		r.mu.Lock()
		if !unprotected {
			delete(r.unprotected, id)
			r.mu.Unlock()
			continue
		}
		reported, seen := r.unprotected[id]
		r.unprotected[id] = record.Reason
		if !record.Allowed {
			r.errorStates[id] = true
			r.lastErrors[id] = record.Reason
		}
		r.mu.Unlock()

		if !seen || reported != record.Reason {
			r.emit("dns.unprotected", id, record.Reason)
		}
	}
}

// checkHostFile compares the checksum of the host resolv.conf file with the recorded
// checksum and records dns.host_file_changed on a difference.
// The check reports the change. The daemon does not write the host file, because a daemon
// that rewrites the host file becomes the thing that it protects against. The event holds
// the first line of each version only, because a resolv.conf file can contain a search
// domain that names an internal host.
func (r *Reconciler) checkHostFile() {
	m := r.HostFileMonitor()
	if m == nil {
		return
	}

	changed, previous, current, err := m.Check()
	if err != nil {
		r.emit("dns.host_file_error", "", err.Error())
		return
	}
	if !changed {
		return
	}

	r.emit("dns.host_file_changed", "", fmt.Sprintf(
		"%s: previous first line %q, current first line %q",
		current.Path, describeHostFile(previous), describeHostFile(current)))
}

// describeHostFile returns the first line of the state, or "(missing)" when the file does
// not exist.
func describeHostFile(state dns.HostFileState) string {
	if state.Missing {
		return "(missing)"
	}
	return state.FirstLine
}

// Loop runs the reconciliation loop until the context is cancelled.
func (r *Reconciler) Loop(ctx context.Context) error {
	r.reapStaleRules()

	// Run once immediately
	if err := r.Reconcile(); err != nil {
		log.Printf("reconcile error: %v", err)
	}

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.emit("loop_stopped", "", "context cancelled")
			return nil
		case <-ticker.C:
			if err := r.Reconcile(); err != nil {
				log.Printf("reconcile error: %v", err)
			}
		}
	}
}

// reapStaleRules removes each host rule that names a veth device that is gone, and it
// records rules.reaped with the count. A rule that outlives its namespace lets traffic
// through a device name that a later namespace can take, and no cycle removes it, because
// the reconciler drives the tailnets that the configuration file declares.
func (r *Reconciler) reapStaleRules() {
	if r.reaper == nil {
		return
	}
	count, err := r.reaper.ReapStaleRules()
	if err != nil {
		r.emit("teardown.failed", "", fmt.Sprintf("reap the stale rules: %v", err))
	}
	r.emit("rules.reaped", "", fmt.Sprintf("%d rules", count))

}

// removeLegacyRules removes the two FORWARD rules that version 0.9 wrote for each host veth
// device.
// removeLegacyRules runs only after a write in the mode enforce, because those rules are the
// only path that accepts the traffic of a namespace until HYDRASCALE-FWD holds the rules of
// the operator. The mode observe writes a chain that accepts nothing and drops nothing,
// therefore a removal in that mode stops every namespace.
func (r *Reconciler) removeLegacyRules() {
	if r.reaper == nil {
		return
	}
	count, err := r.reaper.RemoveLegacyForwardRules()
	if err != nil {
		r.emit("teardown.failed", "", fmt.Sprintf("remove the version 0.9 forward rules: %v", err))
	}
	if count > 0 {
		r.emit("rules.reaped", "", fmt.Sprintf("%d version 0.9 forward rules", count))
	}
}

// ensureForwardPath writes the host rules of the forward path that a namespace lost, and it
// records access.path_repaired with the rules that it wrote.
// desired holds the tailnets that the configuration file declares, and actual holds the
// state of each namespace that the host carries.
// The setup path writes those rules only when it creates the veth pair, therefore an
// operator firewall that reloads breaks every namespace until the daemon creates the
// namespace again. Risk R7 names this. ensureForwardPath runs on a namespace that already
// exists, so the repair arrives on the next tick.
func (r *Reconciler) ensureForwardPath(desired map[string]config.Tailnet, actual map[string]*TailnetState) {
	if r.path == nil {
		return
	}
	for id := range desired {
		state, ok := actual[id]
		if !ok || !state.NsExists {
			continue
		}
		written, err := r.path.EnsureForwardPath(state.NsName, namespaces.VethIndex(state.NsName), r.infraSubnet)
		if err != nil {
			r.emit("access.write_failed", id, err.Error())
			continue
		}
		if len(written) == 0 {
			continue
		}
		r.emit("access.path_repaired", id, fmt.Sprintf("wrote %d rules: %s",
			len(written), strings.Join(written, ", ")))
	}
}

// checkNamespaceForwarding records access.namespace_forwarding for each namespace whose
// net.ipv4.ip_forward differs from the value that the configuration requires.
//
// A tailnet whose host_access is true requires the value 1. The namespace forwards a packet
// of the host to a peer, and it forwards the query that the unified resolver sends to
// 100.100.100.100. Both features failed while the value was 0. See issue #272.
//
// A tailnet that the operator keeps isolated requires the value 0.
//
// The finding SA-48 states that the containment of a packet inside the second namespace is
// a kernel default. The deny happens in HYDRASCALE-FWD on the host before the packet reaches
// the second namespace, therefore the containment does not depend on this value. The event
// reports a value that the configuration did not ask for.
func (r *Reconciler) checkNamespaceForwarding(desired map[string]config.Tailnet) {
	if r.reaper == nil {
		return
	}
	cfg, err := config.LoadConfig(r.configPath)
	if err != nil {
		return
	}
	for id := range desired {
		nsName := r.ns.GetName(id)
		value, err := r.reaper.NamespaceForwarding(nsName)
		if err != nil {
			continue
		}
		want := "0"
		if cfg.TailnetHostAccess(id) {
			want = "1"
		}
		if value != want {
			r.emit("access.namespace_forwarding", id,
				fmt.Sprintf("net.ipv4.ip_forward is %s in %s, and the configuration requires %s", value, nsName, want))
		}
	}
}

// ConfigPath returns the path to the config file used by this reconciler.
func (r *Reconciler) ConfigPath() string {
	return r.configPath
}

// GetTailscaleStatus fetches live Tailscale status for a single tailnet by running
// tailscale status --json inside its network namespace. Returns ErrTailnetNotFound
// (wrapped) if the tailnet ID is not in config; other errors on daemon failure.
func (r *Reconciler) GetTailscaleStatus(ctx context.Context, id string) (*daemon.TailscaleStatus, error) {
	desired, err := r.DesiredState()
	if err != nil {
		return nil, fmt.Errorf("failed to load desired state: %w", err)
	}
	if _, ok := desired[id]; !ok {
		return nil, fmt.Errorf("tailnet %s: %w", id, ErrTailnetNotFound)
	}
	nsName := r.ns.GetName(id)
	return r.dm.GetStatus(ctx, nsName, id)
}

// StopDaemon stops a running tailnet daemon without removing it from config.
// It also pauses the tailnet so the reconciler won't restart it.
func (r *Reconciler) StopDaemon(tailnetID string) error {
	nsName := r.ns.GetName(tailnetID)
	if err := r.dm.Stop(nsName, tailnetID); err != nil {
		return fmt.Errorf("failed to stop daemon for %s: %w", tailnetID, err)
	}
	r.mu.Lock()
	r.pausedStates[tailnetID] = true
	r.mu.Unlock()
	r.emit("daemon_stopped", tailnetID, "disconnected (paused)")
	return nil
}

// PausedStates returns a copy of the paused states map.
func (r *Reconciler) PausedStates() map[string]bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make(map[string]bool, len(r.pausedStates))
	for k, v := range r.pausedStates {
		cp[k] = v
	}
	return cp
}

// ResetError clears the error and paused states for a tailnet, allowing retries.
func (r *Reconciler) ResetError(tailnetID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.errorStates, tailnetID)
	delete(r.failureCounts, tailnetID)
	delete(r.pausedStates, tailnetID)
	// The next cycle reports an overlay mount that still fails.
	delete(r.unprotected, tailnetID)
}

// ResetAllErrors clears all error states. Called by one-shot apply.
func (r *Reconciler) ResetAllErrors() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errorStates = make(map[string]bool)
	r.failureCounts = make(map[string]int)
	r.unprotected = make(map[string]string)
}

// ErrorStates returns a copy of the current error states.
func (r *Reconciler) ErrorStates() map[string]bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make(map[string]bool, len(r.errorStates))
	for k, v := range r.errorStates {
		cp[k] = v
	}
	return cp
}

// Unprotected returns a copy of the overlay mount error of each unprotected tailnet.
// The map holds one entry for each tailnet that runs without the overlay mount on /etc.
// A tailnet that keeps the overlay mount holds no entry.
func (r *Reconciler) Unprotected() map[string]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make(map[string]string, len(r.unprotected))
	for k, v := range r.unprotected {
		cp[k] = v
	}
	return cp
}

// FailureCounts returns a copy of the current failure counts.
func (r *Reconciler) FailureCounts() map[string]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make(map[string]int, len(r.failureCounts))
	for k, v := range r.failureCounts {
		cp[k] = v
	}
	return cp
}

// Events returns a copy of the event log.
func (r *Reconciler) Events() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]Event, len(r.events))
	copy(cp, r.events)
	return cp
}

func (r *Reconciler) recordFailure(tailnetID string, errMsg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failureCounts[tailnetID]++
	r.lastErrors[tailnetID] = errMsg
	if r.failureCounts[tailnetID] >= MaxFailures {
		r.errorStates[tailnetID] = true
		log.Printf("tailnet %s entered error state after %d consecutive failures", tailnetID, MaxFailures)
	}
}

// LastErrors returns a copy of the last error message for each tailnet.
func (r *Reconciler) LastErrors() map[string]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make(map[string]string, len(r.lastErrors))
	for k, v := range r.lastErrors {
		cp[k] = v
	}
	return cp
}

func (r *Reconciler) clearFailure(tailnetID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failureCounts[tailnetID] = 0
	delete(r.lastErrors, tailnetID)
}

// SetEventLog opens a JSON event log file for append writing.
// The record holds the error text of a failed action, and that text names the control
// server, the namespace, and the addresses. The directory mode 0750 and the file mode 0640
// therefore agree with the two install paths, and no other local user reads the file.
func (r *Reconciler) SetEventLog(path string) error {
	dir := path
	if idx := strings.LastIndex(dir, "/"); idx >= 0 {
		dir = dir[:idx]
		if err := os.MkdirAll(dir, 0750); err != nil {
			return fmt.Errorf("failed to create event log directory: %w", err)
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0640)
	if err != nil {
		return fmt.Errorf("failed to open event log: %w", err)
	}
	r.eventLogPath = path
	r.eventFile = f
	return nil
}

// Close cleans up resources (event log file).
func (r *Reconciler) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.eventFile != nil {
		r.eventFile.Close()
		r.eventFile = nil
	}
}

// Shutdown gracefully stops all running tailnet daemons.
// Stops daemons concurrently with a 30-second overall deadline.
func (r *Reconciler) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	actual, err := r.ActualState()
	if err != nil {
		log.Printf("Warning: failed to get actual state during shutdown: %v", err)
		return err
	}

	var wg sync.WaitGroup
	for id, state := range actual {
		if !state.NsExists {
			continue
		}
		wg.Add(1)
		go func(tailnetID string, ts *TailnetState) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				log.Printf("Shutdown timeout for %s", tailnetID)
				return
			default:
			}
			if err := r.dm.Stop(ts.NsName, tailnetID); err != nil {
				log.Printf("Failed to stop daemon for %s: %v", tailnetID, err)
			} else {
				r.emit("daemon_stopped", tailnetID, "shutdown")
			}
		}(id, state)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		r.emit("shutdown_complete", "", "all daemons stopped")
	case <-ctx.Done():
		r.emit("shutdown_timeout", "", "30s deadline exceeded")
	}

	r.mu.Lock()
	writer := r.access
	r.mu.Unlock()
	if writer != nil {
		if err := writer.Teardown(ctx); err != nil {
			return r.reportTeardown("", []error{fmt.Errorf("remove the local rule chains: %w", err)})
		}
	}

	if r.ha != nil {
		if err := r.ha.TeardownAll(); err != nil {
			return r.reportTeardown("", []error{fmt.Errorf("remove the host access state: %w", err)})
		}
	}

	return nil
}

// RecordEvent adds one event to the event log.
// The control API calls it, because the console shows the event list and the daemon
// records an event for every mutating console request. See FR-console-10.
// The message reaches the log file and the API response, so it must hold no secret.
func (r *Reconciler) RecordEvent(eventType, tailnetID, message string) {
	r.emit(eventType, tailnetID, message)
}

func (r *Reconciler) emit(eventType, tailnetID, message string) {
	event := Event{
		Time:      time.Now(),
		Type:      eventType,
		TailnetID: tailnetID,
		Message:   message,
	}

	r.mu.Lock()
	if len(r.events) >= 1000 {
		// Drop oldest events
		r.events = r.events[1:]
	}
	r.events = append(r.events, event)
	f := r.eventFile
	r.mu.Unlock()

	// Log to stderr
	if tailnetID != "" {
		log.Printf("[%s] %s: %s", eventType, tailnetID, message)
	} else if message != "" {
		log.Printf("[%s] %s", eventType, message)
	} else {
		log.Printf("[%s]", eventType)
	}

	// Write JSON to event log file if configured
	if f != nil {
		jsonEvent, _ := json.Marshal(map[string]string{
			"time":    event.Time.Format(time.RFC3339),
			"type":    event.Type,
			"tailnet": event.TailnetID,
			"message": event.Message,
		})
		f.Write(append(jsonEvent, '\n'))
	}
}

// lockPathFor returns the lock file path for a given config path.
// Uses the state directory if writable, otherwise falls back to the config's directory.
func lockPathFor(configPath string) string {
	stateDir := daemon.DefaultStateDir
	// Check if we can write to the state directory
	testPath := filepath.Join(stateDir, ".lock-test")
	if f, err := os.Create(testPath); err == nil {
		f.Close()
		os.Remove(testPath)
		return filepath.Join(stateDir, ".hydrascale.lock")
	}
	// Fallback: use config file's directory (for tests or non-standard installs)
	dir := filepath.Dir(configPath)
	return filepath.Join(dir, ".hydrascale.lock")
}

// acquireLock acquires an exclusive file lock and returns an unlock function.
func acquireLock(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}

	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}
