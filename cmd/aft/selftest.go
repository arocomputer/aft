package main

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/arocomputer/aft/internal/bridge"
	"github.com/arocomputer/aft/internal/bridge/ports"
	"github.com/arocomputer/aft/internal/bridge/proto"
	"github.com/arocomputer/aft/internal/bridge/transport"
	"github.com/arocomputer/aft/internal/config"
	"github.com/arocomputer/aft/internal/home"
	"github.com/arocomputer/aft/internal/launcher"
	"github.com/arocomputer/aft/internal/session"
	"github.com/arocomputer/aft/internal/session/install"
	"github.com/arocomputer/aft/internal/terminal"
)

// selftestNames lists checks available from the menu and ./x try TEST.
var selftestNames = []string{"setup", "cli", "projects", "shell", "latency", "reattach", "scrollback", "clipboard", "paste", "url", "listeners", "agent", "hosts"}

// cmdSelftest runs checks with private Aft state and a temporary workspace.
// Setup also isolates HOME and blocks SSH; agent checks use normal agent accounts.
func cmdSelftest(args []string) int {
	if len(args) > 0 && args[0] == "_terminal" {
		return selftestTerminalChild(args[1:])
	}
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Println("usage: aft selftest [test]")
		for _, name := range selftestNames {
			fmt.Println("  " + name)
		}
		return 0
	}
	if len(args) > 0 && args[0] == "setup" {
		return selftestSetup(args[1:])
	}
	if len(args) > 1 || (len(args) == 1 && !slices.Contains(selftestNames, args[0])) {
		return fail(fmt.Errorf("usage: aft selftest [%s]", strings.Join(selftestNames, "|")))
	}
	realConfig, configErr := config.Load()
	path := selftestPath(os.Getenv("PATH"), home.Launchers())
	directory, err := os.MkdirTemp("/tmp", "aft-try-")
	if err != nil {
		return fail(err)
	}
	previous, err := os.Getwd()
	if err != nil {
		os.RemoveAll(directory)
		return fail(err)
	}
	workspace := filepath.Join(directory, "project")
	if err := os.Mkdir(workspace, 0o700); err != nil {
		os.RemoveAll(directory)
		return fail(err)
	}
	workspace = config.RealPath(workspace)
	restore := selftestEnv(map[string]string{"AFT_HOME": filepath.Join(directory, "state"), "PATH": path, "AFT_SESSION": "", "AFT_REMOTE_HOME": ""})
	defer func() {
		endAllSessions()
		os.Chdir(previous)
		restore()
		os.RemoveAll(directory)
	}()
	if err := os.Chdir(workspace); err != nil {
		return fail(err)
	}
	manual := false
	if err := config.Save(config.Config{Agents: []string{}, EditDotfiles: &manual}); err != nil {
		return fail(err)
	}
	if err := config.SaveState(config.State{Projects: map[string]config.Project{workspace: {Host: "local", Dir: workspace}}}); err != nil {
		return fail(err)
	}
	const prefix = 0x1d
	run := func(name string) int {
		switch name {
		case "cli":
			return selftestCLI()
		case "projects":
			return selftestProjects()
		case "shell":
			return runShell("", true, 0, prefix)
		case "latency":
			return selftestTyping(prefix)
		case "reattach":
			return selftestReattach(prefix)
		case "scrollback":
			return selftestScrollback(prefix)
		case "clipboard":
			return selftestClipboard(prefix)
		case "paste":
			return selftestPaste(prefix)
		case "url":
			return selftestHTTP(prefix, true)
		case "listeners":
			return selftestHTTP(prefix, false)
		case "agent":
			return selftestAgent(prefix)
		case "hosts":
			if configErr != nil {
				return fail(configErr)
			}
			return selftestHosts(realConfig)
		case "setup":
			return selftestSetup(nil)
		}
		return 2
	}
	if len(args) == 1 {
		return run(args[0])
	}
	fmt.Printf("Workspace: %s\nState: %s\n", workspace, home.Dir())
	if os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_TTY") != "" {
		fmt.Println("SSH detected; latency measurements include the SSH connection.")
	}
	for {
		fmt.Println()
		for i, name := range selftestNames {
			fmt.Printf("%2d %s\n", i+1, name)
		}
		fmt.Print(" q exit\n\n> ")
		choice := strings.ToLower(readChoice())
		if choice == "q" || choice == "quit" || choice == "exit" || choice == "" {
			return 0
		}
		if index, err := strconv.Atoi(choice); err == nil && index > 0 && index <= len(selftestNames) {
			choice = selftestNames[index-1]
		}
		if !slices.Contains(selftestNames, choice) {
			fail(fmt.Errorf("unknown test %q", choice))
			continue
		}
		if code := run(choice); code != 0 {
			fmt.Printf("exit status: %d\n", code)
		}
		fmt.Print("\nEnter to return: ")
		readChoice()
	}
}

// readChoice leaves subsequent input available to an attached terminal session.
func readChoice() string {
	var result strings.Builder
	var buffer [1]byte
	for {
		count, err := os.Stdin.Read(buffer[:])
		if count > 0 {
			if buffer[0] == '\n' {
				break
			}
			if buffer[0] != '\r' {
				result.WriteByte(buffer[0])
			}
		}
		if err != nil {
			break
		}
	}
	return strings.TrimSpace(result.String())
}

// runShell starts a local session in the test workspace.
func runShell(startup string, predict bool, latency int, prefix byte) int {
	command := slices.Clone(session.ShellCommand)
	if startup != "" {
		command = []string{"sh", "-c", startup + `; exec "${SHELL:-/bin/sh}"`}
	}
	directory, err := os.Getwd()
	if err != nil {
		return fail(err)
	}
	return selftestTerminal(terminal.Options{Host: transport.Local, Command: command, Dir: directory, Predict: predict, LatencyMS: latency, Prefix: prefix, Ports: ports.Range{Min: 3000, Max: 9999}, Quiet: true})
}

// selftestTerminal gives each client its own process so blocked stdin readers
// cannot consume input after the check returns to the menu or reattaches.
func selftestTerminal(options terminal.Options) int {
	self, err := os.Executable()
	if err != nil {
		return fail(err)
	}
	args := []string{"selftest", "_terminal", "--session", options.SessionID,
		"--predict=" + strconv.FormatBool(options.Predict), "--latency=" + strconv.Itoa(options.LatencyMS), "--"}
	command := exec.Command(self, append(args, options.Command...)...)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return exit.ExitCode()
		}
		return fail(err)
	}
	return 0
}

// selftestTerminalChild is the private local-client entry used by terminal checks.
func selftestTerminalChild(args []string) int {
	fs := flags("selftest _terminal")
	id := fs.String("session", "", "session ID")
	predict := fs.Bool("predict", true, "enable prediction")
	latency := fs.Int("latency", 0, "simulated latency")
	if err := fs.Parse(args); err != nil {
		return flagExit(err)
	}
	directory, err := os.Getwd()
	if err != nil {
		return fail(err)
	}
	return terminal.Run(terminal.Options{Host: transport.Local, Command: fs.Args(), SessionID: *id, Dir: directory, Predict: *predict, LatencyMS: *latency, Prefix: 0x1d, Ports: ports.Range{Min: 3000, Max: 9999}, Quiet: true})
}

// selftestTyping compares prediction at a fixed simulated round-trip latency.
func selftestTyping(prefix byte) int {
	fmt.Println("Latency: 150 ms round trip. Prediction on. Exit to compare with prediction off.")
	if code := runShell("", true, 150, prefix); code != 0 {
		return code
	}
	fmt.Println("Latency: 150 ms round trip. Prediction off. Exit to finish.")
	return runShell("", false, 150, prefix)
}

// selftestReattach attaches only to the session created by this check.
func selftestReattach(prefix byte) int {
	before, _ := session.List()
	known := map[string]bool{}
	for _, record := range before {
		known[record.ID] = true
	}
	fmt.Println("Start a process, then Ctrl-] d to detach. The check reattaches the same session.")
	if code := runShell("", true, 0, prefix); code != 0 {
		return code
	}
	records, err := session.List()
	if err != nil {
		return fail(err)
	}
	for _, record := range records {
		if known[record.ID] {
			continue
		}
		fmt.Printf("Attaching %s. Exit the shell to finish.\n", record.ID)
		return selftestTerminal(terminal.Options{Host: transport.Local, SessionID: record.ID, Predict: true, Prefix: prefix, Ports: ports.Range{Min: 3000, Max: 9999}, Quiet: true})
	}
	fmt.Println("Session ended; nothing to reattach.")
	return 0
}

// selftestScrollback creates numbered output for copy-mode navigation and search.
func selftestScrollback(prefix byte) int {
	fmt.Println("Ctrl-] [ opens copy mode. g/G: top/end; /: search; v then y: select/copy; q: close. Exit finishes.")
	return runShell("printf 'scrollback marker\\n'; seq 1 200", true, 0, prefix)
}

// selftestClipboard exercises OSC 52 clipboard writes through a local session.
func selftestClipboard(prefix byte) int {
	if !bridge.ClipboardAvailable() {
		return fail(fmt.Errorf("clipboard command unavailable"))
	}
	const text = "aft clipboard test"
	fmt.Printf("Clipboard value: %s. Paste after leaving the shell to verify.\n", text)
	data := base64.StdEncoding.EncodeToString([]byte(text))
	return runShell(fmt.Sprintf(`printf '\033]52;c;%s\033\\'`, data), true, 0, prefix)
}

// selftestPaste supplies a file for checking bracketed paste and file/image paths.
func selftestPaste(prefix byte) int {
	directory, err := os.Getwd()
	if err != nil {
		return fail(err)
	}
	path := filepath.Join(directory, "paste.txt")
	if err := os.WriteFile(path, []byte("aft paste test\n"), 0o600); err != nil {
		return fail(err)
	}
	fmt.Printf("Paste text, an image, or this file: %s\nInspect pasted paths with cat or file. Exit finishes.\n", path)
	return runShell("", true, 0, prefix)
}

// selftestHTTP checks local listener discovery or URL delivery through a session.
// It does not claim to exercise SSH forwarding, which requires a remote host.
func selftestHTTP(prefix byte, openURL bool) int {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return fail(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/plain")
		fmt.Fprintln(writer, "aft HTTP test")
	})}
	defer server.Close()
	go server.Serve(listener)
	port := listener.Addr().(*net.TCPAddr).Port
	address := "http://" + listener.Addr().String()
	if !openURL {
		listening, err := session.Listeners()
		if err != nil {
			return fail(err)
		}
		if !slices.Contains(listening, port) {
			return fail(fmt.Errorf("listener %d was not detected", port))
		}
		fmt.Printf("Detected %s\n", address)
		return 0
	}
	fmt.Printf("Opening %s through the session URL bridge. Exit finishes.\n", address)
	return runShell("open "+transport.Quote(address), true, 0, prefix)
}

// selftestAgent uses an installed executable and its normal account configuration.
func selftestAgent(prefix byte) int {
	var found []string
	for _, name := range config.KnownAgents {
		if _, err := launcher.RealBinary(name); err == nil {
			found = append(found, name)
		}
	}
	fmt.Printf("Detected: %s\nAgent command (empty cancels): ", strings.Join(found, ", "))
	choice := readChoice()
	if choice == "" {
		return 0
	}
	if err := launcher.ValidName(choice); err != nil {
		return fail(err)
	}
	if _, err := launcher.RealBinary(choice); err != nil {
		return fail(err)
	}
	fmt.Println("Uses the agent's normal account configuration.")
	directory, err := os.Getwd()
	if err != nil {
		return fail(err)
	}
	return selftestTerminal(terminal.Options{Host: transport.Local, Command: []string{choice}, Dir: directory, Predict: true, Prefix: prefix, Ports: ports.Range{Min: 3000, Max: 9999}, Quiet: true})
}

// selftestHosts reads configured servers without installing or repairing them.
func selftestHosts(configuration config.Config) int {
	for _, name := range configuration.HostNames() {
		host, _ := configuration.Host(name)
		defer transport.CloseMaster(host)
		info, err := install.Info(host)
		if err != nil {
			return fail(fmt.Errorf("%s: %w", name, err))
		}
		fmt.Printf("%s: %s %s/%s\n", name, info.Version, info.OS, info.Arch)
	}
	if len(configuration.Hosts) == 0 {
		fmt.Println("No configured hosts.")
	}
	return 0
}

// selftestCLI previews command output using the test configuration.
func selftestCLI() int {
	for _, request := range [][]string{{}, {"help"}, {"help", "on"}, {"host"}, {"ls"}, {"status"}, {"doctor"}} {
		fmt.Print("$ aft")
		if len(request) > 0 {
			fmt.Print(" ", strings.Join(request, " "))
		}
		fmt.Println()
		code := 0
		if len(request) == 0 {
			printUsage(false)
		} else {
			code = run(request[0], request[1:])
		}
		if code != 0 {
			return code
		}
		fmt.Println()
	}
	return 0
}

// endAllSessions stops only sessions recorded in the active test AFT_HOME.
func endAllSessions() {
	records, _ := session.List()
	for _, record := range records {
		session.Control(record.ID, proto.End)
	}
}

// selftestEnv applies temporary process settings and returns their restoration.
func selftestEnv(values map[string]string) func() {
	previous := map[string]*string{}
	for key, value := range values {
		if old, ok := os.LookupEnv(key); ok {
			copy := old
			previous[key] = &copy
		} else {
			previous[key] = nil
		}
		os.Setenv(key, value)
	}
	return func() {
		for key, value := range previous {
			if value == nil {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, *value)
			}
		}
	}
}

// selftestPath excludes production launchers from discovery in an isolated home.
func selftestPath(value, launchers string) string {
	resolved, _ := filepath.EvalSymlinks(launchers)
	var directories []string
	for _, directory := range filepath.SplitList(value) {
		actual, _ := filepath.EvalSymlinks(directory)
		if filepath.Clean(directory) == launchers || (actual != "" && actual == resolved) {
			continue
		}
		directories = append(directories, directory)
	}
	return strings.Join(directories, string(os.PathListSeparator))
}
