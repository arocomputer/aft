# Changelog

User-visible changes are recorded here. No public release has been published yet.

## Unreleased

Aft was rebuilt around one idea: the agent runs on the server, the terminal
stays on your machine, and typing feels local.

### New features

- Sessions hold an application in a pseudo-terminal on the server, survive a
  lost connection, repaint on reattach, and answer terminal queries while
  detached. No tmux is needed or used.
- The client predicts typed characters and Backspace locally and reconciles them
  with the server's echo, so typing has no network latency. Everything else is
  passed through byte for byte.
- Launchers: `aft on` in a project makes `claude`, `codex`, `opencode`, `pi` and
  any other configured program run on the server; `aft off` makes them local
  again. Agents are not configured or wrapped.
- `aft setup` installs Aft's helper on a server over SSH, creates launchers, and
  adds a marked PATH block to the shell config. `aft uninstall` removes all of
  it, on every machine, and leaves edited blocks alone.
- Bridges: URLs opened on the server open in the local browser; the local
  clipboard, including images, is readable from the server through shims.
- Listening ports on the server are mirrored to the local loopback while
  attached.
- `aft shell`, `aft ls`, `aft attach`, `aft detach`, `aft end`, `aft clone`,
  `aft exec`, `aft status`, `aft doctor`, `aft ports` and `aft agents add`.
- macOS and Linux servers are supported; the helper is embedded for both on
  amd64 and arm64.

### Removed

- Workspaces, project records, `connect`, `open`, `run`, port watch, systemd and
  launchd services, migration, the picker and the dry-run uninstall.
- The dependency on tmux and on `ss` on the server.
- The rule against third-party Go dependencies.
