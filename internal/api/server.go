package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"hydrascale/internal/access"
	"hydrascale/internal/config"
	"hydrascale/internal/daemon"
	"hydrascale/internal/dns"
	"hydrascale/internal/namespaces"
	"hydrascale/internal/reconciler"
	"hydrascale/internal/session"
)

// Server is an HTTP server listening on a Unix socket.
type Server struct {
	reconciler  *reconciler.Reconciler
	listener    net.Listener
	httpServer  *http.Server
	socketPath  string
	socketGroup string
	version     string
	forwarder   *dns.Forwarder

	// sessions reads the sessions that the host holds now. GET /api/access reports the
	// path of each one, and the console warns from that report. See FR-editor-28.
	sessions *session.Reader

	// mux holds the JSON routes. The control socket serves it, and the console listener
	// serves it under /api/, so one route set answers on both. See FR-console-4.
	mux *http.ServeMux

	// tailscaleBaseURL names the root of the Tailscale API. The daemon leaves it empty,
	// which selects policy.DefaultTailscaleBaseURL. A test sets it to a local server, so
	// that no test reaches a real control server.
	tailscaleBaseURL string

	// consoleServer serves the console listener. It is nil when console.enabled is false,
	// and consoleAddress is then the empty string.
	consoleServer  *http.Server
	consoleAddress string
}

// SetSocketGroup configures a unix group that may reach the control socket.
// Empty (default) keeps the socket root-only (0600). Must be called before Start.
func (s *Server) SetSocketGroup(group string) { s.socketGroup = group }

// SetVersion records the daemon version, surfaced to clients via /api/status.
func (s *Server) SetVersion(v string) { s.version = v }

// SetForwarder records the DNS forwarder, which GET /api/dns reads for the upstreams.
// A nil forwarder gives an empty list of upstreams.
func (s *Server) SetForwarder(f *dns.Forwarder) { s.forwarder = f }

// SetSessionReader records the reader of the live sessions, which GET /api/access reads.
// A test replaces the reader, so that the route runs no command on the host.
func (s *Server) SetSessionReader(r *session.Reader) { s.sessions = r }

// NewServer creates a new Server that will listen on socketPath.
func NewServer(socketPath string, r *reconciler.Reconciler) *Server {
	s := &Server{
		reconciler: r,
		socketPath: socketPath,
		sessions:   session.NewReader(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/events", s.handleEvents)
	mux.HandleFunc("/api/reconcile", s.handleReconcile)
	mux.HandleFunc("/api/tailnet/add", s.handleTailnetAdd)
	mux.HandleFunc("/api/tailnet/remove", s.handleTailnetRemove)
	mux.HandleFunc("/api/tailnet/connect", s.handleTailnetConnect)
	mux.HandleFunc("/api/tailnet/disconnect", s.handleTailnetDisconnect)
	mux.HandleFunc("/api/config/dns", s.handleConfigDNS)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/dns", s.handleDNS)
	mux.HandleFunc("/api/access", s.handleAccess)
	// Method-qualified pattern (Go 1.22+) — restricts to GET only and supports {id} wildcard.
	// Registered after the exact-match tailnet routes above, which take priority.
	mux.HandleFunc("GET /api/tailnet/{id}/detail", s.handleTailnetDetail)
	mux.HandleFunc("GET /api/tailnet/{id}/removal-plan", s.handleTailnetRemovalPlan)
	mux.HandleFunc("GET /api/policy", s.handlePolicyList)
	mux.HandleFunc("GET /api/policy/{id}", s.handlePolicyRead)
	mux.HandleFunc("PUT /api/policy/{id}", s.handlePolicyWrite)
	mux.HandleFunc("POST /api/policy/{id}/validate", s.handlePolicyValidate)
	mux.HandleFunc("PUT /api/policy/{id}/credentials", s.handlePolicyCredentials)

	s.mux = mux
	s.httpServer = &http.Server{Handler: mux}
	return s
}

// Start listens on the Unix socket and begins serving HTTP.
// If a stale socket file exists and connection is refused, it is removed first.
func (s *Server) Start() error {
	if _, err := os.Stat(s.socketPath); err == nil {
		// Socket file exists — check if it's stale.
		conn, err := net.Dial("unix", s.socketPath)
		if err != nil {
			// Connection refused: stale socket, remove it.
			if removeErr := os.Remove(s.socketPath); removeErr != nil {
				return fmt.Errorf("failed to remove stale socket %s: %w", s.socketPath, removeErr)
			}
			log.Printf("api: removed stale socket %s", s.socketPath)
		} else {
			conn.Close()
			return fmt.Errorf("socket %s is already in use", s.socketPath)
		}
	}

	// Set restrictive umask before creating the socket to prevent a TOCTOU
	// window where the socket is briefly accessible with default permissions
	oldUmask := syscall.Umask(0077)
	ln, err := net.Listen("unix", s.socketPath)
	syscall.Umask(oldUmask)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.socketPath, err)
	}
	if err := os.Chmod(s.socketPath, 0600); err != nil {
		ln.Close()
		return fmt.Errorf("failed to set socket permissions: %w", err)
	}
	if s.socketGroup != "" {
		if err := applySocketGroup(s.socketPath, s.socketGroup); err != nil {
			ln.Close()
			return fmt.Errorf("failed to apply socket group %q: %w", s.socketGroup, err)
		}
		info, err := os.Stat(s.socketPath)
		if err != nil {
			ln.Close()
			return fmt.Errorf("failed to read the mode of socket %s: %w", s.socketPath, err)
		}
		log.Print(socketGroupLog(s.socketPath, s.socketGroup, info.Mode()))
	}
	s.listener = ln

	go func() {
		if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("api server error: %v", err)
		}
	}()

	log.Printf("api: listening on %s", s.socketPath)
	return nil
}

// Shutdown gracefully stops the control socket server and the console server.
// Shutdown stops both servers even when the first stop fails, and it returns every error
// together.
func (s *Server) Shutdown(ctx context.Context) error {
	var errs []error
	if s.consoleServer != nil {
		if err := s.consoleServer.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("console shutdown error: %w", err))
		}
	}
	if s.httpServer != nil {
		if err := s.httpServer.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("api shutdown error: %w", err))
		}
		if s.socketPath != "" {
			os.Remove(s.socketPath)
		}
	}
	return errors.Join(errs...)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	desired, err := s.reconciler.DesiredState()
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get desired state: %v", err), http.StatusInternalServerError)
		return
	}

	actual, err := s.reconciler.ActualState()
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get actual state: %v", err), http.StatusInternalServerError)
		return
	}

	resp := StatusResponse{
		Desired:       desired,
		Actual:        actual,
		ErrorStates:   s.reconciler.ErrorStates(),
		PausedStates:  s.reconciler.PausedStates(),
		FailureCounts: s.reconciler.FailureCounts(),
		LastErrors:    s.reconciler.LastErrors(),
		ServerVersion: s.version,

		ConfigPath:     s.reconciler.ConfigPath(),
		SocketPath:     s.socketPath,
		ConsoleAddress: s.consoleAddress,
	}

	// The access field carries the mode and the count of rules, which the terminal
	// interface shows. A configuration file that the daemon cannot read leaves the field
	// absent rather than reporting a mode that the daemon does not apply.
	if cfg, err := config.LoadConfig(s.reconciler.ConfigPath()); err == nil {
		status := AccessStatus{
			Mode:         cfg.AccessMode(),
			JumpPosition: s.reconciler.AccessJumpPosition(access.ParentForward),
		}
		if cfg.Access != nil {
			status.Rules = len(cfg.Access.Rules)
		}
		resp.Access = &status
	} else {
		log.Printf("api: read the access block for the status response: %v", err)
	}

	writeJSON(w, resp)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp := EventsResponse{Events: s.reconciler.Events()}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleReconcile serves POST /api/reconcile.
// The route reads no request body, so it validates the method only.
func (s *Server) handleReconcile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	err := s.reconciler.Reconcile()
	s.writeReconcileResponse(w, err)
}

func (s *Server) writeReconcileResponse(w http.ResponseWriter, err error) {
	resp := ReconcileResponse{OK: err == nil}
	if err != nil {
		resp.Message = err.Error()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// socketGroupLog returns the start log line for a group-accessible control socket.
// The line names the socket path, the socket mode, and the group. It also states that a
// member of the group holds root access on the host.
func socketGroupLog(socketPath, group string, mode os.FileMode) string {
	return fmt.Sprintf("api: socket %s has mode %04o and group %q; a member of that group sends a command to the daemon, which runs as root, so membership is equivalent to root access on this host",
		socketPath, mode.Perm(), group)
}

// applySocketGroup makes the control socket reachable by members of group:
// the parent dir gets root:<group> 0750 (group can traverse to the socket) and
// the socket itself root:<group> 0660 (group can connect). The daemon still
// owns both as root; only the named group is additionally granted access.
func applySocketGroup(socketPath, group string) error {
	g, err := user.LookupGroup(group)
	if err != nil {
		return fmt.Errorf("lookup group %q: %w", group, err)
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return fmt.Errorf("parse gid for %q: %w", group, err)
	}
	// A member of the socket group controls a root daemon, so the root group adds no
	// account and it hides that fact from the operator.
	if gid == 0 {
		return fmt.Errorf("group %q has group id 0: name a group other than the root group", group)
	}
	dir := filepath.Dir(socketPath)
	if err := os.Chown(dir, 0, gid); err != nil {
		return fmt.Errorf("chown %s: %w", dir, err)
	}
	if err := os.Chmod(dir, 0750); err != nil {
		return fmt.Errorf("chmod %s: %w", dir, err)
	}
	if err := os.Chown(socketPath, 0, gid); err != nil {
		return fmt.Errorf("chown %s: %w", socketPath, err)
	}
	if err := os.Chmod(socketPath, 0660); err != nil {
		return fmt.Errorf("chmod %s: %w", socketPath, err)
	}
	return nil
}

// writeRefusal refuses the request with HTTP 400 and the body {"error": "<message>"}.
// writeRefusal writes the reason into the log, so message must hold no secret.
func writeRefusal(w http.ResponseWriter, message string) {
	log.Printf("api: refused a request: %s", message)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	if err := json.NewEncoder(w).Encode(ErrorResponse{Error: message}); err != nil {
		log.Printf("api: encode refusal: %v", err)
	}
}

// decodeBody reads a JSON request body of 1 MiB or less into dst.
// decodeBody refuses the request and returns false when the body does not parse.
func decodeBody(w http.ResponseWriter, r *http.Request, dst interface{}) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeRefusal(w, fmt.Sprintf("invalid request body: %v", err))
		return false
	}
	return true
}

// validateTailnetID returns an error when id is not a tailnet identifier that the daemon
// accepts. config.IsValidID holds the one rule, and the configuration loader applies the
// same rule, so an identifier that a route writes always loads again.
func validateTailnetID(id string) error {
	if id == "" {
		return errors.New("id is required")
	}
	if !config.IsValidID(id) {
		return fmt.Errorf("invalid id %q: an id starts with a letter or a digit, holds the characters a-z A-Z 0-9 . _ - only, and holds 63 characters or fewer", id)
	}
	return nil
}

// hasTailnet reports whether the configuration holds a tailnet with this identifier.
func hasTailnet(cfg *config.Config, id string) bool {
	for _, tn := range cfg.Tailnets {
		if tn.ID == id {
			return true
		}
	}
	return false
}

// handleTailnetAdd serves POST /api/tailnet/add.
// The route validates the whole body before it writes the configuration file.
// The route writes the auth key into /etc/hydrascale/config.yaml, which the secrets file
// of Epic 3 replaces. See SA-36 in docs/security-audit.md.
func (s *Server) handleTailnetAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req TailnetRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := validateTailnetID(req.ID); err != nil {
		writeRefusal(w, err.Error())
		return
	}
	if err := config.ValidateControlURL(req.ControlURL); err != nil {
		writeRefusal(w, err.Error())
		return
	}

	cfgPath := s.reconciler.ConfigPath()
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		s.writeReconcileResponse(w, fmt.Errorf("failed to load config: %w", err))
		return
	}

	if hasTailnet(cfg, req.ID) {
		writeRefusal(w, fmt.Sprintf("tailnet %s already exists", req.ID))
		return
	}

	cfg.Tailnets = append(cfg.Tailnets, config.Tailnet{
		ID:         req.ID,
		AuthKey:    req.AuthKey,
		ExitNode:   req.ExitNode,
		ControlURL: req.ControlURL,
		HostAccess: req.HostAccess,
	})

	if err := config.SaveConfig(cfgPath, cfg); err != nil {
		s.writeReconcileResponse(w, fmt.Errorf("failed to save config: %w", err))
		return
	}

	s.writeReconcileResponse(w, s.reconciler.Reconcile())
}

func (s *Server) handleTailnetRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req TailnetRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := validateTailnetID(req.ID); err != nil {
		writeRefusal(w, err.Error())
		return
	}

	cfgPath := s.reconciler.ConfigPath()
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		s.writeReconcileResponse(w, fmt.Errorf("failed to load config: %w", err))
		return
	}

	if !hasTailnet(cfg, req.ID) {
		writeRefusal(w, fmt.Sprintf("tailnet %s not found", req.ID))
		return
	}
	for i, tn := range cfg.Tailnets {
		if tn.ID == req.ID {
			cfg.Tailnets = append(cfg.Tailnets[:i], cfg.Tailnets[i+1:]...)
			break
		}
	}

	if err := config.SaveConfig(cfgPath, cfg); err != nil {
		s.writeReconcileResponse(w, fmt.Errorf("failed to save config: %w", err))
		return
	}

	s.writeReconcileResponse(w, s.reconciler.Reconcile())
}

func (s *Server) handleTailnetConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req TailnetRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := validateTailnetID(req.ID); err != nil {
		writeRefusal(w, err.Error())
		return
	}

	cfg, err := config.LoadConfig(s.reconciler.ConfigPath())
	if err != nil {
		s.writeReconcileResponse(w, fmt.Errorf("failed to load config: %w", err))
		return
	}
	if !hasTailnet(cfg, req.ID) {
		writeRefusal(w, fmt.Sprintf("tailnet %s not found", req.ID))
		return
	}

	s.reconciler.ResetError(req.ID)
	s.writeReconcileResponse(w, s.reconciler.Reconcile())
}

func (s *Server) handleTailnetDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req TailnetRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := validateTailnetID(req.ID); err != nil {
		writeRefusal(w, err.Error())
		return
	}
	// StopDaemon joins the identifier onto the state directory and it removes a file
	// under the result, so the route confirms the containment before it acts. See SA-2.
	if _, ok := config.SafeStateDir(daemon.DefaultStateDir, req.ID); !ok {
		writeRefusal(w, fmt.Sprintf("id %q leaves the state directory %s", req.ID, daemon.DefaultStateDir))
		return
	}

	cfg, err := config.LoadConfig(s.reconciler.ConfigPath())
	if err != nil {
		s.writeReconcileResponse(w, fmt.Errorf("failed to load config: %w", err))
		return
	}
	if !hasTailnet(cfg, req.ID) {
		writeRefusal(w, fmt.Sprintf("tailnet %s not found", req.ID))
		return
	}

	s.writeReconcileResponse(w, s.reconciler.StopDaemon(req.ID))
}

func (s *Server) handleConfigDNS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req DNSConfigRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := config.ValidateResolverMode(req.Mode); err != nil {
		writeRefusal(w, err.Error())
		return
	}
	if err := config.ValidateBindAddress(req.BindAddress); err != nil {
		writeRefusal(w, err.Error())
		return
	}

	cfgPath := s.reconciler.ConfigPath()
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		s.writeReconcileResponse(w, fmt.Errorf("failed to load config: %w", err))
		return
	}

	if req.Mode != "" {
		cfg.Resolver.Mode = req.Mode
	}
	if req.BindAddress != "" {
		cfg.Resolver.BindAddress = req.BindAddress
	}

	if err := config.SaveConfig(cfgPath, cfg); err != nil {
		s.writeReconcileResponse(w, fmt.Errorf("failed to save config: %w", err))
		return
	}

	s.writeReconcileResponse(w, s.reconciler.Reconcile())
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cfgPath := s.reconciler.ConfigPath()
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	redacted := RedactedConfig{
		Version:  cfg.Version,
		Resolver: cfg.Resolver,
		Tailnets: make([]RedactedTailnet, len(cfg.Tailnets)),
	}
	for i, tn := range cfg.Tailnets {
		rt := RedactedTailnet{
			ID:         tn.ID,
			ExitNode:   tn.ExitNode,
			HostAccess: tn.HostAccess,
		}
		if tn.AuthKey != "" {
			rt.AuthKey = "***"
		}
		redacted.Tailnets[i] = rt
	}

	resp := ConfigResponse{Config: redacted}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleDNS serves GET /api/dns, which holds the resolver settings, the checksum of the
// host resolv.conf file, and the DNS protection state of each running namespace.
// The route is read-only, so it rejects a method other than GET with HTTP 405. It returns
// HTTP 500 when it cannot read the configuration file or list the namespaces.
func (s *Server) handleDNS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cfg, err := config.LoadConfig(s.reconciler.ConfigPath())
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	actual, err := s.reconciler.ActualState()
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get actual state: %v", err), http.StatusInternalServerError)
		return
	}

	bindAddress := cfg.Resolver.BindAddress
	if bindAddress == "" {
		bindAddress = dns.DefaultBindAddress
	}

	upstreams := []string{}
	if s.forwarder != nil {
		upstreams = s.forwarder.Upstreams()
	}

	monitor := s.reconciler.HostFileMonitor()
	hostFile, changedAt := monitor.State()
	changedAtText := ""
	if !changedAt.IsZero() {
		changedAtText = changedAt.UTC().Format(time.RFC3339)
	}

	unprotected := s.reconciler.Unprotected()
	ids := make([]string, 0, len(actual))
	for id := range actual {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	namespaceStates := make([]DNSNamespaceState, 0, len(ids))
	for _, id := range ids {
		reason, found := unprotected[id]
		namespaceStates = append(namespaceStates, DNSNamespaceState{
			ID:        id,
			Protected: !found,
			Error:     reason,
		})
	}

	resp := DNSResponse{
		BindAddress:         bindAddress,
		Mode:                cfg.Resolver.Mode,
		Upstreams:           upstreams,
		AllowUnprotected:    cfg.DNS.AllowUnprotected,
		HostResolvPath:      monitor.Path(),
		HostResolvSHA256:    hostFile.Checksum,
		HostResolvChangedAt: changedAtText,
		Namespaces:          namespaceStates,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleTailnetDetail serves GET /api/tailnet/{id}/detail.
// It fetches live TailscaleStatus from inside the tailnet's network namespace.
// Returns HTTP 404 for unknown tailnet IDs.
// Returns HTTP 200 with a non-empty Error field for daemon failures (unreachable,
// starting up) so the TUI can render the error inline without treating it as a
// transport error.
func (s *Server) handleTailnetDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing tailnet id", http.StatusBadRequest)
		return
	}

	var resp TailnetDetailResponse

	status, err := s.reconciler.GetTailscaleStatus(r.Context(), id)
	if err != nil {
		// Distinguish "not found" from other errors for proper HTTP status.
		if errors.Is(err, reconciler.ErrTailnetNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		resp.Error = err.Error()
		w.Header().Set("Content-Type", "application/json")
		if encErr := json.NewEncoder(w).Encode(resp); encErr != nil {
			log.Printf("handleTailnetDetail: encode error response: %v", encErr)
		}
		return
	}

	if status == nil {
		// Daemon exists but returned no status — still starting up.
		resp.Error = "daemon starting, status unavailable"
		w.Header().Set("Content-Type", "application/json")
		if encErr := json.NewEncoder(w).Encode(resp); encErr != nil {
			log.Printf("handleTailnetDetail: encode nil-status response: %v", encErr)
		}
		return
	}

	// FetchedAt stamps when the live data was actually obtained (after the subprocess).
	resp.FetchedAt = time.Now()
	resp.TailscaleIPs = status.Self.TailscaleIPs
	resp.MagicDNSName = strings.TrimSuffix(status.Self.DNSName, ".")
	resp.MagicDNSSuffix = status.MagicDNSSuffix
	resp.BackendState = status.BackendState
	resp.LoginURL = status.AuthURL
	resp.PeerCount = len(status.Peer)
	resp.Peers = make([]PeerInfo, 0, len(status.Peer))
	for _, peer := range status.Peer {
		if peer.Online {
			resp.OnlinePeers++
		}
		resp.Peers = append(resp.Peers, PeerInfo{
			HostName:     peer.HostName,
			DNSName:      strings.TrimSuffix(peer.DNSName, "."),
			OS:           peer.OS,
			TailscaleIPs: peer.TailscaleIPs,
			AllowedIPs:   peer.AllowedIPs,
			Online:       peer.Online,
			LastSeen:     peer.LastSeen,
		})
	}
	// Deterministic order: tailscale's Peer map is keyed by node key.
	sort.Slice(resp.Peers, func(i, j int) bool {
		return resp.Peers[i].HostName < resp.Peers[j].HostName
	})

	w.Header().Set("Content-Type", "application/json")
	if encErr := json.NewEncoder(w).Encode(resp); encErr != nil {
		log.Printf("handleTailnetDetail: encode success response: %v", encErr)
	}
}

// handleTailnetRemovalPlan serves GET /api/tailnet/{id}/removal-plan.
//
// The route states the namespace, the veth device, the state directory, the count of
// iptables rules, and every command that the removal runs. The console dialog of
// FR-console-29 names them before the operator confirms, and it repeats no rule of the
// daemon. The route reads state and it runs no command.
//
// The route returns HTTP 404 for a tailnet that the configuration file does not declare,
// and HTTP 400 for an identifier that the daemon refuses.
func (s *Server) handleTailnetRemovalPlan(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := validateTailnetID(id); err != nil {
		writeRefusal(w, err.Error())
		return
	}

	cfg, err := config.LoadConfig(s.reconciler.ConfigPath())
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to load config: %v", err), http.StatusInternalServerError)
		return
	}
	if !hasTailnet(cfg, id) {
		http.Error(w, fmt.Sprintf("tailnet %s not found", id), http.StatusNotFound)
		return
	}

	plan, err := namespaces.PlanRemoval(id, s.reconciler.InfraSubnet(), daemon.DefaultStateDir)
	if err != nil {
		writeRefusal(w, err.Error())
		return
	}

	writeJSON(w, TailnetRemovalPlanResponse{
		ID:        id,
		Namespace: plan.Namespace,
		HostVeth:  plan.HostVeth,
		StateDir:  plan.StateDir,
		RuleCount: plan.RuleCount,
		Commands:  plan.Commands,
	})
}
