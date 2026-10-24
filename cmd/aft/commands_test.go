package main

import (
	"slices"
	"testing"

	"github.com/arocomputer/aft/internal/config"
)

// TestExpandHome maps a home-relative server path to an absolute one. The Mac
// and the server have different home directories, so a stored ~ would break.
func TestExpandHome(t *testing.T) {
	cases := map[string]string{
		"~":            "/Users/fschr",
		"~/code/app":   "/Users/fschr/code/app",
		"/srv/app":     "/srv/app",
		"relative/dir": "relative/dir",
		"~tmp":         "~tmp",
	}
	for in, want := range cases {
		if got := expandHome(in, "/Users/fschr"); got != want {
			t.Errorf("expandHome(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestSuggest points an unknown word at the command meant: a flag form of a
// command, a near spelling, a retired name, or nothing.
func TestSuggest(t *testing.T) {
	for in, want := range map[string]string{"--help": "help", "-h": "help", "--version": "version", "lst": "ls", "atach": "attach", "end": "kill", "shell": "HOST", "xyz": ""} {
		if got := suggest(in); got != want {
			t.Errorf("suggest(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestKeepDefault writes down a lone host as the default before a second is
// added, so adding one never leaves aft without a default.
func TestKeepDefault(t *testing.T) {
	cfg := config.Config{Hosts: map[string]config.HostConfig{"server": {SSH: "server"}}}
	keepDefault(&cfg)
	cfg.Hosts["mac"] = config.HostConfig{SSH: "mac"}
	if cfg.UsedHost() != "server" {
		t.Fatalf("default is %q, want server", cfg.UsedHost())
	}
}

// TestNameFor names a host after the machine in its SSH destination, and never
// after an aft command.
func TestNameFor(t *testing.T) {
	for dest, want := range map[string]string{"box": "box", "-p 2222 fschr@box.example.com": "box", "user@10.0.0.7": "10-0-0-7", "ls.example.com": "ls-example-com"} {
		if got := nameFor(dest); got != want {
			t.Errorf("nameFor(%q) = %q, want %q", dest, got, want)
		}
	}
}

// TestCompletions offers commands and hosts first, session names only where a
// name goes, and a command's own words after it.
func TestCompletions(t *testing.T) {
	cfg := config.Config{Hosts: map[string]config.HostConfig{"server": {SSH: "server"}}}
	asked := false
	sessions := func() []string { asked = true; return []string{"fox"} }
	first := completions(cfg, nil, sessions)
	for _, w := range []string{"attach", "rename", "server"} {
		if !slices.Contains(first, w) {
			t.Errorf("first word lacks %q: %v", w, first)
		}
	}
	if slices.Contains(first, "HOST") || asked {
		t.Errorf("first word: %v (sessions asked: %v)", first, asked)
	}
	if got := completions(cfg, []string{"attach"}, sessions); !slices.Equal(got, []string{"fox"}) || !asked {
		t.Errorf("after attach: %v", got)
	}
	if got := completions(cfg, []string{"rename", "fox"}, sessions); got != nil {
		t.Errorf("a new name is not completed: %v", got)
	}
	if got := completions(cfg, []string{"host", "rm"}, sessions); !slices.Equal(got, []string{"server"}) {
		t.Errorf("after host rm: %v", got)
	}
}
