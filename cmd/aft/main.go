// aft runs your agents on your server while the terminal stays on your machine.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/arocomputer/aft/internal/bridge"
	"github.com/arocomputer/aft/internal/config"
	"github.com/arocomputer/aft/internal/home"
	"github.com/arocomputer/aft/internal/launcher"
	"github.com/arocomputer/aft/internal/session"
	"github.com/arocomputer/aft/internal/ui"
)

// commands is the help screen: groups of command, arguments and summary.
var commands = []struct {
	group string
	cmds  [][3]string
}{
	{"Start", [][3]string{
		{"setup", "", "add a host, create launchers, add the PATH block"},
		{"on", "[--host H] [--dir D]", "agents started in this directory run on the host"},
		{"off", "", "run them locally again"},
		{"clone", "URL [DIR]", "clone on the server, create a local handle, turn it on"},
	}},
	{"Sessions", [][3]string{
		{"shell", "[--host H]", "a lag-free shell on the server"},
		{"ls", "", "sessions on every host"},
		{"attach", "[NAME]", "reattach to a session"},
		{"detach", "NAME", "detach whoever is attached"},
		{"kill", "NAME", "stop a session"},
	}},
	{"Hosts", [][3]string{
		{"host", "[add|rm]", "list your hosts, or add or remove one"},
		{"use", "[NAME]", "show or set the host used when --host is omitted"},
		{"exec", "[--host H] -- CMD", "run one command on the server"},
		{"ports", "", "mirrored ports"},
		{"mount", "list|add|remove", "map a network-mounted folder"},
	}},
	{"Maintain", [][3]string{
		{"status", "", "where you are, hosts, sessions, projects"},
		{"doctor", "[--fix]", "check SSH, server, agents, launchers, shell block"},
		{"agents add", "NAME", "launch another program through Aft"},
		{"uninstall", "", "remove everything Aft added, everywhere"},
		{"completion", "bash|zsh|fish", "shell completion"},
		{"version", "", "print the version"},
	}},
}

// aliases are other names people reach for.
var aliases = map[string]string{"list": "ls", "sessions": "ls", "--version": "version", "-v": "version"}

func printUsage() {
	fmt.Println(ui.Bold("aft") + ui.Dim(" — your agents on your server, your terminal on your machine"))
	for _, g := range commands {
		fmt.Println("\n" + ui.Bold(g.group))
		for _, c := range g.cmds {
			name := fmt.Sprintf("%-11s", c[0])
			fmt.Printf("  %s %s%s\n", ui.Accent(name), fmt.Sprintf("%-22s", c[1]), ui.Dim(c[2]))
		}
	}
	fmt.Println("\n" + ui.Dim("In a project that is on, run claude, codex, opencode or pi exactly as before."))
}

// version is set by the build.
var version = "dev"

func main() {
	session.Version = version
	base := filepath.Base(os.Args[0])
	if bridge.IsShim(base) {
		os.Exit(bridge.RunShim(os.Args[0], os.Args[1:]))
	}
	if home.Migrate() {
		moveShellBlocks()
	}
	// Invoked under an agent's name (via a launcher symlink), run that agent
	// on the server; invoked as aft (or a dev build whose name starts with
	// "aft"), act as the CLI.
	if !strings.HasPrefix(base, "aft") {
		os.Exit(launcher.Launch(base, os.Args[1:]))
	}
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}
	os.Exit(run(os.Args[1], os.Args[2:]))
}

// moveShellBlocks points the PATH line at the launchers' new home and tells
// you about any line it could not change. Remove it with home.Migrate.
func moveShellBlocks() {
	st, err := config.LoadState()
	if err != nil {
		return
	}
	kept := launcher.MoveShellBlocks(&st)
	config.SaveState(st)
	ui.Note("launchers moved to %s; open a new shell to use them", ui.Path(home.Launchers()))
	if len(st.ShellBlocks) == 0 || len(kept) > 0 {
		ui.Warn("put this first on your PATH instead of $HOME/.aft/launchers%s:", inFiles(kept))
		fmt.Fprintf(os.Stderr, "    export PATH=\"%s:$PATH\"\n", launcher.PathDir)
	}
}

func inFiles(files []string) string {
	if len(files) == 0 {
		return ""
	}
	return " in " + strings.Join(files, ", ")
}

func run(cmd string, args []string) int {
	if a, ok := aliases[cmd]; ok {
		cmd = a
	}
	switch cmd {
	case "help", "-h", "--help":
		printUsage()
		return 0
	case "version":
		fmt.Println("aft", version)
		return 0
	case "server":
		self, err := os.Executable()
		if err != nil {
			return fail(err)
		}
		return session.Main([]string{self, "server"}, args, os.Stdin, os.Stdout, os.Stderr)
	case "setup":
		return cmdSetup(args)
	case "host":
		return cmdHost(args)
	case "use":
		return cmdUse(args)
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
	case "kill":
		return cmdControl("end", args)
	case "status":
		return cmdStatus(args)
	case "doctor":
		return cmdDoctor(args)
	case "exec":
		return cmdExec(args)
	case "ports":
		return cmdPorts(args)
	case "mount":
		return cmdMount(args)
	case "agents":
		return cmdAgents(args)
	case "uninstall":
		return cmdUninstall(args)
	case "selftest":
		return cmdSelftest(args)
	case "completion":
		return cmdCompletion(args)
	}
	ui.Error("unknown command %q", cmd)
	ui.Hint("aft help lists the commands")
	return 2
}

// fail reports err on stderr and returns the exit code for a failed command.
func fail(err error) int {
	ui.Error("%v", err)
	return 1
}
