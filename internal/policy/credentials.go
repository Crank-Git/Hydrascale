// Package policy holds the credential loader and the control server clients.
//
// The secrets file itself belongs to internal/secrets, which reads it, writes it, and
// enforces the mode 0600 and the regular file rule. This package adds the layer above it:
// an environment variable overrides the matching file value. See FR-policy-2.
package policy

import (
	"os"
	"strings"

	"hydrascale/internal/secrets"
)

// envVarSuffix returns the suffix that an environment variable name takes for a tailnet.
// The suffix is the identifier in upper case, with each dash replaced by an underscore.
// This is the form that config.AuthKeyEnvVar already uses.
func envVarSuffix(tailnetID string) string {
	return strings.ToUpper(strings.ReplaceAll(tailnetID, "-", "_"))
}

// TailscaleClientIDEnvVar returns the name of the variable that overrides the Tailscale
// OAuth client identifier of a tailnet. See FR-policy-3.
func TailscaleClientIDEnvVar(tailnetID string) string {
	return "HYDRASCALE_TS_CLIENT_ID_" + envVarSuffix(tailnetID)
}

// TailscaleClientSecretEnvVar returns the name of the variable that overrides the
// Tailscale OAuth client secret of a tailnet. See FR-policy-3.
func TailscaleClientSecretEnvVar(tailnetID string) string {
	return "HYDRASCALE_TS_CLIENT_SECRET_" + envVarSuffix(tailnetID)
}

// HeadscaleAPIKeyEnvVar returns the name of the variable that overrides the Headscale API
// key of a tailnet. See FR-policy-3.
func HeadscaleAPIKeyEnvVar(tailnetID string) string {
	return "HYDRASCALE_HS_API_KEY_" + envVarSuffix(tailnetID)
}

// LoadCredential returns the credential of one tailnet.
// path names the secrets file, which config.Config.SecretsFile holds. tailnetID names the
// tailnet. LoadCredential calls secrets.Load on every call, because the daemon reads a
// credential at the moment it needs it and holds none between requests.
// An environment variable overrides the matching file value, and it overrides that value
// alone. LoadCredential returns an empty credential and no error when the file is absent,
// and when the file holds no block for the tailnet.
// LoadCredential returns the error of secrets.Load unchanged. That error names the path
// and it holds no credential value.
func LoadCredential(path, tailnetID string) (secrets.Tailnet, error) {
	store, err := secrets.Load(path)
	if err != nil {
		return secrets.Tailnet{}, err
	}

	cred := store.Tailnets[tailnetID]
	if v := os.Getenv(TailscaleClientIDEnvVar(tailnetID)); v != "" {
		cred.TailscaleOAuthClientID = v
	}
	if v := os.Getenv(TailscaleClientSecretEnvVar(tailnetID)); v != "" {
		cred.TailscaleOAuthClientSecret = v
	}
	if v := os.Getenv(HeadscaleAPIKeyEnvVar(tailnetID)); v != "" {
		cred.HeadscaleAPIKey = v
	}
	return cred, nil
}

// TailscaleClientSecretPrefix is the prefix of a Tailscale OAuth client secret. The admin
// console of the control server writes a secret that starts with this value.
const TailscaleClientSecretPrefix = "tskey-client"

// tailscaleAuthKeyPrefix is the prefix of a device authentication key. That key enrols a
// device with `tailscale up --authkey`, and it reaches no API.
const tailscaleAuthKeyPrefix = "tskey-auth"

// TailscaleSecretProblem returns a message that states why the control server accepts this
// Tailscale OAuth client secret for no request, and it returns an empty string for a secret
// whose shape is right. It returns an empty string for an empty secret, because a tailnet
// that holds no credential is a separate state.
//
// The check reads the prefix alone, and the message names no part of the value.
//
// The control server answers a device authentication key with HTTP 401 and the message
// "API token invalid", which names neither the mistake nor the value. An operator wrote
// such a key on 2026-08-13 and read `read and write` on the policy view. See issue #276.
func TailscaleSecretProblem(secret string) string {
	if secret == "" || strings.HasPrefix(secret, TailscaleClientSecretPrefix) {
		return ""
	}
	if strings.HasPrefix(secret, tailscaleAuthKeyPrefix) {
		return "the value is a device authentication key, which enrols a device and reaches no API: generate an OAuth client in the admin console of the control server, whose secret starts with " + TailscaleClientSecretPrefix
	}
	return "the value is no Tailscale OAuth client secret: such a secret starts with " + TailscaleClientSecretPrefix
}
