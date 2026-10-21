// Aft connects local tools to persistent development sessions on your machines.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

const usage = `aft — development on your machines

Usage (put flags before positional arguments):
  aft setup [--name NAME --host HOST --root '~/repos']
  aft workspace add [--default] NAME HOST ROOT
  aft workspace list | default NAME | remove NAME
  aft new [--workspace NAME] [--name NAME] RELATIVE_PATH
  aft add [--workspace NAME] [--name NAME] RELATIVE_PATH
  aft project list | remove NAME | migrate [--from DIRECTORY]
  aft project add NAME HOST /REMOTE/PATH   (explicit host, outside workspaces)
  aft open NAME
  aft exec [--host HOST --dir PATH] PROJECT -- COMMAND [ARG...]
  aft run [--session NAME] [--host HOST --dir PATH] PROJECT -- COMMAND [ARG...]
  aft connect [--session NAME] [--dir PATH] HOST
  aft sessions HOST
  aft ports [--project NAME] [--local PORT] [HOST] PORT
  aft ports list [--project NAME] [HOST]
  aft ports watch [--project NAME] [--min PORT --max PORT] [HOST]
  aft doctor [HOST]
  aft status [--project NAME] [HOST]
  aft paths
  aft completion bash|zsh
  aft uninstall --dry-run
  aft service install [--min PORT --max PORT] HOST
  aft service status HOST | remove HOST

No arguments opens a terminal project picker; redirected input shows help.
Host-mode exec/run omit PROJECT and require --host. Commands require --.
SSH authentication and host-key checks remain in your existing SSH config.
New default sessions use aft- names. Existing sessions are never killed.
`

var sessionName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

// quote makes one POSIX shell word; remote input is never interpolated raw.
func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// validHost rejects option-like or control-containing destinations, leaving
// resolution and authentication to the user's system OpenSSH client.
func validHost(host string) bool {
	if host == "" || strings.HasPrefix(host, "-") {
		return false
	}
	return !strings.ContainsAny(host, " \t\r\n\x00")
}

// port validates an unprivileged TCP port before constructing SSH arguments.
func port(value string) (int, error) {
	n, err := strconv.Atoi(value)
	if err != nil || n < 1024 || n > 65535 {
		return 0, fmt.Errorf("port must be between 1024 and 65535: %q", value)
	}
	return n, nil
}

// sshArgs translates one CLI request into SSH arguments without touching files.
// A nil result means help was printed and no connection should be made.
func sshArgs(args []string, out io.Writer) ([]string, error) {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprint(out, usage)
		return nil, nil
	}
	command := args[0]
	if command != "connect" && command != "sessions" && command != "ports" && command != "exec" && command != "run" {
		return nil, fmt.Errorf("unknown command %q; run aft help", command)
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(out)
	flags.Usage = func() { fmt.Fprint(out, usage) }
	var session, dir, local string
	if command == "connect" {
		flags.StringVar(&session, "session", "aft-main", "persistent session name")
	}
	if command == "run" {
		flags.StringVar(&session, "session", "", "required new persistent session name")
	}
	if command == "connect" || command == "run" || command == "exec" {
		flags.StringVar(&dir, "dir", "", "existing remote working directory")
	}
	if command == "ports" {
		flags.StringVar(&local, "local", "", "local port (defaults to remote port)")
	}
	if err := flags.Parse(args[1:]); err != nil {
		if err == flag.ErrHelp {
			return nil, nil
		}
		return nil, err
	}
	want := 1
	if command == "ports" {
		want = 2
	}
	if command == "exec" || command == "run" {
		if flags.NArg() < 2 {
			return nil, fmt.Errorf("%s requires HOST COMMAND [ARG...]; put flags before HOST", command)
		}
	} else if flags.NArg() != want {
		return nil, fmt.Errorf("%s expects %d positional arguments; put flags before HOST", command, want)
	}
	host := flags.Arg(0)
	if !validHost(host) {
		return nil, fmt.Errorf("invalid SSH destination %q", host)
	}
	base := []string{"-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=3"}
	if strings.ContainsAny(dir, "\x00\r\n") {
		return nil, fmt.Errorf("directory cannot contain NUL or newlines")
	}
	if dir != "" && !strings.HasPrefix(dir, "/") {
		dir = "./" + dir
	}
	switch command {
	case "connect":
		if !sessionName.MatchString(session) {
			return nil, fmt.Errorf("session must be 1–64 letters, digits, hyphens or underscores, starting with a letter or digit")
		}
		script := "command -v tmux >/dev/null 2>&1 || { echo 'aft: install tmux on this machine first' >&2; exit 127; }; exec tmux new-session -A -s " + quote(session)
		if dir != "" {
			script += " -c " + quote(dir)
		}
		return append(base, "-t", "--", host, "sh -c "+quote(script)), nil
	case "sessions":
		return append(base, "--", host, "tmux list-sessions"), nil
	case "exec", "run":
		var words []string
		for _, arg := range flags.Args()[1:] {
			if strings.ContainsRune(arg, 0) {
				return nil, fmt.Errorf("command arguments cannot contain NUL")
			}
			words = append(words, quote(arg))
		}
		if flags.Arg(1) == "" || strings.HasPrefix(flags.Arg(1), "-") {
			return nil, fmt.Errorf("invalid command executable")
		}
		script := "exec " + strings.Join(words, " ")
		if command == "exec" {
			if dir != "" {
				script = "cd " + quote(dir) + " && " + script
			}
		} else {
			if !sessionName.MatchString(session) {
				return nil, fmt.Errorf("run requires a valid --session NAME")
			}
			tmux := "exec tmux new-session -d -s " + quote(session)
			if dir != "" {
				tmux += " -c " + quote(dir)
			}
			script = tmux + " " + quote(script)
		}
		return append(base, "--", host, "sh -c "+quote(script)), nil
	case "ports":
		remotePort, err := port(flags.Arg(1))
		if err != nil {
			return nil, err
		}
		if local == "" {
			local = flags.Arg(1)
		}
		localPort, err := port(local)
		if err != nil {
			return nil, err
		}
		forward := fmt.Sprintf("127.0.0.1:%d:localhost:%d", localPort, remotePort)
		return append(base, "-S", "none", "-N", "-T", "-o", "ExitOnForwardFailure=yes", "-L", forward, "--", host), nil
	}
	panic("unreachable")
}

// run preserves SSH's exit status and connects directly to the user's terminal.
func run(args []string) int {
	if len(args) == 2 && (args[1] == "--help" || args[1] == "-h") {
		fmt.Fprint(os.Stdout, usage)
		return 0
	}
	if len(args) == 0 && terminalInput() {
		return pickProject()
	}
	if len(args) > 0 && (args[0] == "exec" || args[0] == "run") {
		ssh, err := executionArgs(args)
		if err != nil {
			fmt.Fprintln(os.Stderr, "aft:", err)
			return 2
		}
		return executeSSH(ssh)
	}
	if len(args) > 0 && args[0] == "ports" {
		resolved, err := resolvePortProject(args)
		if err != nil {
			fmt.Fprintln(os.Stderr, "aft:", err)
			return 2
		}
		args = resolved
	}
	if len(args) > 0 {
		var handler func([]string) error
		switch args[0] {
		case "project":
			handler = runProjects
		case "doctor":
			handler = runDoctor
		case "service":
			handler = runService
		case "setup":
			handler = runSetup
		case "workspace":
			handler = runWorkspace
		case "new", "add":
			action := args[0]
			handler = func(a []string) error { return registerProject(action, a) }
		case "paths":
			handler = runPaths
		case "status":
			handler = runStatus
		case "completion", "_complete":
			action := args[0]
			handler = func(a []string) error { return runCompletion(action, a) }
		case "uninstall":
			handler = runUninstall
		case "open":
			if len(args) != 2 {
				fmt.Fprintln(os.Stderr, "usage: aft open PROJECT")
				return 2
			}
			p, err := loadProject(args[1])
			if err != nil {
				fmt.Fprintln(os.Stderr, "aft:", err)
				return 1
			}
			ssh, err := sshArgs([]string{"connect", "--session", "aft-" + p.Name, "--dir", p.Dir, p.Host}, os.Stdout)
			if err == nil {
				ssh, err = guardProject(ssh, p)
			}
			if err != nil {
				fmt.Fprintln(os.Stderr, "aft:", err)
				return 1
			}
			return executeSSH(ssh)
		}
		if handler != nil {
			if err := handler(args[1:]); err != nil {
				fmt.Fprintln(os.Stderr, "aft:", err)
				return 1
			}
			return 0
		}
	}
	if len(args) >= 2 && args[0] == "ports" && (args[1] == "list" || args[1] == "watch") {
		if err := runPorts(args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, "aft:", err)
			return 1
		}
		return 0
	}
	ssh, err := sshArgs(args, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "aft:", err)
		return 2
	}
	if ssh == nil {
		return 0
	}
	return executeSSH(ssh)
}

// executeSSH owns the SSH child, reaps it on interruption, and preserves remote exit status.
func executeSSH(ssh []string) int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ssh", ssh...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			if exit.ExitCode() >= 0 {
				return exit.ExitCode()
			}
			return 1
		}
		fmt.Fprintln(os.Stderr, "aft:", err)
		return 1
	}
	return 0
}

// main is the process boundary; helpers remain testable without network access.
func main() { os.Exit(run(os.Args[1:])) }
