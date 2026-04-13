package namespaces

import (
	"strings"
	"testing"
)

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

func TestVethIPs(t *testing.T) {
	tests := []struct {
		name        string
		subnet      string
		index       int
		wantHostIP  string
		wantNsIP    string
		wantHostGW  string
		wantNsGW    string
		wantErrFrag string
	}{
		{
			name:       "default subnet index 1",
			subnet:     "10.200.0.0/16",
			index:      1,
			wantHostIP: "10.200.0.1/30",
			wantNsIP:   "10.200.0.2/30",
			wantHostGW: "10.200.0.1",
			wantNsGW:   "10.200.0.2",
		},
		{
			name:       "default subnet index 2",
			subnet:     "10.200.0.0/16",
			index:      2,
			wantHostIP: "10.200.0.5/30",
			wantNsIP:   "10.200.0.6/30",
			wantHostGW: "10.200.0.5",
			wantNsGW:   "10.200.0.6",
		},
		{
			name:       "custom subnet index 1",
			subnet:     "192.168.50.0/16",
			index:      1,
			wantHostIP: "192.168.0.1/30",
			wantNsIP:   "192.168.0.2/30",
			wantHostGW: "192.168.0.1",
			wantNsGW:   "192.168.0.2",
		},
		{
			name:       "max index 254",
			subnet:     "10.200.0.0/16",
			index:      254,
			wantHostIP: "10.200.3.245/30",
			wantNsIP:   "10.200.3.246/30",
			wantHostGW: "10.200.3.245",
			wantNsGW:   "10.200.3.246",
		},
		{
			name:        "invalid subnet",
			subnet:      "not-a-cidr",
			index:       1,
			wantErrFrag: "invalid CIDR",
		},
		{
			name:        "index out of range for small subnet",
			subnet:      "10.50.0.0/24",
			index:       200,
			wantErrFrag: "out of range",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hostIP, nsIP, hostGW, nsGW, err := VethIPs(tt.subnet, tt.index)
			if tt.wantErrFrag != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErrFrag)
				}
				if !strings.Contains(err.Error(), tt.wantErrFrag) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.wantErrFrag)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if hostIP != tt.wantHostIP {
				t.Errorf("hostIP = %q, want %q", hostIP, tt.wantHostIP)
			}
			if nsIP != tt.wantNsIP {
				t.Errorf("nsIP = %q, want %q", nsIP, tt.wantNsIP)
			}
			if hostGW != tt.wantHostGW {
				t.Errorf("hostGW = %q, want %q", hostGW, tt.wantHostGW)
			}
			if nsGW != tt.wantNsGW {
				t.Errorf("nsGW = %q, want %q", nsGW, tt.wantNsGW)
			}
		})
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
