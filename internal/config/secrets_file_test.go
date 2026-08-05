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
