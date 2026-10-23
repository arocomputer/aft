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
- **Input tokenizer** (`internal/client`): escape sequences, bracketed paste,
  multibyte characters and split sequences are never mistaken for typed text,
  and every byte is forwarded in order.
- **Prediction** (`internal/client`): a typed character is visible before the
  echo and the echo leaves the screen as if nothing had been predicted; the
  cursor is restored before server output; a prediction the server never
  confirms is repainted and lowers confidence; Backspace undoes a prediction;
  the drawn caret used by pi moves correctly; passthrough is exact when
  prediction is off.
- **Berth** (`internal/helper`): a real detached session is started, typed
  into, dropped, reattached at a new size with a repaint carrying earlier
  output, and ended with an Exit frame. A detached application's cursor-position
  query is answered. A second client takes over and the first is told.
- **End to end** (`cmd/aft`): the real client runs a shell session under a
  pseudo-terminal on the local host; the relay is killed as a dropped
  connection would be; the client reconnects, the screen is intact and the
  shell is alive. Detach from another terminal and reattach with `aft attach`.
  A shell prints a marker that scrolls off, the prefix opens copy mode, `g`
  jumps to the top, and the marker is back on screen with the status bar.
- **Prediction timing** (`internal/client`): a predicted character is written
  before `Key` returns, so typing is drawn at local speed with no wait on the
  network.
- **Bridges** (`internal/bridge`, `internal/helper`): the open-url bridge
  rejects non-web schemes; the clipboard reader runs the local tool; and the
  `open` shim turns a URL into an open-url request on the session bridge socket.

## The guided self-test

`./x try` opens the interactive test command center (the `aft selftest`
command). It runs everything on this machine as its own server, in a throwaway
`AFT_HOME`, so it needs no SSH and never touches your real config, dotfiles or
sessions. Run it on the computer your terminal is on, not over SSH: prediction
runs wherever aft runs, so testing it through an SSH session measures that SSH
link, not aft. It warns you if it detects an SSH session. A menu
offers: typing feel over a simulated slow link with prediction on then off;
detach and reattach; copy mode and scrollback; the OSC 52 clipboard; and running
a real agent. Run it with `./x try`.

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
embedded helper, `aft on`, a full interactive session, listener discovery from
`/proc/net/tcp`, the `ssh -O forward` primitive mirroring uses, and
`aft uninstall` removing the server home. A test against a second physical
machine over the network is still the remaining check.

Record OS and tool versions, source commit and outcomes in the review; never
commit private hosts, logs or machine-specific paths.
