package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// writeRouteTableConfig writes a configuration file that declares one tailnet and the route
// table, and it returns the path.
func writeRouteTableConfig(t *testing.T, table string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := "version: 2\nroute_table: " + table + "\ntailnets:\n  - id: havoc\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write the configuration file: %v", err)
	}
	return path
}

func TestTheConfigurationFileCarriesTheRouteTable(t *testing.T) {
	cfg, err := LoadConfig(writeRouteTableConfig(t, "53"))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.RouteTable != 53 {
		t.Errorf("route_table is %d, want 53", cfg.RouteTable)
	}
}

func TestAFileWithNoRouteTableLeavesTheFieldAtZero(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("version: 2\ntailnets:\n  - id: havoc\n"), 0644); err != nil {
		t.Fatalf("write the configuration file: %v", err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.RouteTable != 0 {
		t.Errorf("route_table is %d, want 0, which selects the main table", cfg.RouteTable)
	}
}

func TestTheDefaultConfigurationDeclaresNoRouteTable(t *testing.T) {
	// An operator who runs `hydrascale init` keeps the behaviour of version 0.9.
	if got := DefaultConfig().RouteTable; got != 0 {
		t.Errorf("DefaultConfig route_table is %d, want 0", got)
	}
}

func TestLoadConfigRejectsAReservedRouteTable(t *testing.T) {
	for _, table := range []int{RouteTableDefault, RouteTableMain, RouteTableLocal} {
		name := strconv.Itoa(table)
		t.Run(name, func(t *testing.T) {
			_, err := LoadConfig(writeRouteTableConfig(t, name))
			if err == nil {
				t.Fatalf("LoadConfig accepted the reserved route table %s", name)
			}
			if !strings.Contains(err.Error(), "reserve") {
				t.Errorf("the error %q states no reason", err)
			}
		})
	}
}

func TestValidateRouteTableAcceptsAndRejects(t *testing.T) {
	tests := []struct {
		name    string
		table   int
		wantErr bool
	}{
		{name: "zero selects the main table", table: 0},
		{name: "the suggested table", table: 53},
		{name: "the table of tailscaled is free on the host", table: 52},
		{name: "the largest table", table: int(MaxRouteTable)},
		{name: "a negative number", table: -1, wantErr: true},
		{name: "one above the largest table", table: int(MaxRouteTable) + 1, wantErr: true},
		{name: "the default table", table: RouteTableDefault, wantErr: true},
		{name: "the main table", table: RouteTableMain, wantErr: true},
		{name: "the local table", table: RouteTableLocal, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRouteTable(tt.table)
			if tt.wantErr && err == nil {
				t.Errorf("ValidateRouteTable(%d) returned no error, want an error", tt.table)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ValidateRouteTable(%d) returned %v, want no error", tt.table, err)
			}
		})
	}
}
