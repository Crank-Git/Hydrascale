// Package config provides configuration management for HydraScale CLI service.
package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Tailnet represents a single Tailscale tailnet configuration.
type Tailnet struct {
	ID       string `yaml:"id"`
	ExitNode string `yaml:"exit_node,omitempty"` // optional
}

// Config represents the HydraScale service configuration.
type Config struct {
	Tailnets []Tailnet `yaml:"tailnets"`
	Resolver Resolver  `yaml:"resolver"`
}

// Resolver represents DNS resolver configuration.
type Resolver struct {
	Mode string `yaml:"mode"` // "unified" for this release
}

// LoadConfig reads and parses a YAML configuration file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// LoadConfigFromFlag reads and parses a YAML configuration file from CLI flag path.
func LoadConfigFromFlag(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// DefaultConfig returns a default configuration.
func DefaultConfig() *Config {
	return &Config{
		Tailnets: []Tailnet{},
		Resolver: Resolver{
			Mode: "unified",
		},
	}
}
