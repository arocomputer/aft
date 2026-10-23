package config

import "testing"

// TestMapLocal maps a path under a mount to its server path, most specific
// mount winning, and leaves unmounted paths alone.
func TestMapLocal(t *testing.T) {
	c := Config{Mounts: []Mount{
		{Local: "/Volumes/Room", Host: "server", Remote: "/home/fschr/room"},
		{Local: "/Volumes/Room/code", Host: "server", Remote: "/srv/code"},
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
