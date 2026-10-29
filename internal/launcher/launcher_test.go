package launcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fschrhunt/aft/internal/config"
	"github.com/fschrhunt/aft/internal/home"
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
	realDir := filepath.Join(dir, "bin")
	os.MkdirAll(home.Launchers(), 0o755)
	os.MkdirAll(realDir, 0o755)
	// A launcher and a real binary of the same name; the launcher resolves.
	real := filepath.Join(realDir, "claude")
	os.WriteFile(real, []byte("#!/bin/sh\n"), 0o755)
	os.Symlink(real, filepath.Join(home.Launchers(), "claude"))
	t.Setenv("PATH", home.Launchers()+string(os.PathListSeparator)+realDir)
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

// TestServerPathFollowsMounts names a pasted file on this host's mount, also
// through a symlink to the mount, and not one under a more specific mount of
// another host, since that file lives there.
func TestServerPathFollowsMounts(t *testing.T) {
	mount, _ := filepath.EvalSymlinks(t.TempDir()) // kept by its real path, as aft mount add does
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

// TestPathInShellConfig sees the launchers on PATH for new shells from the
// shell config alone: aft's block, or a hand-written line, and not an rc file
// without either.
func TestPathInShellConfig(t *testing.T) {
	h := t.TempDir()
	t.Setenv("HOME", h)
	t.Setenv("SHELL", "/bin/zsh")
	rc := filepath.Join(h, ".zshrc")
	os.WriteFile(rc, []byte("alias ll='ls -l'\n"), 0o644)
	if PathInShellConfig(config.State{}) {
		t.Fatal("an rc file without the line counts")
	}
	os.WriteFile(rc, []byte("alias ll='ls -l'\nexport PATH=\""+PathDir+":$PATH\"\n"), 0o644)
	if !PathInShellConfig(config.State{}) {
		t.Fatal("a hand-written line is missed")
	}
	os.WriteFile(rc, []byte("alias ll='ls -l'\n"+Block()), 0o644)
	if !PathInShellConfig(config.State{ShellBlocks: []config.ShellBlock{{File: rc, Block: Block()}}}) {
		t.Fatal("aft's own block is missed")
	}
}

// TestShellBlockComesBackWhenTheFileLostIt reinstalls the PATH block in a
// recorded file that no longer contains it, as after dotfiles are restored,
// instead of trusting the record.
func TestShellBlockComesBackWhenTheFileLostIt(t *testing.T) {
	h := t.TempDir()
	t.Setenv("HOME", h)
	t.Setenv("SHELL", "/bin/zsh")
	rc := filepath.Join(h, ".zshrc")
	os.WriteFile(rc, []byte("alias ll='ls -l'\n"), 0o644)
	st := config.State{ShellBlocks: []config.ShellBlock{{File: rc, Block: Block()}}}
	changed, err := InstallShellBlock(&st)
	if err != nil || len(changed) != 1 {
		t.Fatalf("changed %v err %v", changed, err)
	}
	if raw, _ := os.ReadFile(rc); !strings.Contains(string(raw), Block()) {
		t.Fatalf("block not put back:\n%s", raw)
	}
}

// TestValidName refuses launcher names aft could not tell apart from its
// own commands and shims.
func TestValidName(t *testing.T) {
	for _, bad := range []string{"", "aft", "aftx", "aft-server", "open", "xclip", "foo bar", "a/b"} {
		if ValidName(bad) == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	for _, good := range []string{"claude", "gh", "git", "cursor-agent"} {
		if err := ValidName(good); err != nil {
			t.Errorf("%q refused: %v", good, err)
		}
	}
}

// TestSyncLaunchersRemovesOnlyOwnedSelections leaves a user-replaced launcher
// and unrelated files intact while removing a deselected Aft symlink.
func TestSyncLaunchersRemovesOnlyOwnedSelections(t *testing.T) {
	t.Setenv("AFT_HOME", t.TempDir())
	var state config.State
	if err := InstallLaunchers("/test/aft", []string{"claude", "codex"}, &state); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home.Launchers(), "codex")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("user-owned"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := SyncLaunchers("/test/aft", nil, &state); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(home.Launchers(), "claude")); !os.IsNotExist(err) {
		t.Fatalf("owned launcher remains: %v", err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "user-owned" {
		t.Fatalf("modified launcher removed: %q %v", data, err)
	}
	if len(state.Shims) != 0 {
		t.Fatal("deselected names still owned")
	}
	if err := InstallLaunchers("/test/aft", []string{"codex"}, &state); err == nil {
		t.Fatal("overwrote the user's file")
	}
}
