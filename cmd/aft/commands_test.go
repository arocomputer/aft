package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/fschrhunt/aft/internal/bridge/transport"
	"github.com/fschrhunt/aft/internal/config"
	"github.com/fschrhunt/aft/internal/session"
)

// fakeSSH puts a test's SSH implementation on PATH without reaching a host.
func fakeSSH(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte("#!/bin/sh\n"+script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestLegacyServerListsOnlyAgents preserves agent access during upgrades without
// exposing shells whose current directory an older server cannot report.
func TestLegacyServerListsOnlyAgents(t *testing.T) {
	t.Setenv("AFT_HOME", t.TempDir())
	records, err := json.Marshal([]session.Record{
		{ID: "agent", Command: []string{"agent"}},
		{ID: "shell", Command: session.ShellCommand},
	})
	if err != nil {
		t.Fatal(err)
	}
	fakeSSH(t, `case "$*" in
  *ls-projects*) printf '%s\n' 'unknown server command "ls-projects"' >&2; exit 2 ;;
  *'ls --all'*) cat <<'RECORDS'
`+string(records)+`
RECORDS
    ;;
  *) exit 99 ;;
esac
`)
	got, err := projectSessions(transport.Host{Name: "box", SSH: "test-destination"}, config.State{})
	if err != nil || len(got) != 1 || got[0].ID != "agent" {
		t.Fatalf("legacy listing: %+v, %v", got, err)
	}
}

// TestHostAttachAuthenticatesBeforeListing lets SSH prompt before querying
// visibility, and never contacts unrelated hosts when --host is specified.
func TestHostAttachAuthenticatesBeforeListing(t *testing.T) {
	t.Setenv("AFT_HOME", t.TempDir())
	connected := filepath.Join(t.TempDir(), "connected")
	t.Setenv("AFT_TEST_CONNECTED", connected)
	if err := config.Save(config.Config{Hosts: map[string]config.HostConfig{
		"box": {SSH: "test-destination"}, "other": {SSH: "unrelated"},
	}}); err != nil {
		t.Fatal(err)
	}
	fakeSSH(t, `case " $* " in
  *unrelated*) exit 99 ;;
  *' -O check '*) test -f "$AFT_TEST_CONNECTED" ;;
  *' -fN '*)
    case "$*" in
      *BatchMode=yes*) printf '%s\n' 'Permission denied' >&2; exit 255 ;;
      *) : > "$AFT_TEST_CONNECTED" ;;
    esac ;;
  *ls-projects*)
    test -f "$AFT_TEST_CONNECTED" || exit 98
    printf '[]\n' ;;
  *) exit 97 ;;
esac
`)
	for _, args := range [][]string{{"--host", "box", "owl"}, {"--host", "box"}} {
		out := captureCLI(t, func() {
			if code := cmdAttach(args); code != 1 {
				t.Errorf("attach %v returned %d", args, code)
			}
		})
		if !strings.Contains(out, "no attachable session") && !strings.Contains(out, "no sessions to attach") {
			t.Fatalf("attach did not reach session selection: %s", out)
		}
	}
	if _, err := os.Stat(connected); err != nil {
		t.Fatalf("SSH never had an opportunity to authenticate: %v", err)
	}
}

// captureCLI collects the small help screens from both output streams.
func captureCLI(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, stderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = w, w
	defer func() { os.Stdout, os.Stderr = stdout, stderr }()
	fn()
	w.Close()
	out, err := io.ReadAll(r)
	r.Close()
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// TestHelpOverview keeps the default short and the full reference flat and readable.
func TestHelpOverview(t *testing.T) {
	compact := captureCLI(t, func() { printUsage(false) })
	full := captureCLI(t, func() { printUsage(true) })
	if len(strings.Split(compact, "\n")) > 15 || !strings.Contains(compact, "help") {
		t.Fatalf("overview should be compact and point to help:\n%s", compact)
	}
	if strings.Contains(compact, "rename") || !strings.Contains(full, "rename") || !strings.Contains(full, "tools add") {
		t.Fatalf("full help should expand the overview:\n%s", full)
	}
	if strings.Contains(full, "\nRun ") || strings.Contains(compact, "\nRun ") || strings.Contains(full, "your agents") || strings.Contains(full, "\x1b[") {
		t.Fatalf("help contains a footer, branding or styling: %q", full)
	}
	for _, line := range strings.Split(full, "\n") {
		if len(line) > 80 {
			t.Errorf("help exceeds 80 columns: %q", line)
		}
		if line != "" && !strings.HasPrefix(line, "  ") && !strings.ContainsAny(line, " :") {
			t.Errorf("unexpected category header: %q", line)
		}
	}
}

// TestHelpRequests accepts conventional help without loading or changing setup.
func TestHelpRequests(t *testing.T) {
	home := filepath.Join(t.TempDir(), "unused")
	t.Setenv("AFT_HOME", home)
	full := captureCLI(t, func() { printUsage(true) })
	for _, cmd := range []string{"help", "--help", "-h"} {
		out := captureCLI(t, func() {
			if code := run(cmd, nil); code != 0 {
				t.Errorf("%s returned %d", cmd, code)
			}
		})
		if out != full {
			t.Errorf("%s differs from full help", cmd)
		}
	}
	for _, cmd := range commands {
		name := strings.Fields(cmd[0])[0]
		if name == "HOST" {
			continue
		}
		captureCLI(t, func() {
			if code := run(name, []string{"--help"}); code != 0 {
				t.Errorf("%s --help returned %d", name, code)
			}
		})
	}
	for _, request := range []struct {
		command string
		args    []string
	}{
		{"help", []string{"on"}},
		{"help", []string{"mount", "add"}},
		{"tools", []string{"add", "--help"}},
		{"help", []string{"kill"}},
	} {
		captureCLI(t, func() {
			if code := run(request.command, request.args); code != 0 {
				t.Errorf("help %s %v returned %d", request.command, request.args, code)
			}
		})
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatalf("help touched AFT_HOME: %v", err)
	}
	captureCLI(t, func() {
		if code := run("on", []string{"--invalid"}); code != 2 {
			t.Errorf("invalid flag returned %d, want 2", code)
		}
	})
}

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

// TestHostNameLocalIsRefused keeps "local" for this machine.
func TestHostNameLocalIsRefused(t *testing.T) {
	if checkHostName("local") == nil {
		t.Fatal("local accepted as a host name")
	}
	if err := checkHostName("mini"); err != nil {
		t.Fatal(err)
	}
}

// TestShellOnlyDoctorIsQuiet avoids requiring launcher PATH edits for a setup
// that deliberately has no agent/tool launchers, while keeping full JSON detail.
func TestShellOnlyDoctorIsQuiet(t *testing.T) {
	t.Setenv("AFT_HOME", t.TempDir())
	fakeSSH(t, "exit 0")
	if err := config.Save(config.Config{Agents: []string{}}); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveState(config.State{Projects: map[string]config.Project{"/test/project": {Host: "local"}}}); err != nil {
		t.Fatal(err)
	}
	output := captureCLI(t, func() {
		if code := cmdDoctor(nil); code != 0 {
			t.Errorf("doctor returned %d", code)
		}
	})
	if output != "ok\n" {
		t.Fatalf("healthy doctor was noisy: %q", output)
	}
	output = captureCLI(t, func() {
		if code := cmdDoctor([]string{"--json"}); code != 0 {
			t.Errorf("JSON doctor returned %d", code)
		}
	})
	var findings []docFinding
	if err := json.Unmarshal([]byte(output), &findings); err != nil || len(findings) < 2 {
		t.Fatalf("missing diagnostic detail: %s, %v", output, err)
	}
}
