package namespaces

import "testing"

func TestGetNamespaceName(t *testing.T) {
	tests := []struct {
		tailnetID string
		want      string
	}{
		{"team-prod", "ns-team-prod"},
		{"devops", "ns-devops"},
		{"tls.test.example.com", "ns-tls.test.example.com"},
	}

	for _, tt := range tests {
		if got := GetNamespaceName(tt.tailnetID); got != tt.want {
			t.Errorf("GetNamespaceName(%q) = %q, want %q", tt.tailnetID, got, tt.want)
		}
	}
}

func TestGetTailnetFromNamespace(t *testing.T) {
	tests := []struct {
		namespaceName string
		want          string
	}{
		{"ns-team-prod", "team-prod"},
		{"ns-devops", "devops"},
		{"ns-tls.test.example.com", "tls.test.example.com"},
		{"default", ""},
		{"ns-", ""},
		{"invalid", ""},
	}

	for _, tt := range tests {
		if got := GetTailnetFromNamespace(tt.namespaceName); got != tt.want {
			t.Errorf("GetTailnetFromNamespace(%q) = %q, want %q", tt.namespaceName, got, tt.want)
		}
	}
}

// Note: CreateNamespace and DeleteNamespace tests would require root privileges
// or container environment, so they're omitted for simplicity in unit testing
func TestCreateNamespace(t *testing.T) {
	// Skip actual namespace creation in unit tests
	t.Skip("Skipping namespace creation test - requires privileges")
}

func TestDeleteNamespace(t *testing.T) {
	// Skip actual namespace deletion in unit tests
	t.Skip("Skipping namespace deletion test - requires privileges")
}
