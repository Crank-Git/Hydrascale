// Package policy holds the credential store and the control server clients.
package policy

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SecretsMode is the mode of the secrets file. A wider mode lets a non-root account read
// a credential, therefore the loader refuses it.
const SecretsMode os.FileMode = 0600

// Credential holds the credentials of one tailnet.
// Every field carries the tag json:"-", because the control API encodes no credential
// value. See FR-policy-4, and issue #72 which gave config.Tailnet.AuthKey the same tag.
type Credential struct {
	TailscaleOAuthClientID     string `yaml:"tailscale_oauth_client_id,omitempty" json:"-"`
	TailscaleOAuthClientSecret string `yaml:"tailscale_oauth_client_secret,omitempty" json:"-"`
	HeadscaleAPIKey            string `yaml:"headscale_api_key,omitempty" json:"-"`
	HeadscaleAddress           string `yaml:"headscale_address,omitempty" json:"-"`
}

// secretsFile is the form of the secrets file on disk.
type secretsFile struct {
	Tailnets map[string]Credential `yaml:"tailnets"`
}

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
// tailnet. LoadCredential reads the file on every call, because the daemon reads a
// credential at the moment it needs it and holds none between requests.
// An environment variable overrides the matching file value, and it overrides that value
// alone. LoadCredential returns an empty credential and no error when the file is absent,
// and when the file holds no block for the tailnet.
// LoadCredential returns an error when the mode of the file is wider than 0600, when it
// cannot read the file, and when the file is not valid YAML. No error holds a credential
// value.
func LoadCredential(path, tailnetID string) (Credential, error) {
	cred, err := fileCredential(path, tailnetID)
	if err != nil {
		return Credential{}, err
	}

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

// fileCredential returns the credential that the secrets file holds for one tailnet, and
// it applies no environment variable.
func fileCredential(path, tailnetID string) (Credential, error) {
	file, err := readSecrets(path)
	if err != nil {
		return Credential{}, err
	}
	return file.Tailnets[tailnetID], nil
}

// readSecrets returns the parsed secrets file at path.
// readSecrets returns an empty file and no error when path names no file. It returns an
// error when the mode is wider than 0600, and it names the path and the mode in that
// error. The error of a parse failure holds the path and the reason of the YAML parser,
// and it holds no line of the file.
func readSecrets(path string) (secretsFile, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return secretsFile{}, nil
	}
	if err != nil {
		return secretsFile{}, fmt.Errorf("failed to stat the secrets file %s: %w", path, err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		return secretsFile{}, fmt.Errorf("the secrets file %s has mode %04o: the mode must be %04o", path, perm, SecretsMode.Perm())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return secretsFile{}, fmt.Errorf("failed to read the secrets file %s: %w", path, err)
	}

	var file secretsFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		// The message of the parser can quote the line that failed, and that line can hold
		// a credential value, therefore the error names the path alone.
		return secretsFile{}, fmt.Errorf("failed to parse the secrets file %s", path)
	}
	return file, nil
}

// WriteCredential writes the credential of one tailnet into the secrets file at path.
// WriteCredential keeps the credential of every other tailnet. It writes a temporary file
// at mode 0600 in the directory of path, and it renames that file over path, so a reader
// never sees a partial file and a previous file at a wider mode never survives. The
// daemon runs as root, therefore the owner of the file is root.
// WriteCredential creates no directory, because three writers already set the mode of
// /etc/hydrascale; see SA-23 of docs/security-audit.md. The caller creates the directory.
// WriteCredential returns an error when tailnetID is empty, when it cannot read an
// existing file, and when it cannot write the file. It changes no file in those cases.
// No error holds a credential value.
func WriteCredential(path, tailnetID string, cred Credential) error {
	if tailnetID == "" {
		return errors.New("the tailnet identifier is empty")
	}

	file, err := readSecrets(path)
	if err != nil {
		return err
	}
	if file.Tailnets == nil {
		file.Tailnets = make(map[string]Credential, 1)
	}
	file.Tailnets[tailnetID] = cred

	data, err := yaml.Marshal(file)
	if err != nil {
		return fmt.Errorf("failed to marshal the secrets file %s", path)
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".hydrascale-secrets-*")
	if err != nil {
		return fmt.Errorf("failed to create a temporary file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()

	// The mode comes before the write, so the credential never rests at a wider mode.
	if err := tmp.Chmod(SecretsMode); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to set the mode of the temporary file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write the temporary file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to close the temporary file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename the temporary file to %s: %w", path, err)
	}
	return nil
}
