package policy

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hydrascale/internal/secrets"
)

// The test values below are not credentials. Each one is a fixed string that a test
// searches for in an error or in a log line.
const (
	testClientID     = "TESTVALUE-client-id-154"
	testClientSecret = "TESTVALUE-client-secret-154"
	testAPIKey       = "TESTVALUE-api-key-154"
	fromTheEnv       = "TESTVALUE-from-the-environment"
)

const twoTailnets = `tailnets:
  jbones:
    tailscale_oauth_client_id: "` + testClientID + `"
    tailscale_oauth_client_secret: "` + testClientSecret + `"
  corp:
    headscale_api_key: "` + testAPIKey + `"
    headscale_address: "https://headscale.example.net"
`

// writeSecrets writes a secrets file at mode 0600 in a new directory and returns its path.
func writeSecrets(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "secrets.yaml")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

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
	if cred != (secrets.Tailnet{}) {
		t.Errorf("LoadCredential returned %+v, want an empty credential", cred)
	}
}

func TestLoadCredential_accepts_a_missing_secrets_file(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.yaml")

	cred, err := LoadCredential(path, "jbones")
	if err != nil {
		t.Fatalf("LoadCredential returned %v, want no error", err)
	}
	if cred != (secrets.Tailnet{}) {
		t.Errorf("LoadCredential returned %+v, want an empty credential", cred)
	}
}

// TestLoadCredential_returns_the_refusal_of_the_secrets_package proves that the override
// layer adds no second reader. internal/secrets holds the mode rule and the regular file
// rule, and TestLoad_refuses_a_file_that_grants_group_access and
// TestLoad_refuses_a_symbolic_link assert them.
func TestLoadCredential_returns_the_refusal_of_the_secrets_package(t *testing.T) {
	path := writeSecrets(t, twoTailnets)
	if err := os.Chmod(path, 0640); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	// The variable is set, and the refusal of the file still wins.
	t.Setenv(TailscaleClientIDEnvVar("jbones"), fromTheEnv)

	if _, err := LoadCredential(path, "jbones"); err == nil {
		t.Fatal("LoadCredential returned no error, want the refusal of secrets.Load")
	}
}

func TestLoadCredential_takes_the_environment_variable_over_the_file_value(t *testing.T) {
	path := writeSecrets(t, twoTailnets)

	cases := []struct {
		name    string
		tailnet string
		envVar  string
		field   func(secrets.Tailnet) string
	}{
		{"the Tailscale client identifier", "jbones", TailscaleClientIDEnvVar("jbones"), func(c secrets.Tailnet) string { return c.TailscaleOAuthClientID }},
		{"the Tailscale client secret", "jbones", TailscaleClientSecretEnvVar("jbones"), func(c secrets.Tailnet) string { return c.TailscaleOAuthClientSecret }},
		{"the Headscale API key", "corp", HeadscaleAPIKeyEnvVar("corp"), func(c secrets.Tailnet) string { return c.HeadscaleAPIKey }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.envVar, fromTheEnv)
			cred, err := LoadCredential(path, tc.tailnet)
			if err != nil {
				t.Fatalf("LoadCredential: %v", err)
			}
			if got := tc.field(cred); got != fromTheEnv {
				t.Errorf("%s = %q, want %q", tc.envVar, got, fromTheEnv)
			}
		})
	}
}

func TestLoadCredential_keeps_the_other_file_values_when_one_variable_overrides(t *testing.T) {
	path := writeSecrets(t, twoTailnets)
	t.Setenv(TailscaleClientIDEnvVar("jbones"), fromTheEnv)

	cred, err := LoadCredential(path, "jbones")
	if err != nil {
		t.Fatalf("LoadCredential: %v", err)
	}
	if cred.TailscaleOAuthClientSecret != testClientSecret {
		t.Errorf("TailscaleOAuthClientSecret = %q, want the file value", cred.TailscaleOAuthClientSecret)
	}
}

func TestLoadCredential_writes_no_variable_back_to_the_secrets_file(t *testing.T) {
	path := writeSecrets(t, twoTailnets)
	t.Setenv(TailscaleClientIDEnvVar("jbones"), fromTheEnv)

	if _, err := LoadCredential(path, "jbones"); err != nil {
		t.Fatalf("LoadCredential: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != twoTailnets {
		t.Errorf("LoadCredential changed the secrets file: %s", data)
	}
}

func TestLoadCredential_takes_the_environment_variable_when_the_file_is_absent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.yaml")
	t.Setenv(HeadscaleAPIKeyEnvVar("corp"), fromTheEnv)

	cred, err := LoadCredential(path, "corp")
	if err != nil {
		t.Fatalf("LoadCredential: %v", err)
	}
	if cred.HeadscaleAPIKey != fromTheEnv {
		t.Errorf("HeadscaleAPIKey = %q, want %q", cred.HeadscaleAPIKey, fromTheEnv)
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

	for _, value := range []string{testClientID, testClientSecret, testAPIKey} {
		if strings.Contains(buf.String(), value) {
			t.Errorf("a log line holds a credential value: %s", buf.String())
		}
		for _, err := range []error{modeErr, parseErr} {
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

func TestTailscaleSecretProblemNamesADeviceAuthenticationKey(t *testing.T) {
	// Issue #276. An operator wrote such a key on 2026-08-13. The control server answered
	// the token request with HTTP 401 and the message "API token invalid", which names
	// neither the mistake nor the value.
	got := TailscaleSecretProblem("tskey-auth-kXXXXXXCNTRL-abcdefghijklmnop")

	if got == "" {
		t.Fatal("TailscaleSecretProblem accepted a device authentication key")
	}
	if !strings.Contains(got, "device authentication key") {
		t.Errorf("the message %q names no device authentication key", got)
	}
	if !strings.Contains(got, TailscaleClientSecretPrefix) {
		t.Errorf("the message %q names no expected prefix", got)
	}
}

func TestTailscaleSecretProblemAcceptsAnOAuthClientSecret(t *testing.T) {
	if got := TailscaleSecretProblem("tskey-client-kXXXXXXCNTRL-abcdefghijklmnop"); got != "" {
		t.Errorf("TailscaleSecretProblem returned %q for an OAuth client secret", got)
	}
}

func TestTailscaleSecretProblemStatesNothingForAnEmptySecret(t *testing.T) {
	// A tailnet that holds no credential is a separate state, which missingCredential names.
	if got := TailscaleSecretProblem(""); got != "" {
		t.Errorf("TailscaleSecretProblem returned %q for an empty secret", got)
	}
}

func TestTailscaleSecretProblemNamesNoPartOfTheValue(t *testing.T) {
	// FR-policy-4. The message reaches the control API, therefore it carries no part of
	// the credential.
	const secret = "tskey-auth-kSECRETVALUE-shouldneverappear"
	got := TailscaleSecretProblem(secret)

	if strings.Contains(got, "kSECRETVALUE") || strings.Contains(got, "shouldneverappear") {
		t.Errorf("the message %q holds part of the credential", got)
	}
}

func TestTailscaleSecretProblemRejectsAValueOfAnotherShape(t *testing.T) {
	got := TailscaleSecretProblem("not-a-tailscale-value")

	if got == "" {
		t.Fatal("TailscaleSecretProblem accepted a value of another shape")
	}
	if !strings.Contains(got, TailscaleClientSecretPrefix) {
		t.Errorf("the message %q names no expected prefix", got)
	}
}
