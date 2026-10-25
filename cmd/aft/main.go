// aft provides the client CLI and configured program launchers.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/arocomputer/aft/internal/bridge"
	"github.com/arocomputer/aft/internal/config"
	"github.com/arocomputer/aft/internal/home"
	"github.com/arocomputer/aft/internal/launcher"
	"github.com/arocomputer/aft/internal/session"
	"github.com/arocomputer/aft/internal/ui"
)

// commands supplies the flat command list for help, completion and suggestions.
var commands = [][3]string{
	{"setup", "[--host DEST]", "Configure a host, agents and tools"},
	{"on", "[--host H] [--dir D]", "Run this project remotely"},
	{"off", "", "Run this project locally"},
	{"HOST", "[CMD...]", "Open a new shell; CMD runs once"},
	{"ls", "", "List sessions"},
	{"attach", "[NAME]", "Attach to a session"},
	{"rename", "NAME NEW", "Rename a session"},
	{"kill", "NAME", "Stop a session"},
	{"host", "[add|rm|use]", "Manage hosts"},
	{"mount", "[list|add|remove]", "Manage mounted projects"},
	{"agents add", "NAME...", "Add agent launchers"},
	{"tools add", "NAME...", "Add one-off command launchers"},
	{"status", "", "Show project and connection status"},
	{"doctor", "[--fix] [--host H]", "Diagnose setup problems"},
	{"uninstall", "", "Remove Aft locally and from its hosts"},
	{"completion", "bash|zsh|fish", "Print shell completion"},
	{"version", "", "Print the version"},
	{"help", "[COMMAND]", "List commands or show command help"},
}

// meant maps common aliases and retired commands to supported commands.
var meant = map[string]string{
	"list": "ls", "sessions": "ls", "ps": "ls",
	"end": "kill", "stop": "kill", "rm": "kill", "mv": "rename",
	"ssh": "HOST", "shell": "HOST", "connect": "HOST",
	"use": "host use", "hosts": "host",
	"clone": "on", "ports": "status", "exec": "tools add", "run": "tools add",
	"check": "doctor", "init": "setup", "install": "setup", "remove": "uninstall",
}

// isCommand reports whether name is one of aft's commands.
func isCommand(name string) bool {
	switch name {
	case "selftest", "_complete":
		return true
	}
	for _, c := range commands {
		if strings.Fields(c[0])[0] == name {
			return true
		}
	}
	return false
}

// suggest returns the command someone probably meant by an unknown word, or
// empty: a known slip, the word without its dashes, or a near spelling.
func suggest(word string) string {
	w := strings.ToLower(strings.TrimLeft(word, "-"))
	switch w {
	case "h":
		w = "help"
	case "v":
		w = "version"
	}
	if m, ok := meant[w]; ok {
		return m
	}
	best, bestD := "", 3
	for _, c := range commands {
		name := strings.Fields(c[0])[0]
		if name == "HOST" {
			continue // a placeholder, not a command
		}
		if name == w && w != word {
			return name // --help, -version
		}
		if d := distance(w, name); d < bestD && d <= len(name)/2 {
			best, bestD = name, d
		}
	}
	return best
}

// distance is the edit distance between a and b.
func distance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}

// printUsage shows common commands, or every command, without branding or a footer.
func printUsage(detailed bool) {
	fmt.Print("usage: aft <command> [args]\n       aft <host> [command...]\n\n")
	for _, command := range commands {
		if command[0] == "HOST" {
			continue
		}
		if !detailed {
			switch command[0] {
			case "setup", "on", "off", "ls", "attach", "status", "help":
			default:
				continue
			}
		}
		fmt.Printf("  %-12s %s\n", command[0], command[2])
	}
}

// printCommandHelp describes flagless commands without loading configuration.
func printCommandHelp(command string) {
	switch command {
	case "host":
		fmt.Print("usage: aft host [command]\n\n  add NAME DEST  Add an SSH host\n  rm NAME        Remove a host\n  use [NAME]     Show or change the default host\n")
	case "mount":
		fmt.Print("usage: aft mount [command]\n\n  list         List mount mappings\n  add          Add a mount mapping\n  remove PATH  Remove a mount mapping\n")
	default:
		for _, entry := range commands {
			if strings.Fields(entry[0])[0] == command {
				fmt.Printf("usage: aft %s\n\n%s\n", strings.TrimSpace(entry[0]+" "+entry[1]), entry[2])
				return
			}
		}
	}
}

// version is set by the build.
var version = "dev"

func main() {
	session.Version = version
	base := filepath.Base(os.Args[0])
	if bridge.IsShim(base) {
		os.Exit(bridge.RunShim(os.Args[0], os.Args[1:]))
	}
	if base == home.ServerName {
		os.Exit(serverMain(os.Args[1:]))
	}
	// Invoked under an agent's name (via a launcher symlink), run that agent
	// on the server; invoked as aft (or a dev build whose name starts with
	// "aft"), act as the CLI.
	if !strings.HasPrefix(base, "aft") {
		os.Exit(launcher.Launch(base, os.Args[1:]))
	}
	if len(os.Args) < 2 {
		printUsage(false)
		os.Exit(0)
	}
	os.Exit(run(os.Args[1], os.Args[2:]))
}

// run dispatches a command; help requests return before any setup or connection.
func run(cmd string, args []string) int {
	if len(args) > 0 && (args[len(args)-1] == "--help" || args[len(args)-1] == "-h") {
		switch cmd {
		case "host", "agents", "tools":
			printCommandHelp(cmd)
			return 0
		case "mount":
			if len(args) == 1 || args[0] != "add" {
				printCommandHelp(cmd)
				return 0
			}
		case "off", "ls", "status", "completion", "version":
			printCommandHelp(cmd)
			return 0
		}
	}

	switch cmd {
	case "help", "--help", "-h":
		if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || (len(args) == 1 && args[0] == "help") {
			printUsage(true)
			return 0
		}
		nested := map[string][]string{"host": {"add", "rm", "use"}, "mount": {"list", "add", "remove"}, "agents": {"add"}, "tools": {"add"}}
		for _, entry := range commands {
			if strings.Fields(entry[0])[0] != args[0] || args[0] == "HOST" || args[0] == "help" {
				continue
			}
			if len(args) == 1 {
				return run(args[0], []string{"--help"})
			}
			if len(args) == 2 && slices.Contains(nested[args[0]], args[1]) {
				return run(args[0], []string{args[1], "--help"})
			}
		}
		ui.Error("usage: aft help [command]")
		return 2
	case "version", "--version", "-v":
		fmt.Println("aft", version)
		return 0
	case "setup":
		return cmdSetup(args)
	case "host":
		return cmdHost(args)
	case "on":
		return cmdOn(args)
	case "off":
		return cmdOff(args)
	case "ls":
		return cmdLs(args)
	case "attach":
		return cmdAttach(args)
	case "kill":
		return cmdKill(args)
	case "rename":
		return cmdRename(args)
	case "status":
		return cmdStatus(args)
	case "doctor":
		return cmdDoctor(args)
	case "mount":
		return cmdMount(args)
	case "agents", "tools":
		return cmdLaunchers(cmd, args)
	case "uninstall":
		return cmdUninstall(args)
	case "selftest":
		return cmdSelftest(args)
	case "completion":
		return cmdCompletion(args)
	case "_complete":
		return cmdComplete(args)
	}
	// aft HOST: a host you have, or one SSH can reach, which becomes one.
	if cfg, err := config.Load(); err == nil {
		if _, ok := cfg.Hosts[cmd]; ok || cmd == "local" {
			h, _ := cfg.Host(cmd)
			return cmdHostShell(cfg, h, args)
		}
		plainly := strings.Contains(cmd, "@")
		if !strings.HasPrefix(cmd, "-") && (plainly || suggest(cmd) == "") {
			if h, ok := adoptHost(&cfg, cmd, !plainly); ok {
				return cmdHostShell(cfg, h, args)
			}
			if plainly {
				return 1
			}
		}
	}
	ui.Error("unknown command %q", cmd)
	if m := suggest(cmd); m != "" {
		if cfg, err := config.Load(); m == "HOST" && err == nil && len(cfg.Hosts) == 1 {
			m = cfg.HostNames()[0]
		}
		ui.Suggest("aft " + m)
	} else {
		ui.Hint("aft help lists the commands")
	}
	return 2
}

// serverMain runs the session server: aft started as aft-server is the server
// for sessions on this machine, the same program a host runs as aft-server.
func serverMain(args []string) int {
	return session.Main(nil, args, os.Stdin, os.Stdout, os.Stderr)
}

// fail reports err on stderr and returns the exit code for a failed command.
func fail(err error) int {
	ui.Error("%v", err)
	return 1
}
