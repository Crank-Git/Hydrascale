package main

import "hydrascale/internal/config"

// tailnetAnswers holds the wizard responses for a single tailnet.
type tailnetAnswers struct {
	ID         string
	HostAccess bool
	ExitNode   string
	UseKey     bool // key vs. browser auth; affects the auth flow, not the config
}

// buildConfig turns wizard answers into a Config. Auth keys are never written
// to the config (they go to the HYDRASCALE_AUTHKEY_<ID> env var); a per-tailnet
// host_access override is only emitted when it differs from the global default.
func buildConfig(ans []tailnetAnswers, globalHostAccess bool) *config.Config {
	cfg := &config.Config{
		Version:    2,
		HostAccess: globalHostAccess,
		Resolver:   config.ResolverConfig{Mode: "unified", BindAddress: "127.0.0.53:5354"},
		Reconciler: config.ReconcilerConfig{RawInterval: "10s"},
	}
	for _, a := range ans {
		tn := config.Tailnet{ID: a.ID, ExitNode: a.ExitNode}
		if a.HostAccess != globalHostAccess {
			v := a.HostAccess
			tn.HostAccess = &v
		}
		cfg.Tailnets = append(cfg.Tailnets, tn)
	}
	return cfg
}
