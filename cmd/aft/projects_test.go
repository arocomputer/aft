package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// physicalTempDir avoids OS-level aliases such as macOS /tmp -> /private/tmp.
func physicalTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// isolateProjects points config resolvers at a physical temporary home, not an OS alias.
func isolateProjects(t *testing.T) {
	t.Helper()
	home := physicalTempDir(t)
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
}

// TestProjectLifecycle pins private, non-overwriting metadata and safe removal.
func TestProjectLifecycle(t *testing.T) {
	isolateProjects(t)
	p := project{"app", "server", "/home/me/project's files", ""}
	if err := saveProject(p); err != nil {
		t.Fatal(err)
	}
	if err := saveProject(project{"app", "other", "/tmp/other", ""}); err == nil {
		t.Fatal("overwrote record")
	}
	got, err := loadProject("app")
	if err != nil || got != p {
		t.Fatalf("%+v %v", got, err)
	}
	file, _ := projectPath("app")
	info, _ := os.Stat(file)
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatal("record is not private")
	}
	if err := runProjects([]string{"remove", "app"}); err != nil {
		t.Fatal(err)
	}
	if _, err := loadProject("app"); !os.IsNotExist(err) {
		t.Fatalf("record remains: %v", err)
	}
}

// TestInvalidProject rejects traversal, option injection, and ambiguous home paths.
func TestInvalidProject(t *testing.T) {
	isolateProjects(t)
	for _, p := range []project{{"../escape", "server", "/work", ""}, {"app", "-oBad", "/work", ""}, {"app", "server", "~/work", ""}, {"app", "server", "/", ""}, {"app", "server", "/work\nother", ""}} {
		if err := saveProject(p); err == nil {
			t.Errorf("accepted %+v", p)
		}
	}
}
