// Package home locates Aft's private directory on either machine. Everything
// Aft writes lives under it, which is what makes uninstall exact.
package home

import (
	"os"
	"path/filepath"
)

// Dir returns $AFT_HOME, or ~/.aft.
func Dir() string {
	if d := os.Getenv("AFT_HOME"); d != "" {
		return d
	}
	h, err := os.UserHomeDir()
	if err != nil {
		h = "."
	}
	return filepath.Join(h, ".aft")
}

// Run holds sockets and control paths.
func Run() string { return filepath.Join(Dir(), "run") }

// Sessions holds one JSON record per live session.
func Sessions() string { return filepath.Join(Dir(), "sessions") }

// Log holds per-session logs.
func Log() string { return filepath.Join(Dir(), "log") }

// Bin holds the helper binary on a server.
func Bin() string { return filepath.Join(Dir(), "bin") }

// Shims holds the agent launcher symlinks on the client.
func Shims() string { return filepath.Join(Dir(), "shims") }

// Bridge holds the bridge shims a session puts on the server's PATH: open,
// clipboard readers and the like. It is deliberately separate from Shims so an
// agent name on the server never resolves to a launcher, which would start a
// session inside a session.
func Bridge() string { return filepath.Join(Dir(), "bridge") }

// BridgeShims are the commands a session intercepts on the server so a remote
// application can reach the user's machine.
var BridgeShims = []string{"open", "xdg-open", "pbpaste", "wl-paste", "xclip", "xsel"}

// InstallBridge points every bridge shim at self, creating the directory. self
// must be a binary that handles these names as shims (aft or helper).
func InstallBridge(self string) error {
	if err := os.MkdirAll(Bridge(), 0o700); err != nil {
		return err
	}
	for _, n := range BridgeShims {
		link := filepath.Join(Bridge(), n)
		if cur, err := os.Readlink(link); err == nil && cur == self {
			continue
		}
		os.Remove(link)
		if err := os.Symlink(self, link); err != nil {
			return err
		}
	}
	return nil
}

// Ensure creates the standard directories with private permissions.
func Ensure() error {
	for _, d := range []string{Dir(), Run(), Sessions(), Log(), Bin(), Shims(), Bridge()} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return err
		}
	}
	return nil
}
