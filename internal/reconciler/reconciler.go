// Package reconciler implements the declarative reconciliation loop for Hydrascale.
// It compares desired state (from config) with actual state (from the system)
// and executes actions to converge them.
package reconciler

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	"hydrascale/internal/config"
	"hydrascale/internal/daemon"
	"hydrascale/internal/namespaces"
	"hydrascale/internal/routing"
)

// ActionType represents the type of reconciliation action.
type ActionType string

const (
	ActionCreateNS    ActionType = "create_namespace"
	ActionDeleteNS    ActionType = "delete_namespace"
	ActionStartDaemon ActionType = "start_daemon"
	ActionStopDaemon  ActionType = "stop_daemon"
	ActionSyncRoutes  ActionType = "sync_routes"
)

// MaxFailures is the number of consecutive failures before a tailnet enters error state.
const MaxFailures = 3

// Action represents a single reconciliation action to be executed.
type Action struct {
	Type      ActionType
	TailnetID string
	NsName    string
}

func (a Action) String() string {
	return fmt.Sprintf("%s %s (%s)", a.Type, a.TailnetID, a.NsName)
}

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
	configPath string
	ns         namespaces.Manager
	dm         daemon.Manager
	rt         routing.Manager
	interval   time.Duration

	mu            sync.Mutex
	failureCounts map[string]int  // tailnetID -> consecutive failure count
	errorStates   map[string]bool // tailnetID -> true if in error state
	events        []Event
}

// New creates a new Reconciler with the given dependencies.
func New(configPath string, ns namespaces.Manager, dm daemon.Manager, rt routing.Manager, interval time.Duration) *Reconciler {
	return &Reconciler{
		configPath:    configPath,
		ns:            ns,
		dm:            dm,
		rt:            rt,
		interval:      interval,
		failureCounts: make(map[string]int),
		errorStates:   make(map[string]bool),
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

	// Create/start tailnets that should exist but don't
	for id := range desired {
		r.mu.Lock()
		inError := r.errorStates[id]
		r.mu.Unlock()

		if inError {
			continue // skip tailnets in error state
		}

		ns := r.ns.GetName(id)
		state, exists := actual[id]

		if !exists || !state.NsExists {
			actions = append(actions, Action{Type: ActionCreateNS, TailnetID: id, NsName: ns})
			actions = append(actions, Action{Type: ActionStartDaemon, TailnetID: id, NsName: ns})
			actions = append(actions, Action{Type: ActionSyncRoutes, TailnetID: id, NsName: ns})
		} else if !state.DaemonHealthy {
			actions = append(actions, Action{Type: ActionStartDaemon, TailnetID: id, NsName: ns})
		} else {
			// Daemon healthy, sync routes
			actions = append(actions, Action{Type: ActionSyncRoutes, TailnetID: id, NsName: ns})
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
// Failed actions increment the per-tailnet failure counter.
func (r *Reconciler) Apply(actions []Action) {
	for _, action := range actions {
		err := r.executeAction(action)
		if err != nil {
			r.emit("action_failed", action.TailnetID, fmt.Sprintf("%s: %v", action.Type, err))
			r.recordFailure(action.TailnetID)
		} else {
			r.emit("action_ok", action.TailnetID, string(action.Type))
			r.clearFailure(action.TailnetID)
		}
	}
}

func (r *Reconciler) executeAction(action Action) error {
	switch action.Type {
	case ActionCreateNS:
		return r.ns.Create(action.TailnetID)
	case ActionDeleteNS:
		return r.ns.Delete(action.NsName)
	case ActionStartDaemon:
		return r.dm.Start(action.TailnetID, action.NsName)
	case ActionStopDaemon:
		return r.dm.Stop(action.NsName, action.TailnetID)
	case ActionSyncRoutes:
		socketPath := r.dm.GetSocketPath(action.TailnetID)
		routes, err := r.rt.PollStatus(action.NsName, socketPath)
		if err != nil {
			return err
		}
		return r.rt.SyncRoutes(action.NsName, routes)
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

// ResetError clears the error state for a tailnet, allowing retries.
func (r *Reconciler) ResetError(tailnetID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.errorStates, tailnetID)
	delete(r.failureCounts, tailnetID)
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

func (r *Reconciler) recordFailure(tailnetID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failureCounts[tailnetID]++
	if r.failureCounts[tailnetID] >= MaxFailures {
		r.errorStates[tailnetID] = true
		log.Printf("tailnet %s entered error state after %d consecutive failures", tailnetID, MaxFailures)
	}
}

func (r *Reconciler) clearFailure(tailnetID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failureCounts[tailnetID] = 0
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
	r.mu.Unlock()

	// Log to stderr
	if tailnetID != "" {
		log.Printf("[%s] %s: %s", eventType, tailnetID, message)
	} else if message != "" {
		log.Printf("[%s] %s", eventType, message)
	} else {
		log.Printf("[%s]", eventType)
	}
}

// lockPathFor returns the lock file path for a given config path.
func lockPathFor(configPath string) string {
	dir := configPath
	if idx := strings.LastIndex(dir, "/"); idx >= 0 {
		dir = dir[:idx]
	}
	return dir + "/.hydrascale.lock"
}

// acquireLock acquires an exclusive file lock and returns an unlock function.
func acquireLock(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
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
