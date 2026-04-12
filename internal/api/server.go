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
	"syscall"
	"time"

	"hydrascale/internal/config"
	"hydrascale/internal/reconciler"
)

// Server is an HTTP server listening on a Unix socket.
type Server struct {
	reconciler *reconciler.Reconciler
	listener   net.Listener
	httpServer *http.Server
	socketPath string
}

// NewServer creates a new Server that will listen on socketPath.
func NewServer(socketPath string, r *reconciler.Reconciler) *Server {
	s := &Server{
		reconciler: r,
		socketPath: socketPath,
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
	// Method-qualified pattern (Go 1.22+) — restricts to GET only and supports {id} wildcard.
	// Registered after the exact-match tailnet routes above, which take priority.
	mux.HandleFunc("GET /api/tailnet/{id}/detail", s.handleTailnetDetail)

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
	s.listener = ln

	go func() {
		if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("api server error: %v", err)
		}
	}()

	log.Printf("api: listening on %s", s.socketPath)
	return nil
}

// Shutdown gracefully stops the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("api shutdown error: %w", err)
	}
	if s.socketPath != "" {
		os.Remove(s.socketPath)
	}
	return nil
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
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
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

func (s *Server) handleTailnetAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req TailnetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}
	if req.ID == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	cfgPath := s.reconciler.ConfigPath()
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		s.writeReconcileResponse(w, fmt.Errorf("failed to load config: %w", err))
		return
	}

	for _, tn := range cfg.Tailnets {
		if tn.ID == req.ID {
			s.writeReconcileResponse(w, fmt.Errorf("tailnet %s already exists", req.ID))
			return
		}
	}

	cfg.Tailnets = append(cfg.Tailnets, config.Tailnet{
		ID:       req.ID,
		AuthKey:  req.AuthKey,
		ExitNode: req.ExitNode,
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
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req TailnetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}
	if req.ID == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	cfgPath := s.reconciler.ConfigPath()
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		s.writeReconcileResponse(w, fmt.Errorf("failed to load config: %w", err))
		return
	}

	found := false
	for i, tn := range cfg.Tailnets {
		if tn.ID == req.ID {
			cfg.Tailnets = append(cfg.Tailnets[:i], cfg.Tailnets[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		s.writeReconcileResponse(w, fmt.Errorf("tailnet %s not found", req.ID))
		return
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
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req TailnetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}
	if req.ID == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
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
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req TailnetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}
	if req.ID == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	s.writeReconcileResponse(w, s.reconciler.StopDaemon(req.ID))
}

func (s *Server) handleConfigDNS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req DNSConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if req.BindAddress != "" {
		if err := config.ValidateBindAddress(req.BindAddress); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
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
			ID:       tn.ID,
			ExitNode: tn.ExitNode,
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
	resp.PeerCount = len(status.Peer)
	for _, peer := range status.Peer {
		if peer.Online {
			resp.OnlinePeers++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if encErr := json.NewEncoder(w).Encode(resp); encErr != nil {
		log.Printf("handleTailnetDetail: encode success response: %v", encErr)
	}
}

