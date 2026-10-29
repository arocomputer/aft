package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fschrhunt/aft/internal/config"
)

// TestSplitUNCandNFS parse the two mount source forms.
func TestSplitUNCandNFS(t *testing.T) {
	if s, sh := splitUNC("//fschr@100.65.233.79/Room"); s != "100.65.233.79" || sh != "Room" {
		t.Fatalf("UNC: %q %q", s, sh)
	}
	if s, sh := splitUNC("//host/Share/sub"); s != "host" || sh != "Share" {
		t.Fatalf("UNC sub: %q %q", s, sh)
	}
	if s, e := splitNFS("server:/export/path"); s != "server" || e != "/export/path" {
		t.Fatalf("NFS: %q %q", s, e)
	}
}

// TestRemoveMountBySymlink removes a mount, kept by its real path, by the
// symlinked spelling it was added with.
func TestRemoveMountBySymlink(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AFT_HOME", filepath.Join(dir, "home"))
	real, _ := filepath.EvalSymlinks(t.TempDir())
	link := filepath.Join(dir, "room")
	if err := os.Symlink(real, link); err != nil {
		t.Skip(err)
	}
	cfg := config.Config{Hosts: map[string]config.HostConfig{"box": {SSH: "box"}}, Mounts: []config.Mount{{Local: real, Host: "box", Path: "/srv/room"}}}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if code := cmdMount([]string{"remove", link}); code != 0 {
		t.Fatalf("remove exited %d", code)
	}
	if got, _ := config.Load(); len(got.Mounts) != 0 {
		t.Fatalf("mount still there: %+v", got.Mounts)
	}
	if code := cmdMount([]string{"remove", link}); code == 0 {
		t.Fatal("removing a mount that is not there reported success")
	}
}
