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

// Log holds per-session holder logs.
func Log() string { return filepath.Join(Dir(), "log") }

// Bin holds the helper binary on a server.
func Bin() string { return filepath.Join(Dir(), "bin") }

// Shims holds the launcher symlinks on the client and the bridge shims on a
// server.
func Shims() string { return filepath.Join(Dir(), "shims") }

// Ensure creates the standard directories with private permissions.
func Ensure() error {
	for _, d := range []string{Dir(), Run(), Sessions(), Log(), Bin(), Shims()} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return err
		}
	}
	return nil
}
