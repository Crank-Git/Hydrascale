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
