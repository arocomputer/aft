// helper is the small binary Aft installs on a server. It starts and
// attaches sessions, answers doctor, and, when invoked through one of the shim
// symlinks, bridges a remote application to the user's machine.
package main

import (
	"os"

	"github.com/arocomputer/aft/internal/helper"
)

func main() {
	if helper.IsShim(os.Args[0]) {
		os.Exit(helper.RunShim(os.Args[0], os.Args[1:]))
	}
	os.Exit(helper.Main(nil, os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
