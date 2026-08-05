// Package secrets reads and writes the root-only credential store of Hydrascale.
//
// The configuration file never holds a credential, because the control API returns the
// configuration to any caller of the socket. A credential lives in the file that the
// configuration key secrets_file names, at mode 0600 and owner root.
package secrets

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// DefaultPath is the default location of the secrets file.
const DefaultPath = "/etc/hydrascale/secrets.yaml"

// Tailnet holds the upstream policy credentials of one tailnet.
// Every field carries the tag json:"-", so no route can encode a credential value.
type Tailnet struct {
	TailscaleOAuthClientID     string `yaml:"tailscale_oauth_client_id,omitempty" json:"-"`
	TailscaleOAuthClientSecret string `yaml:"tailscale_oauth_client_secret,omitempty" json:"-"`
	HeadscaleAPIKey            string `yaml:"headscale_api_key,omitempty" json:"-"`
	HeadscaleAddress           string `yaml:"headscale_address,omitempty" json:"-"`
}

// Store is the content of the secrets file, keyed by tailnet identifier.
type Store struct {
	Tailnets map[string]Tailnet `yaml:"tailnets,omitempty" json:"-"`
}

// Load returns the content of the secrets file that path names.
// When the file does not exist, Load returns an empty store and no error, because the
// daemon starts without upstream policy control.
// Load returns an error when path is not a regular file, when the mode grants access to
// a group or to other accounts, or when the content is not valid YAML.
func Load(path string) (*Store, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return &Store{Tailnets: map[string]Tailnet{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read the secrets file %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("the secrets file %s is not a regular file", path)
	}
	if perm := info.Mode().Perm(); perm&0077 != 0 {
		return nil, fmt.Errorf("the secrets file %s has mode %04o, and the daemon reads mode 0600 only", path, perm)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read the secrets file %s: %w", path, err)
	}
	var store Store
	if err := yaml.Unmarshal(data, &store); err != nil {
		// The error of the parser can quote a line of the file, so it stays out of the
		// message. See FR-fix-14.
		return nil, fmt.Errorf("failed to parse the secrets file %s", path)
	}
	if store.Tailnets == nil {
		store.Tailnets = map[string]Tailnet{}
	}
	return &store, nil
}

// Save writes store to the file that path names, with mode 0600.
// Save sets the owner to root when the daemon runs as root.
// Save returns an error when path names a symbolic link, because a change of the mode
// then applies to the target of the link.
func Save(path string, store *Store) error {
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("the secrets file %s is not a regular file", path)
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read the secrets file %s: %w", path, err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create the secrets directory %s: %w", dir, err)
	}

	data, err := yaml.Marshal(store)
	if err != nil {
		return fmt.Errorf("failed to encode the secrets file: %w", err)
	}

	// The rename is atomic, so a reader gets the previous file or the new file, and
	// never a partial file.
	tmp, err := os.CreateTemp(dir, ".hydrascale-secrets-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create the temporary secrets file: %w", err)
	}
	tmpPath := tmp.Name()
	if err := writeTemp(tmp, data); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename the secrets file: %w", err)
	}
	return nil
}

// writeTemp writes data to f, applies the mode and the owner, and closes f.
// writeTemp returns the first error it meets.
func writeTemp(f *os.File, data []byte) error {
	defer f.Close()
	if err := f.Chmod(0600); err != nil {
		return fmt.Errorf("failed to set the mode of the secrets file: %w", err)
	}
	// A test and the command line interface run as a normal user, and Chown then fails
	// with EPERM. The daemon runs as root, so it applies the owner that FR-fix-16 names.
	if os.Geteuid() == 0 {
		if err := f.Chown(0, 0); err != nil {
			return fmt.Errorf("failed to set the owner of the secrets file: %w", err)
		}
	}
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("failed to write the secrets file: %w", err)
	}
	return nil
}
