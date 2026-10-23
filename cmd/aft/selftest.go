package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/arocomputer/aft/internal/bridge"
	"github.com/arocomputer/aft/internal/client"
	"github.com/arocomputer/aft/internal/ports"
	"github.com/arocomputer/aft/internal/proto"
	"github.com/arocomputer/aft/internal/server"
	"github.com/arocomputer/aft/internal/session"
	"github.com/arocomputer/aft/internal/transport"
)

// cmdSelftest is a guided, hands-on test harness. It runs everything on this
// machine as its own server, in a throwaway AFT_HOME, so it needs no SSH, no
// configuration, and never touches your real setup. Each option opens a real
// aft session you drive yourself.
func cmdSelftest(args []string) int {
	// Use a short base for the throwaway home: Unix socket paths are limited to
	// ~104 bytes, and the default temp dir on macOS (/var/folders/...) is long
	// enough to overflow a session's socket path.
	base := ""
	if _, err := os.Stat("/tmp"); err == nil {
		base = "/tmp"
	}
	tmp, err := os.MkdirTemp(base, "aft-selftest-")
	if err != nil {
		return fail(err)
	}
	os.Setenv("AFT_HOME", tmp)
	defer func() {
		endAllSessions()
		os.RemoveAll(tmp)
	}()
	const prefix = 0x1d // Ctrl-]

	if os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_TTY") != "" {
		fmt.Print(`
!! You are inside an SSH session.

   Prediction runs wherever aft runs, so testing it here measures your SSH
   link to this machine, not aft. Run this on your own computer (your Mac),
   where your terminal is, for an honest result.

`)
	}

	fmt.Print(`
aft selftest — everything runs on this machine, in a throwaway home.
Nothing here touches your real config, dotfiles or sessions.

The aft prefix in these sessions is Ctrl-]. After it:
  Ctrl-] then [   open copy mode (scrollback)
  Ctrl-] then d   detach (the session keeps running)

`)
	for {
		fmt.Print(`Choose a test:
  1  Typing feel     — type over a slow link, with prediction on then off
  2  Reattach        — detach a running session and come back to it
  3  Scrollback      — enter copy mode and scroll, search and copy
  4  Clipboard       — a copy on the "server" lands on your clipboard
  5  Run an agent    — open a real agent through aft
  q  Quit

> `)
		switch readChoice() {
		case "1":
			selftestTyping(prefix)
		case "2":
			selftestReattach(prefix)
		case "3":
			selftestScrollback(prefix)
		case "4":
			selftestClipboard(prefix)
		case "5":
			selftestAgent(prefix)
		case "q", "quit", "exit", "":
			fmt.Println("Done. Cleaning up the throwaway home.")
			return 0
		default:
			fmt.Println("Pick 1-5 or q.")
		}
	}
}

// readChoice reads one line from stdin a byte at a time, so it never buffers
// input that the interactive sessions below need to read themselves.
func readChoice() string {
	var b []byte
	buf := make([]byte, 1)
	for {
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			if buf[0] == '\n' {
				break
			}
			if buf[0] != '\r' {
				b = append(b, buf[0])
			}
		}
		if err != nil {
			break
		}
	}
	return strings.ToLower(strings.TrimSpace(string(b)))
}

func pause() {
	fmt.Print("\nPress Enter for the menu. ")
	readChoice()
}

func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "/"
	}
	return h
}

// runShell opens a login shell as a local session with the given options.
func runShell(startup string, predict bool, latency int, prefix byte) {
	cmd := []string{"sh", "-c", `exec "${SHELL:-/bin/sh}"`}
	if startup != "" {
		cmd = []string{"sh", "-c", startup + `; exec "${SHELL:-/bin/sh}"`}
	}
	client.Run(client.Options{
		Host:      transport.Local,
		Command:   cmd,
		Dir:       homeDir(),
		Predict:   predict,
		LatencyMS: latency,
		Prefix:    prefix,
		Ports:     ports.Range{Min: 3000, Max: 9999},
		Quiet:     true,
	})
}

func selftestTyping(prefix byte) {
	fmt.Print(`
A shell opens with about 150 ms of delay each way (300 ms round trip),
the kind of lag a distant server has, and PREDICTION ON. Type a sentence.
It should appear instantly. Type  exit  to come back.
`)
	pause()
	runShell("", true, 150, prefix)
	fmt.Print(`
Now the SAME delay with PREDICTION OFF, so you can feel the difference.
Type the same sentence; each key waits for the round trip. exit to come back.
`)
	pause()
	runShell("", false, 150, prefix)
	pause()
}

func selftestReattach(prefix byte) {
	fmt.Print(`
A shell opens. Start something that keeps running, for example:
    while true; do date; sleep 1; done
Then press  Ctrl-]  then  d  to detach. The session keeps running.
`)
	pause()
	runShell("", true, 0, prefix)
	list, _ := session.List()
	if len(list) == 0 {
		fmt.Println("\nNo running session found (did you exit instead of detaching?).")
		pause()
		return
	}
	id := list[len(list)-1].ID
	fmt.Printf("\nStill running: %s. Reattaching now — your loop kept going.\n", id)
	fmt.Println("Type  exit  (after Ctrl-C) to stop it and come back.")
	pause()
	client.Run(client.Options{Host: transport.Local, SessionID: id, Predict: true, Prefix: prefix, Ports: ports.Range{Min: 3000, Max: 9999}, Quiet: true})
	pause()
}

func selftestScrollback(prefix byte) {
	fmt.Print(`
A shell opens after printing 200 numbered lines, most scrolled off.
  Press  Ctrl-]  then  [   to enter copy mode.
  Scroll: arrows, PgUp/PgDn, g (top), G (bottom).
  Search: / then text, Enter; n / N for next and previous.
  Copy:   v to start selecting, move, y to copy to your clipboard.
  Quit copy mode: q.    Leave the shell: exit.
`)
	pause()
	runShell("printf 'MARKER: the top of the scrollback\\n'; seq 1 200", true, 0, prefix)
	pause()
}

func selftestClipboard(prefix byte) {
	if !bridge.ClipboardAvailable() {
		fmt.Println("\nNo clipboard tool found. Install pbcopy (macOS), wl-clipboard or xclip, then retry.")
		pause()
		return
	}
	const phrase = "aft clipboard works!"
	// base64 of the phrase, emitted as OSC 52 by the "server".
	b64 := "YWZ0IGNsaXBib2FyZCB3b3JrcyE="
	fmt.Printf(`
A command on the server will copy %q using the OSC 52 escape.
aft sets your local clipboard from it, even if your terminal blocks OSC 52.
When the session closes, paste (Cmd+V / Ctrl+Shift+V) somewhere to check.
`, phrase)
	pause()
	startup := fmt.Sprintf(`printf '\033]52;c;%s\033\\'; echo 'Copied on the server. Close this (exit) and paste somewhere.'`, b64)
	runShell(startup, true, 0, prefix)
	fmt.Println("\nPaste now — you should get:", phrase)
	pause()
}

func selftestAgent(prefix byte) {
	info := server.GatherInfo()
	var found []string
	for _, a := range []string{"claude", "codex", "opencode", "pi", "gemini", "amp", "aider", "goose"} {
		if _, ok := info.Agents[a]; ok {
			found = append(found, a)
		}
	}
	if len(found) == 0 {
		fmt.Println("\nNo agents found on this machine. Install one (claude, codex, opencode, pi).")
		pause()
		return
	}
	fmt.Printf("\nAgents found: %s\nType one to run it through aft (or Enter to cancel): ", strings.Join(found, ", "))
	choice := readChoice()
	if choice == "" {
		return
	}
	if _, ok := info.Agents[choice]; !ok {
		fmt.Println("Not in the list.")
		pause()
		return
	}
	fmt.Printf("\nRunning %s through aft. Everything it does runs on this machine.\nTry copy mode (Ctrl-] [) and the clipboard. Exit the agent to come back.\n", choice)
	pause()
	client.Run(client.Options{
		Host:    transport.Local,
		Command: []string{choice},
		Dir:     homeDir(),
		Predict: true,
		Prefix:  prefix,
		Ports:   ports.Range{Min: 3000, Max: 9999},
		Quiet:   true,
	})
	pause()
}

// endAllSessions stops every session in the throwaway home.
func endAllSessions() {
	list, _ := session.List()
	for _, r := range list {
		session.Control(r.ID, proto.End)
	}
}
