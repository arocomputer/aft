package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arocomputer/aft/internal/config"
	"github.com/arocomputer/aft/internal/home"
)

// TestSetupPreviewIsolatesWritesAndSSH runs the real wizard without exposing
// the caller's shell files, configuration or remote hosts to setup changes.
func TestSetupPreviewIsolatesWritesAndSSH(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("AFT_HOME", filepath.Join(root, "state"))
	if err := config.Save(config.Config{Agents: []string{}}); err != nil {
		t.Fatal(err)
	}
	profile := filepath.Join(root, ".zshrc")
	if err := os.WriteFile(profile, []byte("# preserve shell configuration\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(config.Path())
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "ssh-used")
	t.Setenv("AFT_TEST_SSH_MARKER", marker)
	fakeSSH(t, `touch "$AFT_TEST_SSH_MARKER"; exit 99`)
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, remote := range []bool{false, true} {
		args := []string{"selftest", "setup", "--yes"}
		if remote {
			args = append(args, "--host", "test-destination")
		}
		command := exec.Command(self, args...)
		command.Env = append(os.Environ(), "AFT_TEST_AS_AFT=1")
		output, runErr := command.CombinedOutput()
		if (runErr != nil) != remote {
			t.Fatalf("preview remote=%v: %v, %s", remote, runErr, output)
		}
		if remote && !strings.Contains(string(output), "SSH disabled in setup preview") {
			t.Fatalf("unexpected remote error: %s", output)
		}
		line := strings.SplitN(string(output), "\n", 2)[0]
		directory := strings.TrimPrefix(line, "Preview home: ")
		if directory == line {
			t.Fatalf("missing preview directory: %s", output)
		}
		if _, err := os.Stat(directory); !os.IsNotExist(err) {
			t.Fatalf("preview home remains: %s (%v)", directory, err)
		}
	}
	current, err := os.ReadFile(config.Path())
	if err != nil || string(current) != string(original) {
		t.Fatal("preview changed real configuration")
	}
	current, err = os.ReadFile(profile)
	if err != nil || string(current) != "# preserve shell configuration\n" {
		t.Fatal("preview changed real shell configuration")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("preview invoked the caller's SSH client")
	}
}

// TestLocalChecksRestoreCallerState protects the lab's environment and cwd boundary.
func TestLocalChecksRestoreCallerState(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AFT_HOME", root)
	t.Setenv("AFT_REMOTE_HOME", "previous-remote-home")
	if err := config.Save(config.Config{Agents: []string{}, Hosts: map[string]config.HostConfig{"test": {SSH: "test-destination"}}}); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(config.Path())
	if err != nil {
		t.Fatal(err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	output := captureCLI(t, func() {
		if code := cmdSelftest([]string{"cli"}); code != 0 {
			t.Fatalf("CLI check returned %d", code)
		}
	})
	current, err := os.Getwd()
	if err != nil || current != previous {
		t.Fatal("working directory was not restored")
	}
	if os.Getenv("AFT_HOME") != root || os.Getenv("AFT_REMOTE_HOME") != "previous-remote-home" {
		t.Fatal("environment was not restored")
	}
	data, err := os.ReadFile(config.Path())
	if err != nil || string(data) != string(original) {
		t.Fatal("CLI check changed the caller's configuration")
	}
	if strings.Contains(output, "\x1b[") || !strings.Contains(output, "$ aft doctor\nok") {
		t.Fatalf("unexpected CLI preview: %q", output)
	}
}

// TestProjectsCheckExercisesOnAndOff runs project checks through the CLI entry.
func TestProjectsCheckExercisesOnAndOff(t *testing.T) {
	t.Setenv("AFT_HOME", t.TempDir())
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(self, "selftest", "projects")
	command.Env = append(os.Environ(), "AFT_TEST_AS_AFT=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("projects check: %v, %s", err, output)
	}
	if strings.Contains(string(output), "warning:") {
		t.Fatalf("shell-only project should not require launcher PATH: %s", output)
	}
	if !strings.Contains(string(output), "Subfolder matching: ok") {
		t.Fatalf("project checks incomplete: %s", output)
	}
}

// TestSelftestPathSkipsProductionShims avoids detecting Aft's wrappers as agents.
func TestSelftestPathSkipsProductionShims(t *testing.T) {
	t.Setenv("AFT_HOME", t.TempDir())
	if err := os.MkdirAll(home.Launchers(), 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(home.Launchers(), alias); err != nil {
		t.Fatal(err)
	}
	path := strings.Join([]string{alias, "/usr/bin", home.Launchers()}, string(os.PathListSeparator))
	if actual := selftestPath(path, home.Launchers()); actual != "/usr/bin" {
		t.Fatalf("production shims remain on PATH: %q", actual)
	}
}
