package policy

import (
	"bytes"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// The test values below are not credentials. Each one is a fixed string that a test
// searches for in an error, in a log line, or in a JSON body.
const (
	testClientID     = "TESTVALUE-client-id-154"
	testClientSecret = "TESTVALUE-client-secret-154"
	testAPIKey       = "TESTVALUE-api-key-154"
)

// writeSecrets writes a secrets file at mode 0600 in a new directory and returns its path.
func writeSecrets(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "secrets.yaml")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

const twoTailnets = `tailnets:
  jbones:
    tailscale_oauth_client_id: "` + testClientID + `"
    tailscale_oauth_client_secret: "` + testClientSecret + `"
  corp:
    headscale_api_key: "` + testAPIKey + `"
    headscale_address: "https://headscale.example.net"
`

func TestLoadCredential_reads_the_credential_of_one_tailnet(t *testing.T) {
	path := writeSecrets(t, twoTailnets)

	cred, err := LoadCredential(path, "jbones")
	if err != nil {
		t.Fatalf("LoadCredential: %v", err)
	}
	if cred.TailscaleOAuthClientID != testClientID {
		t.Errorf("TailscaleOAuthClientID = %q, want %q", cred.TailscaleOAuthClientID, testClientID)
	}
	if cred.TailscaleOAuthClientSecret != testClientSecret {
		t.Errorf("TailscaleOAuthClientSecret = %q, want %q", cred.TailscaleOAuthClientSecret, testClientSecret)
	}

	corp, err := LoadCredential(path, "corp")
	if err != nil {
		t.Fatalf("LoadCredential: %v", err)
	}
	if corp.HeadscaleAPIKey != testAPIKey {
		t.Errorf("HeadscaleAPIKey = %q, want %q", corp.HeadscaleAPIKey, testAPIKey)
	}
	if corp.HeadscaleAddress != "https://headscale.example.net" {
		t.Errorf("HeadscaleAddress = %q, want https://headscale.example.net", corp.HeadscaleAddress)
	}
}

func TestLoadCredential_returns_an_empty_credential_for_an_unknown_tailnet(t *testing.T) {
	path := writeSecrets(t, twoTailnets)

	cred, err := LoadCredential(path, "absent")
	if err != nil {
		t.Fatalf("LoadCredential: %v", err)
	}
	if cred != (Credential{}) {
		t.Errorf("LoadCredential returned %+v, want an empty credential", cred)
	}
}

func TestLoadCredential_accepts_a_missing_secrets_file(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.yaml")

	cred, err := LoadCredential(path, "jbones")
	if err != nil {
		t.Fatalf("LoadCredential returned %v, want no error", err)
	}
	if cred != (Credential{}) {
		t.Errorf("LoadCredential returned %+v, want an empty credential", cred)
	}
}

func TestLoadCredential_refuses_a_secrets_file_whose_mode_is_wider_than_0600(t *testing.T) {
	path := writeSecrets(t, twoTailnets)
	if err := os.Chmod(path, 0640); err != nil {
		t.Fatalf("Chmod: %v", err)
	}

	_, err := LoadCredential(path, "jbones")
	if err == nil {
		t.Fatal("LoadCredential returned no error, want an error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("the error names no path: %v", err)
	}
	if !strings.Contains(err.Error(), "0640") {
		t.Errorf("the error names no mode: %v", err)
	}
}

func TestLoadCredential_accepts_a_secrets_file_at_mode_0400(t *testing.T) {
	path := writeSecrets(t, twoTailnets)
	if err := os.Chmod(path, 0400); err != nil {
		t.Fatalf("Chmod: %v", err)
	}

	if _, err := LoadCredential(path, "jbones"); err != nil {
		t.Fatalf("LoadCredential: %v", err)
	}
}

func TestLoadCredential_takes_the_environment_variable_over_the_file_value(t *testing.T) {
	path := writeSecrets(t, twoTailnets)

	cases := []struct {
		name    string
		tailnet string
		envVar  string
		field   func(Credential) string
	}{
		{"the Tailscale client identifier", "jbones", TailscaleClientIDEnvVar("jbones"), func(c Credential) string { return c.TailscaleOAuthClientID }},
		{"the Tailscale client secret", "jbones", TailscaleClientSecretEnvVar("jbones"), func(c Credential) string { return c.TailscaleOAuthClientSecret }},
		{"the Headscale API key", "corp", HeadscaleAPIKeyEnvVar("corp"), func(c Credential) string { return c.HeadscaleAPIKey }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.envVar, "TESTVALUE-from-the-environment")
			cred, err := LoadCredential(path, tc.tailnet)
			if err != nil {
				t.Fatalf("LoadCredential: %v", err)
			}
			if got := tc.field(cred); got != "TESTVALUE-from-the-environment" {
				t.Errorf("%s = %q, want TESTVALUE-from-the-environment", tc.envVar, got)
			}
		})
	}
}

func TestLoadCredential_keeps_the_other_file_values_when_one_variable_overrides(t *testing.T) {
	path := writeSecrets(t, twoTailnets)
	t.Setenv(TailscaleClientIDEnvVar("jbones"), "TESTVALUE-from-the-environment")

	cred, err := LoadCredential(path, "jbones")
	if err != nil {
		t.Fatalf("LoadCredential: %v", err)
	}
	if cred.TailscaleOAuthClientSecret != testClientSecret {
		t.Errorf("TailscaleOAuthClientSecret = %q, want the file value", cred.TailscaleOAuthClientSecret)
	}
}

func TestLoadCredential_takes_the_environment_variable_when_the_file_is_absent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.yaml")
	t.Setenv(HeadscaleAPIKeyEnvVar("corp"), "TESTVALUE-from-the-environment")

	cred, err := LoadCredential(path, "corp")
	if err != nil {
		t.Fatalf("LoadCredential: %v", err)
	}
	if cred.HeadscaleAPIKey != "TESTVALUE-from-the-environment" {
		t.Errorf("HeadscaleAPIKey = %q, want TESTVALUE-from-the-environment", cred.HeadscaleAPIKey)
	}
}

func TestTheEnvironmentVariableNamesFollowTheAuthKeyPattern(t *testing.T) {
	cases := []struct {
		got  string
		want string
	}{
		{TailscaleClientIDEnvVar("corp-prod"), "HYDRASCALE_TS_CLIENT_ID_CORP_PROD"},
		{TailscaleClientSecretEnvVar("corp-prod"), "HYDRASCALE_TS_CLIENT_SECRET_CORP_PROD"},
		{HeadscaleAPIKeyEnvVar("corp-prod"), "HYDRASCALE_HS_API_KEY_CORP_PROD"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("got %q, want %q", tc.got, tc.want)
		}
	}
}

func TestCredential_encodes_no_value_in_JSON(t *testing.T) {
	data, err := json.Marshal(Credential{
		TailscaleOAuthClientID:     testClientID,
		TailscaleOAuthClientSecret: testClientSecret,
		HeadscaleAPIKey:            testAPIKey,
		HeadscaleAddress:           "https://headscale.example.net",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(data) != "{}" {
		t.Errorf("the JSON form of a credential is %s, want {}", data)
	}
}

func TestWriteCredential_creates_the_secrets_file_at_mode_0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.yaml")

	err := WriteCredential(path, "jbones", Credential{
		TailscaleOAuthClientID:     testClientID,
		TailscaleOAuthClientSecret: testClientSecret,
	})
	if err != nil {
		t.Fatalf("WriteCredential: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("the mode of %s is %04o, want 0600", path, perm)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatal("Stat returned no syscall.Stat_t")
	}
	// The daemon runs as root, therefore the owner of the file it creates is root. The
	// test asserts the account that runs the test, which is root in that case.
	if int(stat.Uid) != os.Getuid() {
		t.Errorf("the owner of %s is %d, want %d", path, stat.Uid, os.Getuid())
	}

	cred, err := LoadCredential(path, "jbones")
	if err != nil {
		t.Fatalf("LoadCredential: %v", err)
	}
	if cred.TailscaleOAuthClientID != testClientID {
		t.Errorf("TailscaleOAuthClientID = %q, want %q", cred.TailscaleOAuthClientID, testClientID)
	}
}

func TestWriteCredential_keeps_the_mode_0600_of_an_existing_file(t *testing.T) {
	path := writeSecrets(t, twoTailnets)

	if err := WriteCredential(path, "jbones", Credential{TailscaleOAuthClientID: "TESTVALUE-second"}); err != nil {
		t.Fatalf("WriteCredential: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("the mode of %s is %04o, want 0600", path, perm)
	}
}

func TestWriteCredential_keeps_the_credential_of_the_other_tailnet(t *testing.T) {
	path := writeSecrets(t, twoTailnets)

	if err := WriteCredential(path, "jbones", Credential{TailscaleOAuthClientID: "TESTVALUE-second"}); err != nil {
		t.Fatalf("WriteCredential: %v", err)
	}

	corp, err := LoadCredential(path, "corp")
	if err != nil {
		t.Fatalf("LoadCredential: %v", err)
	}
	if corp.HeadscaleAPIKey != testAPIKey {
		t.Errorf("HeadscaleAPIKey = %q, want %q", corp.HeadscaleAPIKey, testAPIKey)
	}
}

func TestWriteCredential_rejects_an_empty_tailnet_identifier(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.yaml")

	if err := WriteCredential(path, "", Credential{}); err == nil {
		t.Fatal("WriteCredential returned no error, want an error")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("WriteCredential created %s, want no file", path)
	}
}

func TestNoLogLineHoldsACredentialValue(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	// A load that succeeds.
	good := writeSecrets(t, twoTailnets)
	if _, err := LoadCredential(good, "jbones"); err != nil {
		t.Fatalf("LoadCredential: %v", err)
	}

	// A load that fails on the mode, and a load that fails on the file body.
	wide := writeSecrets(t, twoTailnets)
	if err := os.Chmod(wide, 0644); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	modeErr := failedLoad(t, wide)
	broken := writeSecrets(t, "tailnets: [\n  "+testClientSecret+"\n")
	parseErr := failedLoad(t, broken)

	// A write that fails, because the directory does not exist.
	writeErr := WriteCredential(filepath.Join(t.TempDir(), "absent", "secrets.yaml"), "jbones",
		Credential{TailscaleOAuthClientSecret: testClientSecret})
	if writeErr == nil {
		t.Fatal("WriteCredential returned no error, want an error")
	}

	for _, value := range []string{testClientID, testClientSecret, testAPIKey} {
		if strings.Contains(buf.String(), value) {
			t.Errorf("a log line holds a credential value: %s", buf.String())
		}
		for _, err := range []error{modeErr, parseErr, writeErr} {
			if strings.Contains(err.Error(), value) {
				t.Errorf("an error holds a credential value: %v", err)
			}
		}
	}
}

// failedLoad returns the error of a load that must fail.
func failedLoad(t *testing.T, path string) error {
	t.Helper()
	_, err := LoadCredential(path, "jbones")
	if err == nil {
		t.Fatalf("LoadCredential(%s) returned no error, want an error", path)
	}
	return err
}
