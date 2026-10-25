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

// commands supplies the flat command list for help, completion and suggestions.
var commands = [][3]string{
	{"setup", "[--host DEST]", "Set up a host and launchers"},
	{"on", "[--host H] [--dir D]", "Enable Aft for this project"},
	{"off", "", "Run this project's agents locally"},
	{"HOST", "[CMD...]", "Open a new shell; CMD runs once"},
	{"ls", "", "List sessions"},
	{"attach", "[NAME]", "Attach to a session"},
	{"rename", "NAME NEW", "Rename a session"},
	{"kill", "NAME", "Stop a session"},
	{"host", "[add|rm|use]", "Manage hosts and the default host"},
	{"mount", "[list|add|remove]", "Map a mounted folder to a host"},
	{"agents add", "NAME...", "Add programs that run in sessions"},
	{"tools add", "NAME...", "Add commands that run once on the host"},
	{"status", "", "Show projects, hosts, sessions and ports"},
	{"doctor", "[--fix] [--host H]", "Check setup and optionally repair it"},
	{"uninstall", "", "Remove Aft from this machine and its hosts"},
	{"completion", "bash|zsh|fish", "Print shell completion"},
	{"version", "", "Print the version"},
	{"help", "", "Show the full command reference"},
}

// meant maps names people reach for, including commands aft no longer has, to
// what they are after. Standard help and version flags are also accepted.
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

// printUsage prints a compact overview, or the full reference when detailed.
func printUsage(detailed bool) {
	fmt.Println("Usage: aft <command>")
	fmt.Println()
	var rows [][]string
	for _, c := range commands {
		if !detailed {
			switch c[0] {
			case "setup", "on", "off", "HOST", "ls", "attach", "status", "help":
			default:
				continue
			}
		}
		invocation := c[0]
		if (detailed || c[0] == "HOST" || c[0] == "attach") && c[1] != "" {
			invocation += " " + c[1]
		}
		rows = append(rows, []string{"  " + invocation, c[2]})
	}
	ui.Table(nil, rows)
	if detailed {
		fmt.Print("\nRun claude, codex, opencode or pi as usual in an enabled project.\n" +
			"Host shells appear in aft ls only while inside an enabled project.\n" +
			"Host commands: aft host add NAME DEST, aft host rm NAME, aft host use NAME.\n" +
			"Mount a folder: aft mount add --local DIR --host HOST --remote DIR.\n" +
			"Use aft <command> --help for flags (for example, aft on --help).\n")
	} else {
		fmt.Println("\nRun aft setup to get started; aft help lists every command.")
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
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		switch cmd {
		case "off", "ls", "status", "host", "mount", "agents", "tools", "completion", "version":
			for _, c := range commands {
				if strings.Fields(c[0])[0] == cmd {
					fmt.Printf("Usage: aft %s\n%s\n", strings.TrimSpace(c[0]+" "+c[1]), c[2])
					return 0
				}
			}
		}
	}
	switch cmd {
	case "help", "--help", "-h":
		printUsage(true)
		return 0
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
