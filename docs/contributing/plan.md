# Aft plan

Status: ready to build, decided. Revision 4 records the decisions made on
2026-10-23 and replaces every earlier revision. No product code has been
changed for this plan.

## 1. What Aft is

**The agent runs on your server. The terminal you type into stays on your
laptop, and typing feels local. One command turns it on and one turns it
off.**

```sh
aft setup          # once: add a server, install launchers
cd ~/code/app
aft on             # agents started here now run on the server
claude             # same command as always; also codex, opencode, pi, any terminal program
aft off            # back to fully local
```

When a project is on, an agent starts on the server inside an Aft session,
and your terminal attaches to it. Keystrokes are predicted and drawn on your
machine the instant you type. Everything else happens on the server: the
agent, its tools, builds, tests, language servers, MCP servers and git.
Closing the laptop does not stop the agent. Reopening it reattaches you.

Principles that every decision is checked against:

1. **Typing feels local.** A typed character appears in about a
   millisecond, however far away the server is.
2. **Agents never notice.** Aft adds no hooks, plugins or settings inside
   any agent. Any terminal program works.
3. **The work lives on the server.** Aft never syncs or copies code.
4. **On and off are one command each.** Off means exactly what you had
   before Aft.
5. **The work survives the laptop.** Sessions outlive disconnects, sleep and
   network changes.
6. **Aft leaves nothing behind.** `aft uninstall` removes every file and
   every line Aft added, on every machine. Only your code and your agents'
   own data stay.
7. **Your SSH, your machines.** Authentication and host trust stay in
   OpenSSH. Aft never runs sudo and never installs third-party software.

Platforms: macOS and Linux, as both client and server. Windows users run Aft
inside WSL, which counts as Linux. Native Windows is out of scope.

## 2. Decisions

| # | Decision | Detail |
|---|---|---|
| 1 | Aft builds its own predictive terminal and its own session | No mosh, no tmux. See sections 4 and 5 |
| 2 | Code lives only on the server. There is no sync logic of any kind | See section 6 |
| 3 | A small Aft server runs on each host | It is the session and the server half of the bridges |
| 4 | Aft adds the PATH line to the user's shell config itself | `aft uninstall` removes it exactly. See section 8 |
| 5 | Third-party dependencies are allowed | The old no-dependency rule is removed from the README, guides and AGENTS.md |
| 6 | macOS and Linux, client and server | Both are tested in CI |

## 3. Evidence gathered before deciding

These tests ran against the installed agents: Claude Code 2.1.307, Codex
0.163.0, opencode 2.1.2 and pi 1.4.0. The test typed into each agent's
prompt and never pressed Enter.

**Every keystroke was predictable.** A harness typed 18 characters and 3
backspaces into each agent. It checked that each character landed exactly
at the caret and that nothing else on screen changed. The result was the
same with each agent run directly and inside tmux.

| Agent | Caret | Correct echoes | Other cells changed per key |
|---|---|---|---|
| Claude Code | terminal cursor | 18/18, backspace 3/3 | 0 (median) |
| Codex | terminal cursor | 18/18, 3/3 | 0 |
| opencode | terminal cursor | 18/18, 3/3 | 0 |
| pi | drawn caret, cursor hidden | 18/18, 3/3 | 0 |

**Prediction hid the latency.** mosh uses the same prediction method. It was
run through a relay that added a 120 ms round trip. The table shows the
median time for a typed character to appear.

| Agent | Prediction off | Prediction on | Final text correct |
|---|---|---|---|
| Claude Code | 134 ms | 0.4 ms | yes |
| Codex | 153 ms | 0.4 ms | yes |
| pi | 132 ms | 0.4 ms | yes |

Only the first one or two keystrokes after a pause took the full round trip.
That delay comes from mosh confirming the app echoes before it predicts. Aft
can predict from the first keystroke, because it knows when the agent is
waiting at its prompt.

**Terminal modes.**

- Claude Code, Codex and opencode run full-screen and capture the mouse.
- pi prints into normal scrollback.
- All four use the kitty keyboard protocol, which is what makes keys such as
  Shift+Enter work, and bracketed paste.
- The clipboard escape (OSC 52) and the bell passed through to the outer
  terminal.

**Limits.**

- The tests ran on localhost with no packet loss.
- Keystrokes were spaced 0.4 s apart.
- The terminal size was fixed at 100 by 30.
- No person has yet judged the feel on a real network. Phase 2 ends with
  that check.

## 4. The session replaces tmux

tmux can be configured to be invisible, so its appearance alone is not the
reason to replace it. The reasons are these:

- **tmux is a second terminal between the agent and your screen.** It
  re-renders everything, so features pass through only when tmux understands
  them. Its kitty keyboard support is partial, it drops kitty graphics, and
  hyperlinks and synchronized output depend on configuration.
- **tmux breaks native scrollback** for apps such as pi that print into it.
- **tmux would be a server dependency** that users must install. On macOS
  servers that means Homebrew. Aft never installs third-party software.
- **Aft already needs a terminal emulator.** The client needs one for
  prediction. The session needs exactly the same component.

The Aft session is part of the server. Per session it holds three
things: a pseudo-terminal running the agent, a terminal emulator fed
everything the agent prints, and a bounded scrollback.

- **Attached.** Output passes byte-for-byte to the client and on to your
  terminal app. Every feature the agent and your terminal negotiate works
  untouched, including keyboard protocols, images, hyperlinks, mouse and
  colors. Native scrollback works, including for pi.
- **Detached.** The agent keeps running. The emulator keeps tracking the
  screen. It answers the terminal queries an app sends, such as cursor
  position and device attributes, so the agent never blocks waiting for a
  terminal.
- **Reattached.** The session replays recent scrollback for apps on the
  normal screen. It restores terminal modes: alternate screen, mouse,
  bracketed paste, keyboard protocol flags and cursor visibility. It paints
  the current screen, then sends the app a resize signal so it redraws
  itself.
- **Invisible.** There is no status bar and no on-screen chrome while
  things work. Status shows in the terminal title, and only while the
  connection is down, or a pasted file could not be sent, as one line across
  the top of the screen that goes away by itself.
- **One client at a time.** A new attach takes over from the old one.
- **Detaching.** A session detaches when its terminal goes away: the window
  closes, another computer attaches, or the laptop sleeps. Aft does not steal
  a key from the agent by default; a prefix key (Ctrl-] then d) is on, and a
  direct detach key can be enabled in config.

## 5. The terminal client and prediction

The client runs on your machine between your terminal app and the SSH
channel.

- **Passthrough first.** Server output is written unchanged. The client never
  re-renders the app.
- **Shadow screen.** A terminal emulator fed the same bytes tracks what the
  server has drawn, the cursor and the current text style.
- **Prediction.** When you type a printable character or press Backspace,
  the client draws the result at the caret immediately. The caret is the
  terminal cursor, or for apps such as pi, the single drawn caret cell. The
  client then restores the cursor and text style from the shadow screen, so
  the next server output lands exactly where the server expects.
- **Reconciliation.** The server's echo confirms each prediction. Any wrong
  cell is repainted from the shadow screen.
- **Confidence.** Predictions show immediately while the app is known to
  echo. They pause when echoes stop matching, for example at a password
  prompt or in a menu, and resume once echoes match again. Enter, arrow
  keys, control keys and pastes are never predicted.
- **Reconnect.** A dropped connection is detected through SSH keepalives.
  The client reconnects and reattaches. The title shows the state. Typed
  input is never lost silently: it is either delivered or reported.

Still bound by network latency: menus, autocomplete popups, and the agent's
response after Enter. The response is dominated by model time.

One terminal emulation library serves both sides, the client's shadow
screen and the session's emulator. Phase 2 starts by choosing it, against
four tests: mode tracking, query answering, rendering a screen back out to
escape codes, and correctness on recorded agent output.

## 6. Where code lives

All work lives on the server. Aft has no sync logic and none is planned.

- **Commits and pushes happen on the server.** The agent or you make them
  there, with the server's git credentials, as in any remote dev setup.
- **Code reaches the server once, by git.** `git clone` runs on the server,
  as a tool or from `aft HOST`, never as a copy from your machine.
- **The local directory is only a handle.** It tells Aft which server
  project to use. It can be a full local checkout or an empty folder.
  `aft on` is exactly the current folder. It never creates a server folder
  it guessed, only one named with `--dir`.
- **The server path defaults to the same path relative to home.** For
  example, `~/code/app` maps to `~/code/app`. `aft on --dir` overrides it.

## 7. Architecture

```
 Your machine                                      Server
 ────────────                                      ──────
 terminal app (Ghostty, iTerm, Terminal, any)
   ▲ draws            │ keys
   │                  ▼
 aft client                                        aft server
   ├─ passthrough + shadow screen                    ├─ session
   ├─ prediction overlay                             │    ├─ pty → claude │ codex │ any
   ├─ reconnect, reattach, resize                    │    ├─ emulator + scrollback
   ├─ bridge server: open URL, clipboard ◄────────── ├─ bridge shims on the session PATH
   └─ port mirror on 127.0.0.1 ◄──────────────────── └─ listener discovery
        one OpenSSH master connection per host carries everything
```

**Transport.** Aft uses system OpenSSH with one master connection per host.
Keys, ProxyJump, agent, config and host trust all come from the user's
setup. The connection carries four things:

- **A session channel,** opened as
  `ssh -tt host ~/.local/share/aft/aft-server attach ID`.
- **The bridge socket,** a Unix socket forwarded back to your machine.
- **Port mirrors,** added on the live connection with `ssh -O forward`.
- **Short server calls,** such as listing sessions.

The transport is TCP. Prediction hides network stalls for typing, but not
for output. UDP can be considered later if lossy networks prove a problem.

**Server install.** Aft copies its own static binary for the server's OS and
architecture to `~/.local/share/aft/aft-server` on the server. It verifies the hash and pins
the binary to the client's version. macOS arm64 builds are signed ad hoc by
the Go linker. A binary copied over SSH carries no quarantine flag.

**Listener discovery.** On Linux, the server reads `/proc/net/tcp`. On
macOS, it parses the built-in `lsof`.

## 8. Launchers, on/off, and the shell line

**Launchers.** `~/.local/share/aft/launchers/` holds one symlink per agent name, each
pointing at `aft`. When `aft` is invoked as `claude`, it checks whether the
current directory is inside a project that is on. If it is, it starts
`claude` on the server, in the matching directory, with the arguments passed
through literally. If not, it runs the real local `claude`, found on PATH
with the shims directory skipped. The local path adds a few milliseconds.

`aft setup` detects installed agents: Claude Code, Codex, opencode, pi, and
others such as Gemini CLI, Amp, Aider and Goose. `aft agents add NAME`
covers any other program. `aft HOST` starts your login shell on the server
the same way, as a lag-free replacement for SSH and mosh.

**The shell line.** `aft setup` adds a marked block to each shell config it
finds:

```sh
# >>> aft >>>
export PATH="$HOME/.local/share/aft/launchers:$PATH"
# <<< aft <<<
```

- **zsh and bash** get the block in `~/.zshrc`, `~/.bashrc` or
  `~/.bash_profile`, whichever is in use.
- **fish** gets a separate file, `~/.config/fish/conf.d/aft.fish`, rather than
  an edit to the user's config.
- **Before editing a file,** Aft writes a backup and records the file path
  and the exact block it added in `~/.aft/state`.

**Environment.** Only an allowlist crosses to the server: `TERM`,
`COLORTERM`, `TERM_PROGRAM`, locale variables, and anything in `env.pass`.
Everything else comes from the server login environment.

## 9. Uninstall leaves nothing behind

`aft uninstall` does the following, in order.

1. Lists everything it will remove and every running Aft session, then asks
   for confirmation.
2. Ends the Aft sessions on every known host. These are sessions Aft created.
3. Removes `~/.aft/` and `~/.local/share/aft/` on every known host. They hold
   the logs, the server, session records and sockets.
4. Removes the shell block from each recorded file. It removes the block only
   if it still matches exactly what Aft wrote. If the user edited it, Aft
   shows the block and leaves it for the user to remove. It deletes the
   fish drop-in file.
5. Removes the local `~/.aft/`, which holds config and state, and
   `~/.local/share/aft/`, which holds the launchers and control sockets.
6. Tells the user how they installed the `aft` binary, so they can remove it.

What stays: code on the server, your SSH config and known hosts, and each
agent's own login and data. Those were never Aft's.

A test enforces this. It installs, turns a project on, runs sessions, then
uninstalls. It then checks that every file on both machines is
byte-identical to before, except the code.

## 10. Bridges

| Feature | How | Phase |
|---|---|---|
| Copy from the agent to your clipboard | OSC 52 passes straight through to your terminal | Free with passthrough |
| Bell and notifications (OSC 9, OSC 777) | Pass straight through | Free with passthrough |
| Opening a URL, such as a browser login | `open` and `xdg-open` shims on the session PATH send URLs to your machine. Anything that isn't a URL goes to the real tool | 4 |
| Login callbacks on `localhost:PORT` | The port mirror makes the server port reachable from your browser | 4 |
| Pasting an image into the agent | Clipboard shims (`pbpaste`, `wl-paste`, `xclip`) fetch your local clipboard through the bridge | 4, verify each agent |
| Dev servers and previews | Server listeners in range are mirrored to the same port on local `127.0.0.1` | 4 |
| Dragging a local file into the terminal | Upload the file and paste its server path | Later |

Agent logins and config, including `CLAUDE.md`, skills and MCP servers, live
on the server. An agent's first launch on a server uses its normal login
flow, through the URL and port bridges. Aft never installs agents.
`aft doctor` reports which agents are missing on each server.

## 11. CLI

```
aft setup                    add a server, create launchers, add the PATH block
aft on [--host H] [--dir D]  agents started here run on the server
aft off                      agents started here run locally
aft HOST [CMD...]            a lag-free terminal on that host, or one command
aft ls                       sessions on all hosts
aft attach [NAME]            reattach
aft kill NAME                stop a session
aft status                   connection, sessions, mirrored ports
aft doctor                   SSH, server, agents on the host, launchers, shell block
aft agents add NAME          launch another program this way
aft uninstall                remove everything Aft added, everywhere
aft completion bash|zsh|fish
```

The server runs on the host as `aft-server`, its own binary, as mosh-server
is mosh's, with the subcommands `attach`, `new`, `udp`, `ls`, `end`,
`remains` and `listeners`, plus the bridge shims. For the host `local`, the
aft binary is the server when started under the name `aft-server`.

## 12. Settings

`~/.aft/config.toml`:

```toml
use = "box"
agents = ["claude", "codex", "opencode", "pi"]
predict = true
detach = ""                  # off by default; nothing is stolen from agents
env = []
ports = "3000-9999"

[host.box]
ssh = "box"                  # alias from ~/.ssh/config
```

Which projects are on, and where each maps, is local state in `~/.aft/`.
A repository may also commit an optional `aft.toml` so a team shares the same
host name, server path and port range.

## 13. Code layout

The rewrite happens on a new branch. Current commands are deleted, not
ported. These principles carry over from the current code:

- input validation, and treating remote data as untrusted
- private, atomic metadata writes
- loopback-only listeners
- reaping owned child processes
- fake-executable tests

Three parts, terminal, session and bridge, plus supporting packages:

```
internal/terminal/          on your machine: passthrough, shadow screen, prediction, reconnect, pasting, copy mode
internal/session/           on the server: pty, emulator, scrollback, query answering, reattach, server CLI, listeners
internal/session/install/   embedded server builds and their install on a host
internal/bridge/            between the two: clipboard (copy, read on paste, pasted files), open-URL, shims
internal/bridge/transport/  OpenSSH master, channels, forwards
internal/bridge/proto/      frames between terminal and session
internal/bridge/ports/      port mirror
cmd/aft/                    CLI dispatch, argv0 launcher entry
internal/config/            config, aft.toml, state
internal/vt/                shared terminal emulation
internal/launcher/          on/off decision, shell block install and removal
internal/home/              where Aft writes on each machine
```

Removed from the current code entirely:

- workspaces and the project records
- `connect`, `open`, `sessions` and `run`
- jobs, logs, clip, mount and services
- the `uninstall` dry run
- migration
- the picker
- the `ss` dependency

The uncommitted feature work in this worktree is superseded and will not
be merged.

## 14. Reliability contract and tests

**Typing.**

- A predicted character appears within 5 ms.
- After reconciliation, no wrong cell remains. This is tested with random key
  sequences against apps that echo, do not echo, open menus, or prompt for
  passwords.
- With prediction off, output matches passthrough exactly.

**session.**

- Reattach restores the screen, the terminal modes and scrollback.
- While detached, terminal queries are answered.
- Sessions survive a client crash, a killed connection and a simulated
  sleep.

**Fidelity.**

- Shift+Enter and other kitty-protocol keys work.
- Bracketed paste, mouse, true color, hyperlinks and synchronized output
  work.
- Resize works.
- The section 3 echo harness runs against recorded output from all four
  agents.

**Network.** Tests run with simulated latency at 50, 120 and 250 ms round
trip, with 1% packet loss, on Linux CI.

**Launchers.**

- With the project off, the launcher runs the real binary and adds under
  5 ms.
- With the project on, arguments reach the server literally.

**Uninstall.** Both machines end byte-identical to before install, except the
code. An edited shell block is never deleted.

**Platforms.** CI covers a Linux client with a Linux server, a macOS client
with a Linux server, and a macOS client with a macOS server.

## 15. Build order

| Phase | Delivers | Exit criteria |
|---|---|---|
| 1 | SSH master, server install, session with passthrough, attach, detach, ls, reconnect | `claude` runs on a server, detaches, survives a killed connection, and reattaches with the screen restored. Typing still has full latency |
| 2 | Terminal library choice, shadow screen, prediction, query answering | Typing and fidelity tests pass. **Go or no-go:** a person types into Claude Code at a 120 ms round trip and it feels local |
| 3 | Launchers, shell block, `aft setup`, `on`, `off`, `ssh` | The same `claude` command runs locally or remotely depending on on/off |
| 4 | Bridges and port mirror | Browser login, image paste and a dev server on local `localhost` all work |
| 5 | `uninstall`, `doctor`, `status`, docs, macOS server, release checks | The uninstall test passes on both platforms. Docs describe only this model |

## 16. Risks

- **Prediction edge cases,** such as vim-mode inputs, popups and password
  prompts. Mitigations: confidence gating, never predicting non-printing
  keys, and randomized reconciliation tests.
- **Reattach fidelity.** Images and some app state may not survive a
  reattach. Mitigation: after repainting, signal the app to redraw itself,
  which most agents do completely.
- **Query timing at launch.** Agents probe the terminal at startup. A launch
  always happens with a client attached, so your real terminal answers. While
  detached, the session answers.
- **Terminal library fit.** It must track modes and render back out to escape
  codes. Mitigation: choose it by test at the start of Phase 2. Writing a
  focused emulator is the fallback.
- **TCP stalls on lossy networks.** Output is delayed, but typing is not.
  Mitigation: measure with simulated loss; UDP is a later option.
- **Image paste on macOS servers.** Some agents read the clipboard through
  `osascript`, which cannot be shimmed safely. Mitigation: verify each agent
  in Phase 4, and document any gaps.
- **Shell config edits.** Mitigation: backups, an exact-match removal rule,
  and the byte-identical uninstall test.
