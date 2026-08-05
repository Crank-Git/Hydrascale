package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig_applies_the_default_secrets_file_path(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("version: 2\ntailnets: []\n"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.SecretsFile != DefaultSecretsPath {
		t.Errorf("SecretsFile = %q, want %q", cfg.SecretsFile, DefaultSecretsPath)
	}
}

func TestLoadConfig_keeps_the_declared_secrets_file_path(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := "version: 2\ntailnets: []\nsecrets_file: /etc/hydrascale/other.yaml\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.SecretsFile != "/etc/hydrascale/other.yaml" {
		t.Errorf("SecretsFile = %q, want /etc/hydrascale/other.yaml", cfg.SecretsFile)
	}
}

func TestSaveConfig_writes_no_credential_key(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := DefaultConfig()
	cfg.Tailnets = []Tailnet{{ID: "jbones"}, {ID: "corp"}}
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	// A credential lives in the secrets file alone. The configuration file names the path
	// of that file and it holds no credential key. See FR-policy-1.
	for _, key := range []string{
		"tailscale_oauth_client_id",
		"tailscale_oauth_client_secret",
		"headscale_api_key",
	} {
		if strings.Contains(string(data), key) {
			t.Errorf("the configuration file holds the key %q: %s", key, data)
		}
	}
	if !strings.Contains(string(data), "secrets_file: "+DefaultSecretsPath) {
		t.Errorf("the configuration file names no secrets file: %s", data)
	}
}

func TestTailnet_encodes_no_auth_key_in_JSON(t *testing.T) {
	data, err := json.Marshal(Tailnet{ID: "corp", AuthKey: "tskey-auth-kTESTSECRET123"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data), "tskey-auth-kTESTSECRET123") {
		t.Errorf("the JSON form of a tailnet holds the auth key value: %s", data)
	}
	if !strings.Contains(string(data), "corp") {
		t.Errorf("the JSON form of a tailnet holds no identifier: %s", data)
	}
}
