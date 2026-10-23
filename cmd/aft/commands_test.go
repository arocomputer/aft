package main

import "testing"

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
