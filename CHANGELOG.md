# Changelog

User-visible changes are recorded here. No public release has been published yet.

## Unreleased

Aft was rebuilt around one idea: the agent runs on the server, the terminal
stays on your machine, and typing feels local.

### New features

- Sessions hold an application in a pseudo-terminal on the server, survive a
  lost connection, repaint on reattach, and answer terminal queries while
  detached. No tmux is needed or used.
- Sending is asynchronous: the input loop queues keystrokes to a writer
  goroutine and never waits on the network, so backpressure or a slow link
  cannot delay your typing or its prediction.
- The client predicts typed characters and Backspace locally and reconciles them
  with the server's echo, so typing has no network latency. Everything else is
  passed through byte for byte.
- Launchers: `aft on` in a project makes `claude`, `codex`, `opencode`, `pi` and
  any other configured program run on the server; `aft off` makes them local
  again. Agents are not configured or wrapped.
- `aft setup` installs Aft's server on a host over SSH, creates launchers, and
  adds a marked PATH block to the shell config. `aft uninstall` removes all of
  it, on every machine, and leaves edited blocks alone.
- Bridges: URLs opened on the server open in the local browser; the local
  clipboard, including images, is readable from the server through shims,
  only in the seconds after you paste or press Ctrl-V.
- Copy mode: the prefix (Ctrl-] by default) then `[` opens a scrollback pager in
  the client over a snapshot from the server, with keyboard scroll, search and
  selection that copies to the local clipboard, and native drag-select.
- Predictions emit the fewest bytes: a bare character in steady typing, exactly
  like native echo, with no cursor-move or style reset, so there is no flicker.
  The prediction itself costs about 12 microseconds; felt latency is the
  terminal and display, which aft draws within.
- Typing ahead of the server's echo no longer makes the caret jump back: new
  predictions go after the ones still in flight, and agents that redraw the
  whole input line on each echo keep the in-flight characters on screen.
- The client draws each predicted keystroke before sending it to the network,
  and gives keystrokes priority over output, so neither a congested link nor a
  burst of agent output can stall your typing.
- `aft selftest`, opened with `./x try`, is a guided, isolated harness to try
  typing feel, reattach,
  copy mode, the clipboard and a real agent, with no SSH or setup.
- A copy made by the agent on the server (OSC 52) sets the laptop clipboard,
  even when the terminal blocks OSC 52.
- The cursor no longer jumps back while typing into Claude Code. Its
  renderer echoes a typed space by moving over the blank cell, so the space
  was never confirmed, expired after a moment and pulled the cursor back to
  it. Expiry, Backspace and redraws now leave the cursor where the next
  character goes.
- Pasting or dragging a local file into the terminal uploads it to the
  server's temporary directory and pastes the server path, so agents can open
  pasted screenshots, recordings and documents. macOS names that hold a
  narrow no-break space (screen recordings, before AM or PM) are read as
  one file, and a file that cannot be sent says why across the top of the
  screen. A file in a mounted folder pastes as its server path with no copy,
  system paths stay text, and Ctrl-C cancels an upload.
- Aft keeps what you read or edit (config, state, logs) in `~/.aft` and its
  plumbing (server, shims, launchers, sessions, sockets) in
  `~/.local/share/aft`, on macOS and Linux alike.
- One look across every command: colored marks (✓ ! ✗) that never rely on
  color alone, aligned tables, short `~` paths and relative times. Slow steps
  show a spinner on stderr, and `aft doctor` shows each check as it finishes
  above a progress bar. Grouped `aft help`; `aft list` works as `aft ls`;
  `aft attach` with several sessions offers a picker.
- Sessions are named with short animal words (`fox`, `owl`, `yak`), unique on
  their host, and `aft attach fox` reattaches to one. `aft ls` shows what each session
  is from the program and the window title it sets, so a Claude Code session
  reads as `Claude · Fix the login bug`.
- `aft ls` and `aft status` connect to each host themselves, all at once,
  instead of reporting hosts with no open connection as unreachable.
- Launchers run the real program inside an Aft session, so a session on a
  machine that is also its own host never nests.
- Listening ports on the server are mirrored to the local loopback while
  attached.
- `aft HOST`, `aft ls`, `aft attach`, `aft kill`, `aft status`,
  `aft doctor`, `aft host` (with `add`, `rm` and `use`), `aft agents add` and
  `aft tools add`.
- Sessions over UDP, as mosh does: SSH introduces the two ends, then the
  client connects with QUIC to a port in 60000-61000, pinning the session's
  certificate and presenting a one-time token. A lost packet no longer stalls
  the stream, and after a network change or sleep the client redials the
  session directly. Falls back to SSH when UDP is blocked; `udp = false` turns
  it off.
- Tools: `aft tools add git gh` makes those commands run once on the server,
  in the project's directory, wherever aft is on, with the terminal or pipes
  passed through and the exit code returned. Agents still run in sessions.
- `aft HOST` (`aft server`, `aft mini`) is a terminal on that host that
  survives disconnects, roams over UDP and predicts typing, and comes back to
  the shell already open in the same directory; `aft HOST CMD` runs one
  command there, as ssh does. A host not added yet is adopted the first time
  SSH can reach it. Host names cannot be commands, so they never conflict.
- Leaving says what happened, as ssh does: the program exited (with its code
  or signal), the session was detached, or you left while disconnected. A
  dropped connection shows a line across the top of the screen until it is
  back.
- A session that ends with no terminal attached keeps its last screen and how
  it ended; `aft ls` lists it as exited or lost, and `aft attach` shows it.
  Starting an agent in a project whose server folder is gone says so.
- `aft on` is exactly the folder you are in, with no search for a project
  root. A missing server folder is reported, not created, unless you name it
  with `--dir`.
- An unknown command, including a flag such as `--help`, names the command
  meant: commands are words, and flags, near spellings and retired names get
  a "did you mean aft help?" hint.
- A session detaches when its terminal goes away: a closed tab, another
  computer attaching, or a laptop that sleeps, which the client's heartbeat
  reveals within thirty seconds. `aft ls` shows which sessions are attached.
- `aft doctor` prints one line per check, and one per host when all is well
  there, and ends each problem with the exact command to fix it; `aft doctor --fix` applies the safe repairs (reinstall a
  stale or missing server, re-add the PATH line), `--json` prints findings for
  scripts, and `--host` limits the checks to one host.
- macOS and Linux hosts are supported; the server is embedded (gzipped) for both
  on amd64 and arm64.
- Multiple hosts are first class. Name each in `~/.aft/config.toml` under a
  singular `[host.<name>]` table, and manage them with `aft host` (list),
  `aft host add NAME DEST` (add and install) and `aft host rm`. The host used
  when `--host` is omitted is the top-level `use` key, set with `aft host use NAME`;
  with a single host it is implicit, so no `use` line is needed. Pick one per
  command with `--host`, or pin a project to a host for every machine by
  committing `host = "..."` in `aft.toml`.
- The config is flat and `snake_case`: top-level `env`, `ports` and `detach`
  replace the old `[env] pass`, `[ports] mirror` and `detach_key`, and
  `edit_dotfiles` (a boolean) replaces `path_integration`.
- Configure per host and per project, with project over host over global. A
  launch wrapper runs the agent inside any environment you name (a container, a
  nix or mise shell, a custom shell); per-host prediction, port range,
  passed env and SSH keepalive tuning are all overridable.
- Interactive setup: aft asks how to put its launchers on your PATH and, for
  people who manage their dotfiles, prints the line instead of editing. Prompts
  are driven directly, with no terminal probing, so they never disturb a
  session.
- Recognize a project on a network mount (SMB or NFS): `aft on` maps it to its
  real path on the server with no clone and no sync, auto-detecting the mapping
  from the mount and the server's share definition. Manage mappings with
  `aft mount`.
- `aft rename NAME NEW` gives a session a name of your own; a terminal attached
  to it is told, and follows.
- Launching an agent where one already runs in the same folder asks whether to
  attach to it or start a new one, instead of starting a second; `existing` in
  the config answers once for good. A terminal that loses a session to another
  machine is told which one took it.
- Aft never sets the terminal title: your tab says what the agent wants it to.
  Reconnecting, uploads and takeovers show as a line across the top of the
  screen, and only while they last.
- `aft HOST`, `aft attach` and `aft kill` connect the way ssh does: no spinner,
  and in two round trips instead of five. A session is looked up on the default
  host first. Prompts show their choices without a key legend.
- A client never replaces a newer server with its own older build; builds are
  ordered by commit time, and `aft doctor` says when the server is the newer
  one. The install check is one round trip.
- Backspace is predicted only over characters you typed: at an empty prompt
  it no longer erases the prompt before the application answers.
- `aft status` and `aft doctor` are flat: one line per fact, no labels or
  sections. A port record a killed client left behind no longer shows.
- Reattaching no longer leaves stray colored cells: the repaint erased a blank
  row in the color the row above had left in the pen, which showed as gray
  squares under Codex's input box when a terminal joined while it was still
  loading.
- The shadow terminal follows ncurses applications exactly: DEC line drawing
  (box characters through `smacs`), the repeat-character sequence that the
  xterm terminfos advertise, and cursor movement that treats a cursor outside
  the scroll region the way a VT does. A repaint restores the character sets
  too, so an application drawing a box keeps drawing lines after a reattach.
- After Enter, nothing you type is drawn until the application echoes
  again, so a password prompt never shows what you type. Keys typed during
  an outage are delivered after the repaint, not drawn onto a blank screen.
- Copy mode on a long session no longer freezes the client: a snapshot is
  trimmed to what one frame can carry, a frame that cannot be decoded ends the
  link instead of waiting on it forever, and a snapshot that never arrives
  stops blocking copy mode after a few seconds.
- Leaving a session closes the link at once, so the session sees the detach
  rather than waiting out its idle timeout; closing the window releases the
  port forwards it held.
- Port mirroring applies updates in order, off the input loop; a notice never
  draws over copy mode, and a flash takes down only itself.
- The session server no longer deadlocks when an attached terminal stops
  reading: a failed write releases everyone waiting to send to it. A renamed
  session no longer removes another session's socket when it ends, starting a
  session under a name in use leaves the running one untouched, a terminal
  that joins is told the ports already listening, a replaced terminal's keys
  no longer reach the application, and the last output of an application
  that exits unseen is kept.
- A reattach to a session with more history than one frame can carry
  repaints the screen rather than nothing. fish works as the login shell on a
  host. A newer server missing its shims is set up, never replaced. An
  upload cut short by Ctrl-C leaves no truncated file on the server. A TIFF
  on the macOS pasteboard pastes as an image.
- Setup and `aft doctor --fix` put the PATH line back when a restored shell
  config has lost it, instead of trusting their notes. A host that cannot be
  reached is never written to the config; `aft setup` on a host you already
  have keeps its settings; `aft host rm` turns off the projects that ran on
  it. Escape or Ctrl-C in a prompt backs out instead of taking the default.
  A launcher name that aft could not tell from its own commands is refused.
  A broken `aft.toml` is reported, not ignored. A host table without `ssh` is
  an error rather than this machine.
- Hyperlinks (OSC 8) are kept per cell and restored by a repaint, so a link
  on screen stays clickable after a reattach.
- Copy mode's status bar names no keys; the guide lists them.
- `aft completion` completes hosts and session names, not just commands.
  `aft ls` with nothing running says how to start one, `aft` alone on a fresh
  install says to run `aft setup`, and `aft doctor` sees launchers put on PATH
  by the shell config even when run from a script.

### Removed

- `aft detach` (detaching follows the terminal), `aft clone` (`aft on` creates
  the directory; clone with git as a tool), `aft exec` (tools), `aft shell`
  (now `aft HOST`), and `aft ports` and `aft use`, now part of `aft status` and
  `aft host`. The server binary is `aft-server`, as mosh's is `mosh-server`, at
  `~/.local/share/aft/aft-server` on each host and in process lists; the aft
  binary is the server when started under that name, so no hidden command
  can clash with a host called `server`.

- Workspaces, project records, `connect`, `open`, `run`, port watch, systemd and
  launchd services, migration, the picker and the dry-run uninstall.
- The dependency on tmux and on `ss` on the server.
- The rule against third-party Go dependencies.
