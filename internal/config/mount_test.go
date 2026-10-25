package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMapLocal maps a path under a mount to its server path, most specific
// mount winning, and leaves unmounted paths alone.
func TestMapLocal(t *testing.T) {
	c := Config{Mounts: []Mount{
		{Local: "/Volumes/Room", Host: "server", Path: "/home/fschr/room"},
		{Local: "/Volumes/Room/code", Host: "server", Path: "/srv/code"},
	}}
	cases := []struct {
		in, host, remote string
		ok               bool
	}{
		{"/Volumes/Room", "server", "/home/fschr/room", true},
		{"/Volumes/Room/media/x", "server", "/home/fschr/room/media/x", true},
		{"/Volumes/Room/code/repos/foo", "server", "/srv/code/repos/foo", true},
		{"/Users/fschr/other", "", "", false},
		{"/Volumes/RoomBackup", "", "", false},
	}
	for _, tc := range cases {
		h, r, ok := c.MapLocal(tc.in)
		if ok != tc.ok || h != tc.host || r != tc.remote {
			t.Errorf("MapLocal(%q) = %q %q %v, want %q %q %v", tc.in, h, r, ok, tc.host, tc.remote, tc.ok)
		}
	}
}

// TestMapLocalThroughSymlinks maps a path reached through a symlink to a
// mount (~/room for /Volumes/Room), and still lets a more specific mount win
// when only the real spelling reaches it.
func TestMapLocalThroughSymlinks(t *testing.T) {
	real, _ := filepath.EvalSymlinks(t.TempDir())
	os.MkdirAll(filepath.Join(real, "code"), 0o755)
	link := filepath.Join(t.TempDir(), "room")
	if err := os.Symlink(real, link); err != nil {
		t.Skip(err)
	}
	c := Config{Mounts: []Mount{
		{Local: real, Host: "box", Path: "/srv/room"},
		{Local: filepath.Join(real, "code"), Host: "other", Path: "/elsewhere"},
	}}
	if h, r, ok := c.MapLocal(filepath.Join(link, "a.png")); !ok || h != "box" || r != "/srv/room/a.png" {
		t.Fatalf("through the symlink: %q %q %v", h, r, ok)
	}
	if h, r, ok := c.MapLocal(filepath.Join(link, "code", "b.txt")); !ok || h != "other" || r != "/elsewhere/b.txt" {
		t.Fatalf("nested mount through the symlink: %q %q %v", h, r, ok)
	}
}
