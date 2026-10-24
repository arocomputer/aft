package launcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arocomputer/aft/internal/config"
)

// TestShellBlockRoundTrip installs the PATH block into a shell file, leaving
// earlier content and a trailing newline intact, and removes it exactly.
func TestShellBlockRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AFT_HOME", filepath.Join(dir, ".aft"))
	t.Setenv("HOME", dir)
	t.Setenv("SHELL", "/bin/zsh")
	rc := filepath.Join(dir, ".zshrc")
	original := "export EDITOR=vim\nalias ll='ls -l'\n"
	if err := os.WriteFile(rc, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	var st config.State
	if _, err := InstallShellBlock(&st); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(rc)
	if !strings.Contains(string(after), blockStart) || !strings.HasPrefix(string(after), original) {
		t.Fatalf("install mangled the file:\n%s", after)
	}
	if len(st.ShellBlocks) != 1 || st.ShellBlocks[0].File != rc {
		t.Fatalf("not recorded: %+v", st.ShellBlocks)
	}
	removed, kept := RemoveShellBlocks(&st)
	if len(removed) != 1 || len(kept) != 0 {
		t.Fatalf("remove reported %v %v", removed, kept)
	}
	final, _ := os.ReadFile(rc)
	if string(final) != original {
		t.Fatalf("file not restored:\n%q\nwant\n%q", final, original)
	}
}

// TestEditedBlockIsKept checks a block the user changed is never deleted.
func TestEditedBlockIsKept(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AFT_HOME", filepath.Join(dir, ".aft"))
	t.Setenv("HOME", dir)
	t.Setenv("SHELL", "/bin/bash")
	rc := filepath.Join(dir, ".bashrc")
	os.WriteFile(rc, []byte("x=1\n"), 0o644)
	var st config.State
	InstallShellBlock(&st)
	// The user edits inside the block.
	content, _ := os.ReadFile(rc)
	edited := strings.Replace(string(content), PathDir, PathDir+":/extra", 1)
	os.WriteFile(rc, []byte(edited), 0o644)
	removed, kept := RemoveShellBlocks(&st)
	if len(removed) != 0 || len(kept) != 1 {
		t.Fatalf("edited block: removed %v kept %v", removed, kept)
	}
	if got, _ := os.ReadFile(rc); string(got) != edited {
		t.Fatal("edited block was modified")
	}
}

// TestRealBinarySkipsShims checks the launcher finds the actual program, not
// its own shim, so "off" runs the local binary.
func TestRealBinarySkipsShims(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AFT_HOME", filepath.Join(dir, ".aft"))
	shims := filepath.Join(dir, ".aft", "shims")
	realDir := filepath.Join(dir, "bin")
	os.MkdirAll(shims, 0o755)
	os.MkdirAll(realDir, 0o755)
	// A shim and a real binary of the same name.
	os.Symlink("/does/not/matter", filepath.Join(shims, "claude"))
	real := filepath.Join(realDir, "claude")
	os.WriteFile(real, []byte("#!/bin/sh\n"), 0o755)
	t.Setenv("PATH", shims+string(os.PathListSeparator)+realDir)
	got, err := RealBinary("claude")
	if err != nil || got != real {
		t.Fatalf("got %q %v, want %q", got, err, real)
	}
}

// TestEnvironmentAllowlist sends only allowlisted variables to the server.
func TestEnvironmentAllowlist(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("SECRET_TOKEN", "nope")
	t.Setenv("DATABASE_URL", "postgres://x")
	cfg := config.Config{Env: []string{"DATABASE_URL"}}
	env := Environment(cfg.Env)
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "TERM=xterm-256color") || !strings.Contains(joined, "DATABASE_URL=postgres://x") {
		t.Fatalf("missing allowed vars: %v", env)
	}
	if strings.Contains(joined, "SECRET_TOKEN") {
		t.Fatalf("leaked a non-allowlisted var: %v", env)
	}
}

// TestMoveShellBlocks rewrites a recorded block that names the old launchers
// directory, keeping the rest of the file.
func TestMoveShellBlocks(t *testing.T) {
	rc := filepath.Join(t.TempDir(), ".zshrc")
	old := blockStart + "\nexport PATH=\"$HOME/.aft/launchers:$PATH\"\n" + blockEnd + "\n"
	os.WriteFile(rc, []byte("alias ll='ls -l'\n\n"+old), 0o644)
	st := config.State{ShellBlocks: []config.ShellBlock{{File: rc, Block: old}}}
	if kept := MoveShellBlocks(&st); len(kept) != 0 {
		t.Fatalf("kept %v", kept)
	}
	got, _ := os.ReadFile(rc)
	if string(got) != "alias ll='ls -l'\n\n"+Block() || st.ShellBlocks[0].Block != Block() {
		t.Fatalf("not moved:\n%s", got)
	}
}

// TestServerPathFollowsMounts names a pasted file on this host's mount, also
// through a symlink to the mount, and not one under a more specific mount of
// another host, since that file lives there.
func TestServerPathFollowsMounts(t *testing.T) {
	mount := t.TempDir()
	os.MkdirAll(filepath.Join(mount, "code"), 0o755)
	link := filepath.Join(t.TempDir(), "room")
	if err := os.Symlink(mount, link); err != nil {
		t.Skip(err)
	}
	cfg := config.Config{Hosts: map[string]config.HostConfig{"box": {SSH: "box"}, "other": {SSH: "other"}}, Mounts: []config.Mount{
		{Local: mount, Host: "box", Path: "/srv/room"},
		{Local: filepath.Join(mount, "code"), Host: "other", Path: "/elsewhere"},
	}}
	opts, err := SessionOptions(cfg, "box", "", config.Project{})
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := opts.ServerPath(filepath.Join(link, "a.png")); !ok || got != "/srv/room/a.png" {
		t.Errorf("through the symlink: %q, %v", got, ok)
	}
	if got, ok := opts.ServerPath(filepath.Join(mount, "code", "b.txt")); ok {
		t.Errorf("a file on other's mount was named on box: %q", got)
	}
}
