package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"hydrascale/internal/access"
	"hydrascale/internal/config"
	"hydrascale/internal/dns"
	"hydrascale/internal/namespaces"
)

// kindTailnet names a node that is one tailnet of the configuration.
const kindTailnet = "tailnet"

// handleAccess serves GET /api/access and PUT /api/access.
// GET returns the mode, the rule set, and the node list.
// PUT replaces the whole rule set. It validates every rule before it writes any rule, and
// the query parameter dry_run=true returns the compiled result and writes nothing.
func (s *Server) handleAccess(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.getAccess(w, r)
	case http.MethodPut:
		s.putAccess(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// getAccess serves GET /api/access.
// getAccess returns HTTP 500 when it cannot read the configuration file, and when it
// cannot build the node list.
func (s *Server) getAccess(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig(s.reconciler.ConfigPath())
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	set := access.RuleSet{}
	if cfg.Access != nil {
		set = *cfg.Access
	}

	resp, err := s.buildAccessResponse(r.Context(), cfg, set)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to build the access response: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
}

// putAccess serves PUT /api/access.
// putAccess validates the mode and every rule of the request body before it writes the
// configuration file. A failure returns HTTP 400 and the body {"error": "<message>"}, and
// the configuration file keeps the rule set that it held.
// putAccess returns HTTP 500 when the configuration file write fails, and when the
// reconcile that applies the new rule set fails.
func (s *Server) putAccess(w http.ResponseWriter, r *http.Request) {
	dryRun, ok := dryRunParam(w, r)
	if !ok {
		return
	}

	var req AccessRequest
	if !decodeBody(w, r, &req) {
		return
	}

	cfg, err := config.LoadConfig(s.reconciler.ConfigPath())
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	set := access.RuleSet{Mode: req.Mode, Rules: req.Rules}

	// Validate reports every failure together, therefore the operator sees each rule that
	// the daemon rejects rather than the first one only.
	ids := make([]string, 0, len(cfg.Tailnets))
	for _, tn := range cfg.Tailnets {
		ids = append(ids, tn.ID)
	}
	if err := set.Validate(ids); err != nil {
		writeRefusal(w, err.Error())
		return
	}

	// The compiler holds the rest of the refusals, such as a tailnet that the topology
	// names no device for. The route runs it before the write, so that a rule set that
	// the daemon cannot apply never reaches the configuration file.
	tail, err := access.TailForMode(set.Mode)
	if err != nil {
		writeRefusal(w, err.Error())
		return
	}
	if _, err := access.Compile(set, topology(cfg), tail); err != nil {
		writeRefusal(w, err.Error())
		return
	}

	resp, err := s.buildAccessResponse(r.Context(), cfg, set)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to build the access response: %v", err), http.StatusInternalServerError)
		return
	}

	if dryRun {
		writeJSON(w, resp)
		return
	}

	cfg.Access = &set
	if err := config.SaveConfig(s.reconciler.ConfigPath(), cfg); err != nil {
		http.Error(w, fmt.Sprintf("failed to save config: %v", err), http.StatusInternalServerError)
		return
	}

	if err := s.reconciler.ApplyAccess(len(set.Rules)); err != nil {
		http.Error(w, fmt.Sprintf("failed to apply the rule set: %v", err), http.StatusInternalServerError)
		return
	}

	writeJSON(w, resp)
}

// dryRunParam returns the value of the query parameter dry_run, and it reports whether
// the request continues. An absent parameter means false.
// dryRunParam refuses the request with HTTP 400 when the value is not a boolean, because
// a route corrects no bad value.
func dryRunParam(w http.ResponseWriter, r *http.Request) (bool, bool) {
	raw := r.URL.Query().Get("dry_run")
	if raw == "" {
		return false, true
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		writeRefusal(w, fmt.Sprintf("invalid dry_run %q: the value is true or false", raw))
		return false, false
	}
	return value, true
}

// topology returns the host facts that the compiler needs, built from the configuration
// alone. The veth device name and the veth address come from the tailnet identifier, so
// the route runs no host command and it needs no running namespace.
func topology(cfg *config.Config) access.Topology {
	devices := make(map[string]string, len(cfg.Tailnets))
	for _, tn := range cfg.Tailnets {
		hostVeth, _ := namespaces.VethNames(namespaces.GetNamespaceName(tn.ID))
		devices[tn.ID] = hostVeth
	}
	bindAddress := cfg.Resolver.BindAddress
	if bindAddress == "" {
		bindAddress = dns.DefaultBindAddress
	}
	return access.Topology{Devices: devices, DNSAddress: bindAddress}
}

// buildAccessResponse returns the body that GET /api/access and PUT /api/access return.
// cfg holds the declared tailnets, and set holds the rule set to report.
// buildAccessResponse reads the peer count of each tailnet from the live daemon, and it
// reports the count 0 for a tailnet whose daemon answers with an error, because a tailnet
// that is not running is still a node of the model.
// buildAccessResponse returns an error when the veth address of a tailnet is outside the
// infra subnet.
func (s *Server) buildAccessResponse(ctx context.Context, cfg *config.Config, set access.RuleSet) (*AccessResponse, error) {
	rules := make([]access.Rule, 0, len(set.Rules))
	for _, rule := range set.Rules {
		// The console reads ports as a list, therefore an absent port list encodes as an
		// empty list rather than as null.
		if rule.Ports == nil {
			rule.Ports = []string{}
		}
		rules = append(rules, rule)
	}

	nodes := make([]AccessNode, 0, len(cfg.Tailnets)+2)
	for _, tn := range cfg.Tailnets {
		nsName := namespaces.GetNamespaceName(tn.ID)
		_, _, _, nsAddress, err := namespaces.VethIPs(s.reconciler.InfraSubnet(), namespaces.VethIndex(nsName))
		if err != nil {
			return nil, fmt.Errorf("the veth address of the tailnet %q: %w", tn.ID, err)
		}
		nodes = append(nodes, AccessNode{
			ID:    tn.ID,
			Kind:  kindTailnet,
			Peers: s.peerCount(ctx, tn.ID),
			Veth:  nsAddress,
		})
	}
	nodes = append(nodes,
		AccessNode{ID: access.Host, Kind: access.Host},
		AccessNode{ID: access.Internet, Kind: access.Internet})

	return &AccessResponse{Mode: set.EffectiveMode(), Rules: rules, Nodes: nodes}, nil
}

// peerCount returns the number of peers that the tailnet holds, and it returns 0 when the
// daemon of the tailnet is not running or answers with an error.
func (s *Server) peerCount(ctx context.Context, id string) int {
	status, err := s.reconciler.GetTailscaleStatus(ctx, id)
	if err != nil || status == nil {
		return 0
	}
	return len(status.Peer)
}

// writeJSON writes value as the JSON body of a successful response.
func writeJSON(w http.ResponseWriter, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("api: encode the response: %v", err)
	}
}
