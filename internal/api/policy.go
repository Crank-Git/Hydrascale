package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"

	"hydrascale/internal/config"
	"hydrascale/internal/policy"
	"hydrascale/internal/secrets"
)

// The two control server kinds. A tailnet that declares no control URL joins the
// Tailscale coordination server, and a tailnet that declares one joins a Headscale
// control server.
const (
	KindTailscale = "tailscale"
	KindHeadscale = "headscale"
)

// EventPolicyPushed is the event kind of a policy write. See FR-policy-20.
const EventPolicyPushed = "policy.pushed"

// The three credential states that one row of GET /api/policy carries. See issue #276.
const (
	// CredentialAbsent states that the tailnet holds no credential.
	CredentialAbsent = "absent"
	// CredentialRejected states that the tailnet holds a credential that the control
	// server accepts for no request. The shape of the value states it before any request,
	// and the answer of the control server states it after one.
	CredentialRejected = "rejected"
	// CredentialUsable states that the tailnet holds a credential of the right shape that
	// the control server refused for no request yet.
	CredentialUsable = "usable"
)

// tailnetOfAccessToken is the tailnet name that the daemon sends to the Tailscale API.
// The dash names the default tailnet of the access token, which the description of the
// path parameter tailnet states in the Tailscale OpenAPI schema, retrieved 2026-08-05.
// The credential is per tailnet, therefore the dash always names the tailnet that the
// operator declared the credential for.
const tailnetOfAccessToken = "-"

// handlePolicyList serves GET /api/policy.
// The response holds one row per declared tailnet, with the control server kind, the
// credential state, and the write availability. It holds no credential value, which
// FR-policy-4 requires and which the tag json:"-" on every field of secrets.Tailnet
// enforces.
func (s *Server) handlePolicyList(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig(s.reconciler.ConfigPath())
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	rows := make([]PolicyTailnet, 0, len(cfg.Tailnets))
	for _, tn := range cfg.Tailnets {
		cred, err := policy.LoadCredential(cfg.SecretsFile, tn.ID)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to read the secrets file: %v", err), http.StatusInternalServerError)
			return
		}
		rows = append(rows, policyRow(tn.ID, controlServerKind(cfg, tn), cred, s.credentialRejection(tn.ID)))
	}
	writeJSON(w, PolicyListResponse{Tailnets: rows})
}

// handlePolicyRead serves GET /api/policy/{id}.
// The response holds the policy document as text, the control server kind, the write
// availability, and the ETag value of a Tailscale read. See FR-policy-9 to FR-policy-12.
// handlePolicyRead returns HTTP 409 and a message that names the credential when the
// tailnet holds none, which FR-policy-13 requires.
func (s *Server) handlePolicyRead(w http.ResponseWriter, r *http.Request) {
	target, ok := s.policyTarget(w, r, true)
	if !ok {
		return
	}

	if target.kind == KindHeadscale {
		document, err := s.headscaleClient(target).Read(r.Context())
		s.recordCredentialResult(target.id, err)
		if err != nil {
			writePolicyError(w, "the policy read", err)
			return
		}
		writeJSON(w, PolicyResponse{ID: target.id, Kind: target.kind, Document: document, WriteAvailable: true})
		return
	}

	document, err := s.tailscaleClient(target).ReadPolicy(r.Context())
	s.recordCredentialResult(target.id, err)
	if err != nil {
		writePolicyError(w, "the policy read", err)
		return
	}
	writeJSON(w, PolicyResponse{
		ID:             target.id,
		Kind:           target.kind,
		Document:       document.Text,
		ETag:           document.ETag,
		WriteAvailable: true,
	})
}

// handlePolicyValidate serves POST /api/policy/{id}/validate.
// The route sends the document of the request body to the validate route of the control
// server and it returns the result, which FR-policy-14 requires. It writes nothing.
func (s *Server) handlePolicyValidate(w http.ResponseWriter, r *http.Request) {
	target, ok := s.policyTarget(w, r, true)
	if !ok {
		return
	}
	var req PolicyValidateRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Document == "" {
		writeRefusal(w, "document is required")
		return
	}

	result, err := s.validate(r.Context(), target, req.Document)
	s.recordCredentialResult(target.id, err)
	if err != nil {
		writePolicyError(w, "the policy validate", err)
		return
	}
	writeJSON(w, result)
}

// handlePolicyWrite serves PUT /api/policy/{id}.
// The route validates the whole request body, it then sends the document to the validate
// route of the control server, and it writes the document only when validate accepts it.
// FR-policy-21 requires that order.
// handlePolicyWrite returns HTTP 400 when validate rejects the document, and HTTP 409
// when the control server reports a conflict. It records the event policy.pushed after a
// write that succeeds. See FR-policy-16 to FR-policy-20.
func (s *Server) handlePolicyWrite(w http.ResponseWriter, r *http.Request) {
	target, ok := s.policyTarget(w, r, true)
	if !ok {
		return
	}
	var req PolicyWriteRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Document == "" {
		writeRefusal(w, "document is required")
		return
	}
	// The Headscale control server holds no ETag value, therefore an etag in the body is
	// a value that the daemon cannot honour. The route rejects it and corrects it never.
	if target.kind == KindHeadscale && req.ETag != "" {
		writeRefusal(w, "etag is a Tailscale value, and the tailnet "+target.id+" uses a Headscale control server")
		return
	}

	result, err := s.validate(r.Context(), target, req.Document)
	s.recordCredentialResult(target.id, err)
	if err != nil {
		writePolicyError(w, "the policy validate", err)
		return
	}
	if !result.Passed {
		writeRefusal(w, "the control server rejected the document: "+result.Result)
		return
	}

	response := PolicyResponse{ID: target.id, Kind: target.kind, Document: req.Document, WriteAvailable: true}
	if target.kind == KindHeadscale {
		err := s.headscaleClient(target).Write(r.Context(), req.Document)
		s.recordCredentialResult(target.id, err)
		if err != nil {
			writePolicyError(w, "the policy write", err)
			return
		}
	} else {
		written, err := s.tailscaleClient(target).WritePolicy(r.Context(), req.Document, req.ETag)
		s.recordCredentialResult(target.id, err)
		if err != nil {
			writePolicyError(w, "the policy write", err)
			return
		}
		response.Document = written.Text
		response.ETag = written.ETag
	}

	// The message holds the control server kind and no part of the document, because an
	// event reaches the event log file and every reader of GET /api/events.
	s.reconciler.RecordEvent(EventPolicyPushed, target.id, "wrote the policy to the "+target.kind+" control server")
	writeJSON(w, response)
}

// handlePolicyCredentials serves PUT /api/policy/{id}/credentials.
// The route validates every field of the request body against the control server kind of
// the tailnet, and it then writes the secrets file. See FR-policy-6.
// The response states the credential state and it holds no credential value.
func (s *Server) handlePolicyCredentials(w http.ResponseWriter, r *http.Request) {
	target, ok := s.policyTarget(w, r, false)
	if !ok {
		return
	}
	var req PolicyCredentialsRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := req.validate(target.kind, target.id); err != nil {
		writeRefusal(w, err.Error())
		return
	}

	store, err := secrets.Load(target.secretsFile)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read the secrets file: %v", err), http.StatusInternalServerError)
		return
	}
	cred := secrets.Tailnet{
		TailscaleOAuthClientID:     req.TailscaleOAuthClientID,
		TailscaleOAuthClientSecret: req.TailscaleOAuthClientSecret,
		HeadscaleAPIKey:            req.HeadscaleAPIKey,
		HeadscaleAddress:           req.HeadscaleAddress,
	}
	store.Tailnets[target.id] = cred
	if err := secrets.Save(target.secretsFile, store); err != nil {
		http.Error(w, fmt.Sprintf("failed to write the secrets file: %v", err), http.StatusInternalServerError)
		return
	}
	// The stored rejection describes the credential that this write replaced, therefore it
	// describes the new one never. The next call to the control server records its own
	// result. See issue #276.
	s.recordCredentialResult(target.id, nil)
	writeJSON(w, policyRow(target.id, target.kind, cred, ""))
}

// The three edit operations that POST /api/policy/{id}/sections/edit accepts.
const (
	sectionOpAdd     = "add"
	sectionOpReplace = "replace"
	sectionOpRemove  = "remove"
)

// sectionNames lists every top-level key that features/11-policy-document-model.md
// FR-model-2 resolves into a named section. A key outside this list is opaque, per
// FR-vacl-18.
var sectionNames = []string{
	"groups", "hosts", "tagOwners", "ipsets", "acls", "grants",
	"ssh", "autoApprovers", "nodeAttrs", "postures", "tests", "sshTests",
}

// handlePolicySections serves POST /api/policy/{id}/sections.
// The route parses the request body's document field through the document model and
// returns every named section as JSON, per features/12-visual-acl-editor.md's
// Interfaces section. It reaches no control server and it changes nothing.
func (s *Server) handlePolicySections(w http.ResponseWriter, r *http.Request) {
	if err := validateTailnetID(r.PathValue("id")); err != nil {
		writeRefusal(w, err.Error())
		return
	}
	var req PolicySectionsRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Document == "" {
		writeRefusal(w, "document is required")
		return
	}

	doc, err := policy.Parse(req.Document)
	if err != nil {
		writeRefusal(w, err.Error())
		return
	}
	response, err := policySections(doc)
	if err != nil {
		writeRefusal(w, err.Error())
		return
	}
	writeJSON(w, response)
}

// policySections reads every named section of doc, plus the list of opaque top-level
// keys. Groups, ACLs, SSH, and AutoApprovers use the typed accessors of #310; a section
// FR-model-2 names with no typed accessor yet is read from the raw top-level key, so
// that the response holds every section the plan promises to the sibling issues.
func policySections(doc *policy.Document) (PolicySectionsResponse, error) {
	groups, err := doc.Groups()
	if err != nil {
		return PolicySectionsResponse{}, err
	}
	acls, err := doc.ACLs()
	if err != nil {
		return PolicySectionsResponse{}, err
	}
	ssh, err := doc.SSH()
	if err != nil {
		return PolicySectionsResponse{}, err
	}
	autoApprovers, err := doc.AutoApprovers()
	if err != nil {
		return PolicySectionsResponse{}, err
	}

	raw, err := doc.RawSections()
	if err != nil {
		return PolicySectionsResponse{}, err
	}

	response := PolicySectionsResponse{
		Groups:        emptyIfNil(groups),
		Hosts:         map[string]string{},
		TagOwners:     map[string][]string{},
		IPSets:        map[string][]string{},
		ACLs:          emptyIfNilACLs(acls),
		Grants:        []json.RawMessage{},
		SSH:           emptyIfNilSSH(ssh),
		AutoApprovers: autoApprovers,
		NodeAttrs:     []json.RawMessage{},
		Postures:      map[string][]string{},
		Tests:         []json.RawMessage{},
		SSHTests:      []json.RawMessage{},
	}
	if err := unmarshalSection(raw, "hosts", &response.Hosts); err != nil {
		return PolicySectionsResponse{}, err
	}
	if err := unmarshalSection(raw, "tagOwners", &response.TagOwners); err != nil {
		return PolicySectionsResponse{}, err
	}
	if err := unmarshalSection(raw, "ipsets", &response.IPSets); err != nil {
		return PolicySectionsResponse{}, err
	}
	if err := unmarshalSection(raw, "grants", &response.Grants); err != nil {
		return PolicySectionsResponse{}, err
	}
	if err := unmarshalSection(raw, "nodeAttrs", &response.NodeAttrs); err != nil {
		return PolicySectionsResponse{}, err
	}
	if err := unmarshalSection(raw, "postures", &response.Postures); err != nil {
		return PolicySectionsResponse{}, err
	}
	if err := unmarshalSection(raw, "tests", &response.Tests); err != nil {
		return PolicySectionsResponse{}, err
	}
	if err := unmarshalSection(raw, "sshTests", &response.SSHTests); err != nil {
		return PolicySectionsResponse{}, err
	}

	known := make(map[string]bool, len(sectionNames))
	for _, name := range sectionNames {
		known[name] = true
	}
	opaque := make([]string, 0, len(raw))
	for key := range raw {
		if !known[key] {
			opaque = append(opaque, key)
		}
	}
	sort.Strings(opaque)
	response.OpaqueKeys = opaque

	return response, nil
}

// unmarshalSection decodes raw[name] into dst when the document holds that key. dst
// keeps its zero value, an empty map or an empty list, when the document holds no such
// key, per FR-model-6.
func unmarshalSection(raw map[string]json.RawMessage, name string, dst interface{}) error {
	value, ok := raw[name]
	if !ok {
		return nil
	}
	if err := json.Unmarshal(value, dst); err != nil {
		return fmt.Errorf("policy: section %q: %w", name, err)
	}
	return nil
}

func emptyIfNil(groups map[string][]string) map[string][]string {
	if groups == nil {
		return map[string][]string{}
	}
	return groups
}

func emptyIfNilACLs(acls []policy.ACLRule) []policy.ACLRule {
	if acls == nil {
		return []policy.ACLRule{}
	}
	return acls
}

func emptyIfNilSSH(rules []policy.SSHRule) []policy.SSHRule {
	if rules == nil {
		return []policy.SSHRule{}
	}
	return rules
}

// handlePolicySectionsEdit serves POST /api/policy/{id}/sections/edit.
// The route validates the whole request body before it changes anything, per this
// project's validation convention. It then parses the document, applies the one edit
// through the matching method of #311, and serializes the result with #312's Bytes.
// It reaches no control server; the operator's later Push writes the result.
func (s *Server) handlePolicySectionsEdit(w http.ResponseWriter, r *http.Request) {
	if err := validateTailnetID(r.PathValue("id")); err != nil {
		writeRefusal(w, err.Error())
		return
	}
	var req PolicySectionsEditRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := req.validate(); err != nil {
		writeRefusal(w, err.Error())
		return
	}

	doc, err := policy.Parse(req.Document)
	if err != nil {
		writeRefusal(w, err.Error())
		return
	}

	switch req.Op {
	case sectionOpAdd:
		if err := doc.AddEntry(req.Section, string(req.Entry)); err != nil {
			writeRefusal(w, err.Error())
			return
		}
	case sectionOpReplace:
		if err := doc.ReplaceEntry(req.Section, *req.Index, string(req.Entry)); err != nil {
			writeRefusal(w, err.Error())
			return
		}
	case sectionOpRemove:
		if err := doc.RemoveEntry(req.Section, *req.Index); err != nil {
			writeRefusal(w, err.Error())
			return
		}
	}

	writeJSON(w, PolicySectionsEditResponse{Document: string(doc.Bytes())})
}

// validate returns an error when the request body of POST /api/policy/{id}/sections/edit
// holds a bad value. It runs before the route changes anything, matching this project's
// validation convention.
func (req PolicySectionsEditRequest) validate() error {
	if req.Document == "" {
		return errors.New("document is required")
	}
	if req.Section == "" {
		return errors.New("section is required")
	}
	switch req.Op {
	case sectionOpAdd:
		if len(req.Entry) == 0 {
			return errors.New("entry is required for op \"add\"")
		}
	case sectionOpReplace:
		if req.Index == nil {
			return errors.New("index is required for op \"replace\"")
		}
		if len(req.Entry) == 0 {
			return errors.New("entry is required for op \"replace\"")
		}
	case sectionOpRemove:
		if req.Index == nil {
			return errors.New("index is required for op \"remove\"")
		}
	default:
		return fmt.Errorf("op %q is not one of \"add\", \"replace\", \"remove\"", req.Op)
	}
	return nil
}

// validate returns the result of the validate route of the control server.
// target names the tailnet and document holds the text to check.
func (s *Server) validate(ctx context.Context, target policyTarget, document string) (PolicyValidateResponse, error) {
	if target.kind == KindHeadscale {
		// The Headscale check route answers with an empty body and status 200 when it
		// accepts the document, therefore the error is the whole result.
		if err := s.headscaleClient(target).Check(ctx, document); err != nil {
			if errors.Is(err, policy.ErrHeadscaleNoPolicyRoute) {
				return PolicyValidateResponse{}, err
			}
			return PolicyValidateResponse{Passed: false, Result: err.Error()}, nil
		}
		return PolicyValidateResponse{Passed: true}, nil
	}

	result, err := s.tailscaleClient(target).ValidatePolicy(ctx, document)
	if err != nil {
		return PolicyValidateResponse{}, err
	}
	return PolicyValidateResponse{Passed: result.Passed, Result: result.Body}, nil
}

// policyTarget holds the facts that one policy route needs about one tailnet.
type policyTarget struct {
	id          string
	kind        string
	cred        secrets.Tailnet
	secretsFile string
}

// policyTarget returns the target of the request, and it reports whether the request
// continues.
// needCredential is true for a route that reaches the control server. policyTarget then
// returns HTTP 409 and a message that names the credential when the tailnet holds none.
// policyTarget refuses an identifier that config.IsValidID rejects, before the identifier
// reaches any file path. See SA-15.
func (s *Server) policyTarget(w http.ResponseWriter, r *http.Request, needCredential bool) (policyTarget, bool) {
	id := r.PathValue("id")
	if err := validateTailnetID(id); err != nil {
		writeRefusal(w, err.Error())
		return policyTarget{}, false
	}

	cfg, err := config.LoadConfig(s.reconciler.ConfigPath())
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to load config: %v", err), http.StatusInternalServerError)
		return policyTarget{}, false
	}
	if !hasTailnet(cfg, id) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("the configuration declares no tailnet %q", id))
		return policyTarget{}, false
	}

	var declared config.Tailnet
	for _, tn := range cfg.Tailnets {
		if tn.ID == id {
			declared = tn
		}
	}
	cred, err := policy.LoadCredential(cfg.SecretsFile, id)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read the secrets file: %v", err), http.StatusInternalServerError)
		return policyTarget{}, false
	}

	target := policyTarget{
		id:          id,
		kind:        controlServerKind(cfg, declared),
		cred:        cred,
		secretsFile: cfg.SecretsFile,
	}
	if needCredential {
		if reason := missingCredential(target.kind, target.id, cred); reason != "" {
			writeError(w, http.StatusConflict, reason)
			return policyTarget{}, false
		}
	}
	return target, true
}

// tailscaleClient returns the Tailscale client of the tailnet.
// The client reads the credential at the moment it needs it, therefore the daemon holds
// no credential between requests.
func (s *Server) tailscaleClient(target policyTarget) *policy.TailscaleClient {
	secretsFile, id := target.secretsFile, target.id
	return policy.NewTailscaleClient(s.tailscaleBaseURL, tailnetOfAccessToken, func() (secrets.Tailnet, error) {
		return policy.LoadCredential(secretsFile, id)
	})
}

// headscaleClient returns the Headscale client of the tailnet.
// The address is the value of headscale_address in the credential block, which
// features/08-upstream-policy.md names as the base URL.
func (s *Server) headscaleClient(target policyTarget) *policy.HeadscaleClient {
	return policy.NewHeadscaleClient(target.cred.HeadscaleAddress, target.cred.HeadscaleAPIKey)
}

// controlServerKind returns the control server kind of one declared tailnet.
// A tailnet with no control URL joins the Tailscale coordination server. A tailnet with a
// control URL, per tailnet or global, joins a Headscale control server.
func controlServerKind(cfg *config.Config, tn config.Tailnet) string {
	if config.ResolveControlURL(tn.ControlURL, cfg.ControlURL) == "" {
		return KindTailscale
	}
	return KindHeadscale
}

// policyRow returns one row of GET /api/policy for a tailnet.
// The write availability of a Headscale tailnet states that the daemon holds the
// credential. The daemon learns the policy mode of that control server from the answer to
// a write, because the control server exposes no route that states the mode.
// rejection carries the message of the last call that the control server refused for this
// tailnet, and it is empty when no call was refused.
func policyRow(id, kind string, cred secrets.Tailnet, rejection string) PolicyTailnet {
	if reason := missingCredential(kind, id, cred); reason != "" {
		return PolicyTailnet{
			ID:              id,
			Kind:            kind,
			CredentialState: CredentialAbsent,
			Reason:          reason,
		}
	}
	if reason := credentialRefused(kind, cred, rejection); reason != "" {
		// The credential is present, therefore CredentialPresent stays true and
		// FR-policy-5 holds. The write is not available, because the control server takes
		// no request that carries this credential.
		return PolicyTailnet{
			ID:                id,
			Kind:              kind,
			CredentialPresent: true,
			CredentialState:   CredentialRejected,
			Reason:            reason,
		}
	}
	return PolicyTailnet{
		ID:                id,
		Kind:              kind,
		CredentialPresent: true,
		WriteAvailable:    true,
		CredentialState:   CredentialUsable,
	}
}

// credentialRefused returns the reason that the control server takes no request with this
// credential, and it returns an empty string when no reason is known.
//
// The shape of the value states the reason before any request runs. The answer of the
// control server states it after one, and that answer reaches this function as rejection.
// See issue #276.
func credentialRefused(kind string, cred secrets.Tailnet, rejection string) string {
	if kind == KindTailscale {
		if problem := policy.TailscaleSecretProblem(cred.TailscaleOAuthClientSecret); problem != "" {
			return problem
		}
	}
	if rejection != "" {
		return "the control server refused the credential: " + rejection
	}
	return ""
}

// missingCredential returns a message that names the credential that the tailnet needs,
// and it returns an empty string when the tailnet holds a complete credential.
// The message names the file keys and the environment variables, so that the operator
// reads which value to supply. See FR-policy-13.
func missingCredential(kind, id string, cred secrets.Tailnet) string {
	if kind == KindHeadscale {
		if cred.HeadscaleAPIKey == "" || cred.HeadscaleAddress == "" {
			return fmt.Sprintf("the tailnet %q has no Headscale credential: set headscale_api_key and headscale_address in the secrets file, or set %s",
				id, policy.HeadscaleAPIKeyEnvVar(id))
		}
		return ""
	}
	if cred.TailscaleOAuthClientID == "" || cred.TailscaleOAuthClientSecret == "" {
		return fmt.Sprintf("the tailnet %q has no Tailscale OAuth credential: set tailscale_oauth_client_id and tailscale_oauth_client_secret in the secrets file, or set %s and %s",
			id, policy.TailscaleClientIDEnvVar(id), policy.TailscaleClientSecretEnvVar(id))
	}
	return ""
}

// validate returns an error when the request body does not hold the credential that the
// control server kind needs.
// The route rejects a field of the other kind rather than ignoring it, because a value
// that the daemon ignores reads to the operator as a value that the daemon holds.
func (req PolicyCredentialsRequest) validate(kind, id string) error {
	if kind == KindHeadscale {
		if req.TailscaleOAuthClientID != "" || req.TailscaleOAuthClientSecret != "" {
			return fmt.Errorf("the tailnet %q uses a Headscale control server, and it takes no Tailscale OAuth credential", id)
		}
		if req.HeadscaleAPIKey == "" {
			return errors.New("headscale_api_key is required")
		}
		if req.HeadscaleAddress == "" {
			return errors.New("headscale_address is required")
		}
		if err := config.ValidateControlURL(req.HeadscaleAddress); err != nil {
			return fmt.Errorf("invalid headscale_address: %w", err)
		}
		return nil
	}

	if req.HeadscaleAPIKey != "" || req.HeadscaleAddress != "" {
		return fmt.Errorf("the tailnet %q uses the Tailscale control server, and it takes no Headscale credential", id)
	}
	if req.TailscaleOAuthClientID == "" {
		return errors.New("tailscale_oauth_client_id is required")
	}
	if req.TailscaleOAuthClientSecret == "" {
		return errors.New("tailscale_oauth_client_secret is required")
	}
	return nil
}

// writePolicyError refuses the request with the status that the failure of the control
// server maps to.
// operation names the call. A conflict and the file policy mode both return HTTP 409,
// because each is a state of the control server that the operator resolves. Every other
// failure of the control server returns HTTP 502, which
// features/08-upstream-policy.md states for an invalid OAuth client.
func writePolicyError(w http.ResponseWriter, operation string, err error) {
	switch {
	case errors.Is(err, policy.ErrPolicyConflict):
		writeError(w, http.StatusConflict, "the policy changed since the read: read the policy again and apply the change to the new document")
	case errors.Is(err, policy.ErrNoCredential), errors.Is(err, policy.ErrHeadscaleFileMode):
		writeError(w, http.StatusConflict, err.Error())
	default:
		writeError(w, http.StatusBadGateway, fmt.Sprintf("%s failed: %v", operation, err))
	}
}

// writeError refuses the request with status and the body {"error": "<message>"}.
// writeError writes the reason into the log, so message must hold no credential value.
func writeError(w http.ResponseWriter, status int, message string) {
	log.Printf("api: refused a request with status %d: %s", status, message)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(ErrorResponse{Error: message}); err != nil {
		log.Printf("api: encode the refusal: %v", err)
	}
}

// recordCredentialResult keeps the result of one call to a control server for the tailnet
// id. It stores the message of a call that the control server refused because the
// credential itself is not valid, and it clears the entry after a call that succeeds.
//
// The status 401 alone marks the credential. A 403 states that the credential is valid and
// that its scopes do not cover the request, which is a different repair. See issue #276.
func (s *Server) recordCredentialResult(id string, err error) {
	s.credMu.Lock()
	defer s.credMu.Unlock()
	if s.credRejections == nil {
		s.credRejections = make(map[string]string)
	}
	if err == nil {
		delete(s.credRejections, id)
		return
	}
	var apiErr *policy.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusUnauthorized {
		s.credRejections[id] = apiErr.Message
	}
}

// credentialRejection returns the message that recordCredentialResult stored for id.
func (s *Server) credentialRejection(id string) string {
	s.credMu.Lock()
	defer s.credMu.Unlock()
	return s.credRejections[id]
}
