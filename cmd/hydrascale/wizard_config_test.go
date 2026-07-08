package main

import "testing"

func TestBuildConfig(t *testing.T) {
	cfg := buildConfig([]tailnetAnswers{
		{ID: "personal", HostAccess: true, UseKey: true},
		{ID: "work", HostAccess: false, ExitNode: "exit.tail1234.ts.net"},
	}, false)

	if cfg.Version != 2 {
		t.Errorf("Version = %d, want 2", cfg.Version)
	}
	if cfg.Resolver.Mode != "unified" || cfg.Resolver.BindAddress != "127.0.0.53:5354" {
		t.Errorf("Resolver = %+v", cfg.Resolver)
	}
	if cfg.Reconciler.RawInterval != "10s" {
		t.Errorf("interval = %q, want 10s", cfg.Reconciler.RawInterval)
	}
	if len(cfg.Tailnets) != 2 {
		t.Fatalf("tailnets = %d, want 2", len(cfg.Tailnets))
	}

	// personal: host_access differs from global(false) -> explicit override true.
	if cfg.Tailnets[0].HostAccess == nil || !*cfg.Tailnets[0].HostAccess {
		t.Errorf("personal host_access override = %v, want true", cfg.Tailnets[0].HostAccess)
	}
	// keys never land in the config.
	if cfg.Tailnets[0].AuthKey != "" {
		t.Errorf("personal AuthKey = %q, want empty (env var)", cfg.Tailnets[0].AuthKey)
	}
	// work: matches global(false) -> no override; exit node carried through.
	if cfg.Tailnets[1].HostAccess != nil {
		t.Errorf("work host_access override = %v, want nil", cfg.Tailnets[1].HostAccess)
	}
	if cfg.Tailnets[1].ExitNode != "exit.tail1234.ts.net" {
		t.Errorf("work ExitNode = %q", cfg.Tailnets[1].ExitNode)
	}
}
