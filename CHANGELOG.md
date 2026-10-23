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
  clipboard, including images, is readable from the server through shims.
- Copy mode: the prefix (Ctrl-] by default) then `[` opens a scrollback pager in
  the client over a snapshot from the server, with keyboard scroll, search and
  selection that copies to the local clipboard, and native drag-select.
- Predictions emit the fewest bytes: a bare character in steady typing, exactly
  like native echo, with no cursor-move or style reset, so there is no flicker.
  The prediction itself costs about 12 microseconds; felt latency is the
  terminal and display, which aft draws within.
- The client draws each predicted keystroke before sending it to the network,
  and gives keystrokes priority over output, so neither a congested link nor a
  burst of agent output can stall your typing.
- `aft selftest`, opened with `./x try`, is a guided, isolated harness to try
  typing feel, reattach,
  copy mode, the clipboard and a real agent, with no SSH or setup.
- - A copy made by the agent on the server (OSC 52) sets the laptop clipboard,
  even when the terminal blocks OSC 52. Clipboard reads from the server stay
  off by default.
- Listening ports on the server are mirrored to the local loopback while
  attached.
- `aft shell`, `aft ls`, `aft attach`, `aft detach`, `aft end`, `aft clone`,
  `aft exec`, `aft status`, `aft doctor`, `aft ports` and `aft agents add`.
- `aft doctor` groups its checks by machine and ends each problem with the exact
  command to fix it; `aft doctor --fix` applies the safe repairs (reinstall a
  stale or missing server, re-add the PATH line), `--json` prints findings for
  scripts, and `--host` limits the checks to one host.
- macOS and Linux hosts are supported; the server is embedded (gzipped) for both
  on amd64 and arm64.
- Configure per host and per project, with project over host over global. A
  launch wrapper runs the agent inside any environment you name (a container, a
  nix or mise shell, a custom shell); per-host home, prediction, port range,
  passed env and SSH keepalive tuning are all overridable.
- Interactive setup: aft asks how to put its launchers on your PATH and, for
  people who manage their dotfiles, prints the line instead of editing. Prompts
  are driven directly, with no terminal probing, so they never disturb a
  session.
- Recognize a project on a network mount (SMB or NFS): `aft on` maps it to its
  real path on the server with no clone and no sync, auto-detecting the mapping
  from the mount and the server's share definition. Manage mappings with
  `aft mount`.

### Removed

- Workspaces, project records, `connect`, `open`, `run`, port watch, systemd and
  launchd services, migration, the picker and the dry-run uninstall.
- The dependency on tmux and on `ss` on the server.
- The rule against third-party Go dependencies.
