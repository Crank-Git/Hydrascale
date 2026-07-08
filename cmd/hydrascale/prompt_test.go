package main

import (
	"io"
	"strings"
	"testing"
)

func TestPromptLineDefault(t *testing.T) {
	p := newPrompter(strings.NewReader("\n"), io.Discard)
	if got := p.line("id", "personal"); got != "personal" {
		t.Fatalf("line empty = %q, want personal (default)", got)
	}
	p2 := newPrompter(strings.NewReader("  work \n"), io.Discard)
	if got := p2.line("id", "personal"); got != "work" {
		t.Fatalf("line = %q, want work (trimmed)", got)
	}
}

func TestPromptYesNo(t *testing.T) {
	cases := []struct {
		in   string
		def  bool
		want bool
	}{
		{"y\n", false, true},
		{"YES\n", false, true},
		{"n\n", true, false},
		{"no\n", true, false},
		{"\n", true, true},
		{"\n", false, false},
		{"huh\n", true, true},
	}
	for _, c := range cases {
		p := newPrompter(strings.NewReader(c.in), io.Discard)
		if got := p.yesNo("ok?", c.def); got != c.want {
			t.Errorf("yesNo(%q, def=%v) = %v, want %v", c.in, c.def, got, c.want)
		}
	}
}

func TestPromptSecretTrims(t *testing.T) {
	p := newPrompter(strings.NewReader(""), io.Discard)
	p.secretFn = func() (string, error) { return "tskey-abc  ", nil }
	got, err := p.secret("auth key")
	if err != nil || got != "tskey-abc" {
		t.Fatalf("secret = %q err=%v, want tskey-abc", got, err)
	}
}
