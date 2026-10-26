# Testing

Run `./x check` for formatting, vet, race-enabled tests, documentation checks and
a build. Tests need no server and no credentials.

## What the automated tests cover

- **Emulator** (`internal/vt`): basic operations, wrapping, scrollback, erase,
  attributes, alternate screen, modes, wide characters, the drawn caret, and
  query answers. A render round trip replays `Render` output into a fresh
  emulator and expects an identical screen, cursor and modes. A differential
  test compares screen text and cursor against pyte, an independent emulator,
  on output recorded from Claude Code, Codex, opencode, pi and a shell.
  Recordings live in `internal/vt/testdata`; regenerate them with a pseudo-
  terminal harness when agents change materially.
- **Input tokenizer** (`internal/terminal`): escape sequences, bracketed paste,
  multibyte characters and split sequences are never mistaken for typed text,
  and every byte is forwarded in order.
- **Prediction** (`internal/terminal`): a typed character is visible before the
  echo and the echo leaves the screen as if nothing had been predicted; the
  cursor is restored before server output; a prediction the server never
  confirms is repainted and lowers confidence; Backspace undoes a prediction;
  the drawn caret used by pi moves correctly; passthrough is exact when
  prediction is off.
- **Session** (`internal/session`): a real detached session is started, typed
  into, dropped, reattached at a new size with a repaint carrying earlier
  output, and ended with an Exit frame. A detached application's cursor-position
  query is answered. A second client takes over and the first is told.
- **End to end** (`cmd/aft`): the real client runs a shell session under a
  pseudo-terminal on the local host; the relay is killed as a dropped
  connection would be; the client reconnects, the screen is intact and the
  shell is alive. Detach from another terminal and reattach with `aft attach`.
  A shell prints a marker that scrolls off, the prefix opens copy mode, `g`
  jumps to the top, and the marker is back on screen with the status bar.
- **Prediction timing** (`internal/terminal`): a predicted character is written
  before `Key` returns, so typing is drawn at local speed with no wait on the
  network.
- **Bridge** (`internal/bridge`): the open-url bridge rejects non-web schemes;
  the clipboard reader runs the local tool, and only right after a paste; the
  `open` shim turns a URL into an open-url request on the session bridge
  socket; a pasted list of local files is recognized while ordinary text is
  left alone; and a stored pasted file stays private and is pruned after a week.

## Interactive checks

```sh
./x setup
./x try
./x try cli
./x try projects
```

`./x setup` builds Aft and runs setup with a temporary home and configuration.
Detected executables are selectable, but existing agent configuration directories
are hidden by the temporary home. Skip the SSH destination prompt: remote
operations are disabled in this preview. The resulting config is printed before
the temporary home is removed. To configure your actual installation, run
`./bin/aft setup`.

`./x try` opens a plain text menu. Pass a check name to run it directly:
`setup`, `cli`, `projects`, `shell`, `latency`, `reattach`, `scrollback`,
`clipboard`, `paste`, `url`, `listeners`, `agent`, or `hosts`.
The checks use private Aft state and a temporary project. Each terminal client runs in a separate process so input returns to the menu
cleanly. Test sessions end when the harness exits; existing Aft sessions and configuration stay intact.

`cli` previews help, listings, status and diagnostics. `projects` checks on/off
and subfolder matching. `latency` compares prediction on and off at a simulated
300 ms round trip. Run terminal checks on the computer hosting your terminal;
an SSH connection adds its own latency.

`clipboard` writes a sample to your clipboard; `url` opens a local test page
through the session bridge. `paste` provides a file and shell for text, image
and file paste checks. `agent` runs an installed agent with its normal account
configuration in the temporary project. These actions run only when selected.
`listeners` checks local listener discovery, not remote port forwarding.
`hosts` reads server information from configured hosts without installing or
repairing anything.

## Manual checks before a release

1. On a Mac with a Linux server and a macOS server: `aft setup`, `aft on`,
   launch Claude Code, Codex, opencode and pi. Type at a 100 ms or worse round
   trip and judge whether typing feels local.
2. Close the laptop lid mid-session, reopen, confirm the reattach.
3. Browser login from a fresh server account; image paste; copying text from
   the agent to the local clipboard; a dev server reachable on local
   `localhost`.
4. `aft off` and confirm the local agent runs; `aft uninstall` and confirm
   both machines are as they were apart from your code.

The real OpenSSH path has been exercised against a local sshd: installing the
embedded server, `aft on`, a full interactive session, listener discovery from
`/proc/net/tcp`, the `ssh -O forward` primitive mirroring uses, and
`aft uninstall` removing the server home. Automated checks do not replace
hands-on testing between physical machines.

Record OS and tool versions, source commit and outcomes in the review; never
commit private hosts, logs or machine-specific paths.

## Release artifact checks

`./x release-check v0.1.0` builds matching client and embedded-server versions
for all four targets. It inspects platform, CGO-free build settings and source
revision, verifies archive contents and checksums, then extracts the native
archive and checks `aft version` and the matching embedded server's metadata.

The native executable runs through the existing terminal tests for independent
shells, project visibility, reconnect and copy mode. It also exercises
setup/on/off/uninstall in a throwaway home, checking that shell configuration,
SSH configuration, agent data and project files are restored byte for byte.
These checks use the reserved `local` host and require no SSH service or
credentials. CI runs native artifact checks on Linux and macOS. They do not
replace real-host browser login, clipboard or uninstall checks.
