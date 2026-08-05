package secrets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFile writes body to a new file under a temporary directory, applies mode, and
// returns the path.
func writeFile(t *testing.T, body string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "secrets.yaml")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	return path
}

const sampleStore = `tailnets:
  jbones:
    tailscale_oauth_client_id: "kID"
    tailscale_oauth_client_secret: "kSECRET"
  corp:
    headscale_api_key: "kAPIKEY"
    headscale_address: "https://headscale.example.net"
`

func TestLoad_returns_an_empty_store_when_the_file_does_not_exist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.yaml")

	store, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if store == nil {
		t.Fatal("Load returned no store")
	}
	if len(store.Tailnets) != 0 {
		t.Errorf("Load returned %d tailnets, want 0", len(store.Tailnets))
	}
}

func TestLoad_reads_a_file_with_mode_0600(t *testing.T) {
	path := writeFile(t, sampleStore, 0600)

	store, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := store.Tailnets["jbones"].TailscaleOAuthClientSecret; got != "kSECRET" {
		t.Errorf("the OAuth client secret of jbones = %q, want kSECRET", got)
	}
	if got := store.Tailnets["corp"].HeadscaleAPIKey; got != "kAPIKEY" {
		t.Errorf("the Headscale API key of corp = %q, want kAPIKEY", got)
	}
	if got := store.Tailnets["corp"].HeadscaleAddress; got != "https://headscale.example.net" {
		t.Errorf("the Headscale address of corp = %q", got)
	}
}

func TestLoad_refuses_a_file_that_grants_group_access(t *testing.T) {
	path := writeFile(t, sampleStore, 0640)

	store, err := Load(path)
	if err == nil {
		t.Fatal("Load accepted a file with mode 0640")
	}
	if store != nil {
		t.Error("Load returned a store for a file it refuses")
	}
	if !strings.Contains(err.Error(), "0640") {
		t.Errorf("the error names no mode: %v", err)
	}
}

func TestLoad_refuses_a_file_that_grants_other_access(t *testing.T) {
	path := writeFile(t, sampleStore, 0644)

	if _, err := Load(path); err == nil {
		t.Fatal("Load accepted a file with mode 0644")
	}
}

func TestLoad_refuses_a_symbolic_link(t *testing.T) {
	target := writeFile(t, sampleStore, 0600)
	link := filepath.Join(filepath.Dir(target), "link.yaml")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	if _, err := Load(link); err == nil {
		t.Fatal("Load accepted a symbolic link")
	}
}

func TestSave_creates_the_file_with_mode_0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hydrascale", "secrets.yaml")
	store := &Store{Tailnets: map[string]Tailnet{
		"corp": {HeadscaleAPIKey: "kAPIKEY"},
	}}

	if err := Save(path, store); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("the mode of the secrets file = %04o, want 0600", info.Mode().Perm())
	}

	back, err := Load(path)
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if back.Tailnets["corp"].HeadscaleAPIKey != "kAPIKEY" {
		t.Errorf("the Headscale API key of corp did not survive the write")
	}
}

func TestSave_narrows_the_mode_of_an_existing_file(t *testing.T) {
	path := writeFile(t, sampleStore, 0644)

	if err := Save(path, &Store{}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("the mode of the secrets file = %04o, want 0600", info.Mode().Perm())
	}
}

func TestSave_refuses_a_symbolic_link(t *testing.T) {
	target := writeFile(t, sampleStore, 0600)
	link := filepath.Join(filepath.Dir(target), "link.yaml")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	if err := Save(link, &Store{}); err == nil {
		t.Fatal("Save accepted a symbolic link")
	}
}

func TestStore_encodes_no_credential_in_JSON(t *testing.T) {
	store := &Store{Tailnets: map[string]Tailnet{
		"corp": {
			TailscaleOAuthClientID:     "kID",
			TailscaleOAuthClientSecret: "kSECRET",
			HeadscaleAPIKey:            "kAPIKEY",
		},
	}}

	data, err := json.Marshal(store)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, value := range []string{"kID", "kSECRET", "kAPIKEY"} {
		if strings.Contains(string(data), value) {
			t.Errorf("the JSON form of the store holds the credential %q: %s", value, data)
		}
	}
}
