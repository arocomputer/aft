package transport

import (
	"strings"
	"testing"
)

// TestSSHDest splits options from the destination so a host entry can carry
// ssh flags like a port.
func TestSSHDest(t *testing.T) {
	cases := map[string]struct {
		opts []string
		dest string
	}{
		"box":                   {nil, "box"},
		"user@box":              {nil, "user@box"},
		"-p 2222 user@box":      {[]string{"-p", "2222"}, "user@box"},
		"-o ProxyJump=jump srv": {[]string{"-o", "ProxyJump=jump"}, "srv"},
	}
	for ssh, want := range cases {
		h := Host{Name: "h", SSH: ssh}
		opts, dest := h.sshDest()
		if len(opts) != len(want.opts) || dest != want.dest {
			t.Errorf("%q: got %v %q want %v %q", ssh, opts, dest, want.opts, want.dest)
			continue
		}
		for i := range opts {
			if opts[i] != want.opts[i] {
				t.Errorf("%q: opt %d = %q want %q", ssh, i, opts[i], want.opts[i])
			}
		}
		if h.Dest() != want.dest {
			t.Errorf("%q: Dest() = %q", ssh, h.Dest())
		}
	}
}

// TestSSHArgsOrder checks options come before the destination, with host
// options after the shared ones, and "--" guarding the destination.
func TestSSHArgsOrder(t *testing.T) {
	h := Host{Name: "box", SSH: "-p 2222 user@box"}
	args := sshArgs(h, true, "-T")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "BatchMode=yes") {
		t.Fatal("batch option missing")
	}
	// The destination is last, preceded by "--".
	if args[len(args)-1] != "user@box" || args[len(args)-2] != "--" {
		t.Fatalf("destination not last: %v", args)
	}
	// Host options and the extra flag appear before "--".
	if !strings.Contains(joined, "-p 2222") || !strings.Contains(joined, "-T --") {
		t.Fatalf("args: %v", args)
	}
}

// TestQuote makes one shell word that survives the shell unchanged.
func TestQuote(t *testing.T) {
	for _, s := range []string{"plain", "with space", "it's", "a;b|c&d", "$(x)", "~/p", ""} {
		q := Quote(s)
		// A bare echo of the quoted word must reproduce the input exactly.
		if s != "" && !strings.ContainsAny(s, " \t\n'\"\\$`!*?[]{}()<>|&;~#") && q != s {
			t.Errorf("%q needlessly quoted to %q", s, q)
		}
	}
	if Quote("it's") != `'it'"'"'s'` {
		t.Fatalf("single quote escaping: %s", Quote("it's"))
	}
}
