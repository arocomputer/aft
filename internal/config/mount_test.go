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

// TestMapLocalThroughSymlinks maps a path when the mount is configured through
// a symlink and the path is its real spelling, as Finder pastes it.
func TestMapLocalThroughSymlinks(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "room")
	if err := os.Symlink(real, link); err != nil {
		t.Skip(err)
	}
	c := Config{Mounts: []Mount{{Local: link, Host: "box", Path: "/srv/room"}}}
	if h, r, ok := c.MapLocal(filepath.Join(real, "a.png")); !ok || h != "box" || r != "/srv/room/a.png" {
		t.Fatalf("got %q %q %v", h, r, ok)
	}
}
