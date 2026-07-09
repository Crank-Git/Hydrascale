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

	"hydrascale/internal/config"
	"hydrascale/internal/daemon"
	"hydrascale/internal/hostaccess"
	"hydrascale/internal/namespaces"
	"hydrascale/internal/routing"
)

// ActionType represents the type of reconciliation action.
type ActionType string

const (
	ActionCreateNS       ActionType = "create_namespace"
	ActionDeleteNS       ActionType = "delete_namespace"
	ActionStartDaemon    ActionType = "start_daemon"
	ActionStopDaemon     ActionType = "stop_daemon"
	ActionSyncRoutes     ActionType = "sync_routes"
	ActionSyncHostAccess ActionType = "sync_host_access"
	ActionAuthDaemon     ActionType = "auth_daemon"
	ActionRefreshDNS     ActionType = "refresh_dns"
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
	events        []Event

	eventLogPath string
	eventFile    *os.File
}

// New creates a new Reconciler with the given dependencies.
func New(configPath string, ns namespaces.Manager, dm daemon.Manager, rt routing.Manager, interval time.Duration, ha *hostaccess.Manager, infraSubnet string) *Reconciler {
	if infraSubnet == "" {
		infraSubnet = "10.200.0.0/16"
	}
	return &Reconciler{
		configPath:    configPath,
		ns:            ns,
		dm:            dm,
		rt:            rt,
		ha:            ha,
		interval:      interval,
		infraSubnet:   infraSubnet,
		failureCounts: make(map[string]int),
		errorStates:   make(map[string]bool),
		pausedStates:  make(map[string]bool),
		lastErrors:    make(map[string]string),
	}
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
			ID:       tailnetID,
			NsName:   nsName,
			NsExists: true,
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
			actions = append(actions, Action{Type: ActionRefreshDNS, TailnetID: id, NsName: ns})
		} else {
			// Daemon healthy, sync routes
			actions = append(actions, Action{Type: ActionSyncRoutes, TailnetID: id, NsName: ns})
			// Sync host access if enabled for this tailnet
			if cfgErr == nil && cfg.TailnetHostAccess(id) {
				actions = append(actions, Action{Type: ActionSyncHostAccess, TailnetID: id, NsName: ns})
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
			}
		}
		return nil
	case ActionDeleteNS:
		if err := r.ns.Delete(action.NsName, r.infraSubnet); err != nil {
			return err
		}
		// Remove the tailnet's state dir (tailscaled state/socket + the #28
		// overlay upper/work dirs) so `remove` leaves nothing behind — otherwise
		// stale per-tailnet dirs accumulate under /var/lib/hydrascale/state.
		// Best-effort; a failure here doesn't fail the teardown. See issue #32.
		//
		// The id is derived from a live namespace name (not the IsValidID-
		// validated config), so validate the path before this root-level
		// RemoveAll — never let a crafted id (e.g. "..") traverse out.
		if stateDir, ok := safeStateDir(action.TailnetID); ok {
			if err := os.RemoveAll(stateDir); err != nil {
				log.Printf("reconciler: failed to remove state dir %s: %v", stateDir, err)
			}
		}
		return nil
	case ActionStartDaemon:
		// Belt-and-braces: ensure the namespace's resolv.conf bind-mount
		// override exists before tailscaled launches. ActionCreateNS already
		// writes it on first creation, but on a daemon restart the namespace
		// may already exist and we'd otherwise miss this. Idempotent. See #22.
		if err := namespaces.WriteNamespaceResolvConf(action.NsName); err != nil {
			log.Printf("namespace: failed to write resolv.conf for %s: %v", action.NsName, err)
		}
		return r.dm.Start(action.TailnetID, action.NsName)
	case ActionStopDaemon:
		return r.dm.Stop(action.NsName, action.TailnetID)
	case ActionAuthDaemon:
		return r.dm.AuthorizeDaemon(action.TailnetID, action.NsName, action.AuthKey, action.ControlURL)
	case ActionRefreshDNS:
		return r.dm.RefreshDNSConfig(action.TailnetID, action.NsName)
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
		namespaces.SetupHostAccess(nsName, index, r.infraSubnet)
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

	desired, err := r.DesiredState()
	if err != nil {
		return err
	}

	actual, err := r.ActualState()
	if err != nil {
		return err
	}

	actions := r.Diff(desired, actual)
	if len(actions) == 0 {
		r.emit("reconcile_complete", "", "no changes needed")
		return nil
	}

	r.emit("reconcile_apply", "", fmt.Sprintf("%d actions", len(actions)))
	r.Apply(actions)
	r.emit("reconcile_complete", "", fmt.Sprintf("applied %d actions", len(actions)))
	return nil
}

// Loop runs the reconciliation loop until the context is cancelled.
func (r *Reconciler) Loop(ctx context.Context) error {
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
}

// ResetAllErrors clears all error states. Called by one-shot apply.
func (r *Reconciler) ResetAllErrors() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errorStates = make(map[string]bool)
	r.failureCounts = make(map[string]int)
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
func (r *Reconciler) SetEventLog(path string) error {
	dir := path
	if idx := strings.LastIndex(dir, "/"); idx >= 0 {
		dir = dir[:idx]
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create event log directory: %w", err)
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
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

	if r.ha != nil {
		r.ha.TeardownAll()
	}

	return nil
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
