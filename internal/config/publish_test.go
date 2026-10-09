package config

import (
	"strings"
	"testing"
)

// publishFile returns a configuration file that declares the tailnet alpha with host
// access on, the publish list of alpha, and the access block.
func publishFile(publish, accessBlock string) string {
	return `
version: 2
tailnets:
  - id: "alpha"
    host_access: true
    publish: ` + publish + `
  - id: "beta"
resolver:
  mode: "unified"
` + accessBlock
}

func TestLoadConfigPublish(t *testing.T) {
	t.Run("loads a published port that a local rule to the host covers", func(t *testing.T) {
		tmp := writeTemp(t, publishFile(`["tcp/22", "udp/53"]`, `
access:
  rules:
    - from: "alpha"
      to: "host"
`))
		cfg, err := LoadConfig(tmp)
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		got := cfg.Tailnets[0].Publish
		if len(got) != 2 || got[0] != "tcp/22" || got[1] != "udp/53" {
			t.Errorf("Publish = %v, want [tcp/22 udp/53] in the order of the file", got)
		}
	})

	t.Run("loads a published port inside the port range of a local rule", func(t *testing.T) {
		tmp := writeTemp(t, publishFile(`["tcp/22"]`, `
access:
  rules:
    - from: "alpha"
      to: "host"
      ports: ["tcp/20-30"]
`))
		if _, err := LoadConfig(tmp); err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
	})

	t.Run("loads a file with no publish key as no published port", func(t *testing.T) {
		tmp := writeTemp(t, `
version: 2
tailnets:
  - id: "alpha"
resolver:
  mode: "unified"
`)
		cfg, err := LoadConfig(tmp)
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if len(cfg.Tailnets[0].Publish) != 0 {
			t.Errorf("Publish = %v, want an empty list", cfg.Tailnets[0].Publish)
		}
	})

	t.Run("refuses a published port that no local rule covers and names the rule", func(t *testing.T) {
		tmp := writeTemp(t, publishFile(`["tcp/22"]`, `
access:
  rules:
    - from: "alpha"
      to: "host"
      ports: ["tcp/443"]
`))
		_, err := LoadConfig(tmp)
		if err == nil {
			t.Fatal("LoadConfig returned no error, want a refusal of the uncovered port")
		}
		for _, want := range []string{`"alpha"`, `"tcp/22"`, "from: alpha, to: host"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error = %q, want a message that holds %s", err, want)
			}
		}
	})

	t.Run("refuses a published port when the file holds no access block", func(t *testing.T) {
		tmp := writeTemp(t, publishFile(`["tcp/22"]`, ""))
		_, err := LoadConfig(tmp)
		if err == nil || !strings.Contains(err.Error(), "from: alpha, to: host") {
			t.Fatalf("LoadConfig error = %v, want a message that names the rule the file needs", err)
		}
	})

	t.Run("refuses a range", func(t *testing.T) {
		tmp := writeTemp(t, publishFile(`["tcp/22-23"]`, `
access:
  rules:
    - from: "alpha"
      to: "host"
`))
		_, err := LoadConfig(tmp)
		if err == nil || !strings.Contains(err.Error(), `"tcp/22-23"`) || !strings.Contains(err.Error(), `"alpha"`) {
			t.Fatalf("LoadConfig error = %v, want a refusal that names the tailnet and the range", err)
		}
	})

	t.Run("refuses an entry that is not a port", func(t *testing.T) {
		tmp := writeTemp(t, publishFile(`["ssh"]`, `
access:
  rules:
    - from: "alpha"
      to: "host"
`))
		_, err := LoadConfig(tmp)
		if err == nil || !strings.Contains(err.Error(), `"ssh"`) || !strings.Contains(err.Error(), `"alpha"`) {
			t.Fatalf("LoadConfig error = %v, want a refusal that names the tailnet and the entry", err)
		}
	})

	t.Run("refuses a duplicate entry in one tailnet", func(t *testing.T) {
		tmp := writeTemp(t, publishFile(`["tcp/22", "tcp/22"]`, `
access:
  rules:
    - from: "alpha"
      to: "host"
`))
		_, err := LoadConfig(tmp)
		if err == nil || !strings.Contains(err.Error(), "duplicate") || !strings.Contains(err.Error(), `"tcp/22"`) {
			t.Fatalf("LoadConfig error = %v, want a refusal of the duplicate entry", err)
		}
	})

	t.Run("refuses a publish list on a tailnet with host access off", func(t *testing.T) {
		tmp := writeTemp(t, `
version: 2
tailnets:
  - id: "alpha"
    host_access: false
    publish: ["tcp/22"]
resolver:
  mode: "unified"
access:
  rules:
    - from: "alpha"
      to: "host"
`)
		_, err := LoadConfig(tmp)
		if err == nil || !strings.Contains(err.Error(), `"alpha"`) || !strings.Contains(err.Error(), "host access") {
			t.Fatalf("LoadConfig error = %v, want a refusal that names the tailnet and host access", err)
		}
	})

	t.Run("accepts the same port on two tailnets", func(t *testing.T) {
		tmp := writeTemp(t, `
version: 2
host_access: true
tailnets:
  - id: "alpha"
    publish: ["tcp/22"]
  - id: "beta"
    publish: ["tcp/22"]
resolver:
  mode: "unified"
access:
  rules:
    - from: "alpha"
      to: "host"
    - from: "beta"
      to: "host"
      ports: ["tcp/22"]
`)
		if _, err := LoadConfig(tmp); err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
	})
}

func TestValidatePublishReportsEveryFailureTogether(t *testing.T) {
	tailnets := []Tailnet{
		{ID: "alpha", Publish: []string{"tcp/22-23", "udp/53", "udp/53", "tcp/80"}},
		{ID: "beta", Publish: []string{"tcp/22"}},
	}
	hostAccess := func(id string) bool { return id == "alpha" }

	err := ValidatePublish(tailnets, hostAccess, nil)
	if err == nil {
		t.Fatal("ValidatePublish returned no error")
	}
	for _, want := range []string{`"tcp/22-23"`, "duplicate", `"tcp/80"`, `"beta"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want a message that holds %s", err, want)
		}
	}
}
