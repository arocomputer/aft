package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/arocomputer/aft/internal/config"
	"github.com/arocomputer/aft/internal/home"
	"github.com/arocomputer/aft/internal/session"
)

// TestSetupDiscoveryUsesTheExecutionHost avoids selecting a local-only command
// for a remote project or treating a leftover config directory as an executable.
func TestSetupDiscoveryUsesTheExecutionHost(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("HOME", directory)
	t.Setenv("AFT_HOME", filepath.Join(directory, "aft"))
	bin := filepath.Join(directory, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(directory, ".gemini"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	info := session.Info{Agents: map[string]string{"codex": "/bin/codex"}}
	options := setupProgramOptions([]string{"claude", "codex", "gemini"}, nil, nil, info, "box", true)
	if options[0].Checked || !options[1].Checked || options[2].Checked {
		t.Fatalf("remote defaults: %+v", options)
	}
	if !strings.Contains(options[0].Hint, "this device") || !strings.Contains(options[1].Hint, "box") || !strings.Contains(options[2].Hint, "configuration found") {
		t.Fatalf("discovery hints: %+v", options)
	}
	local := setupProgramOptions([]string{"claude"}, nil, nil, session.Info{}, "", true)
	if !local[0].Checked {
		t.Fatal("local setup did not preselect an installed command")
	}
	kept := setupProgramOptions([]string{"claude"}, []string{"custom-agent"}, nil, session.Info{}, "", false)
	if kept[0].Checked || !kept[1].Checked {
		t.Fatalf("saved choices were replaced by discovery: %+v", kept)
	}
}

// TestSetupYesDetectsAndRemembersEmptyChoices protects scripted setup from
// installing the catalog and from resurrecting deliberately removed launchers.
func TestSetupYesDetectsAndRemembersEmptyChoices(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("HOME", directory)
	t.Setenv("AFT_HOME", filepath.Join(directory, "aft"))
	bin := filepath.Join(directory, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"claude", "git"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	manual := false
	if err := config.Save(config.Config{EditDotfiles: &manual}); err != nil {
		t.Fatal(err)
	}
	captureCLI(t, func() {
		if code := cmdSetup([]string{"--yes"}); code != 0 {
			t.Fatalf("setup returned %d", code)
		}
	})
	cfg, err := config.Load()
	if err != nil || !reflect.DeepEqual(cfg.Agents, []string{"claude"}) || !reflect.DeepEqual(cfg.Tools, []string{"git"}) {
		t.Fatalf("detected config: %+v, %v", cfg, err)
	}
	cfg.Agents, cfg.Tools = []string{}, []string{}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	captureCLI(t, func() {
		if code := cmdSetup([]string{"--yes"}); code != 0 {
			t.Fatalf("setup returned %d", code)
		}
	})
	cfg, err = config.Load()
	if err != nil || len(cfg.Launched()) != 0 {
		t.Fatalf("empty choices were reset: %+v, %v", cfg, err)
	}
	for _, name := range []string{"claude", "git"} {
		if _, err := os.Lstat(filepath.Join(home.Launchers(), name)); !os.IsNotExist(err) {
			t.Fatalf("deselected %s launcher remains: %v", name, err)
		}
	}
}
