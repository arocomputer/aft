// aft runs your agents on your server while the terminal stays on your machine.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fschrhunt/aft/internal/helper"
	"github.com/fschrhunt/aft/internal/launcher"
)

const usage = `aft — your agents on your server, your terminal on your machine

  aft setup                     add a server, create launchers, add the PATH block
  aft on [--host H] [--dir D]   agents started in this directory run on the server
  aft off                       run them locally again
  aft clone URL [DIR]           clone on the server, create a local handle, turn it on
  aft shell [--host H]          a lag-free shell on the server
  aft ls                        sessions on every host
  aft attach [ID]               reattach to a session
  aft detach ID                 detach whoever is attached
  aft end ID                    stop a session
  aft status                    hosts, connections, sessions, mirrors
  aft doctor                    check SSH, helper, agents, launchers, shell block
  aft exec [--host H] -- CMD    run one command on the server
  aft ports                     mirrored ports
  aft agents add NAME           launch another program through Aft
  aft uninstall                 remove everything Aft added, everywhere
  aft completion bash|zsh|fish
  aft version

In a project that is on, run claude, codex, opencode or pi exactly as before.
`

// version is set by the build.
var version = "dev"

func main() {
	helper.Version = version
	base := filepath.Base(os.Args[0])
	if helper.IsShim(base) {
		os.Exit(helper.RunShim(os.Args[0], os.Args[1:]))
	}
	// Invoked under an agent's name (via a launcher symlink), run that agent
	// on the server; invoked as aft (or a dev build whose name starts with
	// "aft"), act as the CLI.
	if !strings.HasPrefix(base, "aft") {
		os.Exit(launcher.Launch(base, os.Args[1:]))
	}
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stdout, usage)
		os.Exit(0)
	}
	os.Exit(run(os.Args[1], os.Args[2:]))
}

func run(cmd string, args []string) int {
	switch cmd {
	case "help", "-h", "--help":
		fmt.Fprint(os.Stdout, usage)
		return 0
	case "version", "--version":
		fmt.Println("aft", version)
		return 0
	case "helper":
		self, err := os.Executable()
		if err != nil {
			return fail(err)
		}
		return helper.Main([]string{self, "helper"}, args, os.Stdin, os.Stdout, os.Stderr)
	case "setup":
		return cmdSetup(args)
	case "on":
		return cmdOn(args)
	case "off":
		return cmdOff(args)
	case "clone":
		return cmdClone(args)
	case "shell":
		return cmdShell(args)
	case "ls":
		return cmdLs(args)
	case "attach":
		return cmdAttach(args)
	case "detach":
		return cmdControl("kick", args)
	case "end":
		return cmdControl("end", args)
	case "status":
		return cmdStatus(args)
	case "doctor":
		return cmdDoctor(args)
	case "exec":
		return cmdExec(args)
	case "ports":
		return cmdPorts(args)
	case "agents":
		return cmdAgents(args)
	case "uninstall":
		return cmdUninstall(args)
	case "completion":
		return cmdCompletion(args)
	}
	fmt.Fprintf(os.Stderr, "aft: unknown command %q\n%s", cmd, usage)
	return 2
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "aft:", err)
	return 1
}
