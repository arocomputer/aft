// Package home locates Aft's private directory on either machine. Everything
// Aft writes lives under it, which is what makes uninstall exact. The layout:
//
//	~/.aft/
//	  config.toml   you:   hosts and preferences
//	  state.json    you:   which projects are on, and their host and directory
//	  launchers/    you:   the agent launcher symlinks
//	  server        host:  the installed server binary
//	  shims/        host:  the bridge shims (open, clipboard)
//	  sessions/     host:  one record per running session
//	  sockets/      host:  session and bridge sockets
//	  logs/         host:  per-session logs
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

// Launchers holds the agent launcher symlinks on a machine you type on.
func Launchers() string { return filepath.Join(Dir(), "launchers") }

// Server is the path of the installed server binary on a host.
func Server() string { return filepath.Join(Dir(), "server") }

// Shims holds the bridge shims a session puts on the host's PATH: open and the
// clipboard readers. It is separate from Launchers so an agent name on the host
// never resolves to a launcher, which would start a session inside a session.
func Shims() string { return filepath.Join(Dir(), "shims") }

// Sessions holds one JSON record per live session on a host.
func Sessions() string { return filepath.Join(Dir(), "sessions") }

// Sockets holds the session and bridge unix sockets on a host.
func Sockets() string { return filepath.Join(Dir(), "sockets") }

// Logs holds per-session logs on a host.
func Logs() string { return filepath.Join(Dir(), "logs") }

// ShimNames are the commands a session intercepts on the host so a remote
// application can reach the machine you are typing on.
var ShimNames = []string{"open", "xdg-open", "pbpaste", "wl-paste", "xclip", "xsel"}

// InstallShims points every bridge shim at self, creating the directory. self
// must be a binary that handles these names as shims (aft or the server).
func InstallShims(self string) error {
	if err := os.MkdirAll(Shims(), 0o700); err != nil {
		return err
	}
	for _, n := range ShimNames {
		link := filepath.Join(Shims(), n)
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
	for _, d := range []string{Dir(), Launchers(), Shims(), Sessions(), Sockets(), Logs()} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return err
		}
	}
	return nil
}
