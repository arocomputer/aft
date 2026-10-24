// server is the small binary Aft installs on a session. It starts and
// attaches sessions, answers doctor, and, when invoked through one of the shim
// symlinks, bridges a remote application to the user's machine.
package main

import (
	"os"

	"github.com/arocomputer/aft/internal/bridge"
	"github.com/arocomputer/aft/internal/session"
)

func main() {
	if bridge.IsShim(os.Args[0]) {
		os.Exit(bridge.RunShim(os.Args[0], os.Args[1:]))
	}
	os.Exit(session.Main(nil, os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
