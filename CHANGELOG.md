# Changelog

User-visible changes are recorded here. Publication status and downloads are on
[GitHub Releases](https://github.com/arocomputer/aft/releases).

## Unreleased

- Simplify command output: plain text, compact listings without table headers,
  shorter help without branding or footers, and `aft help COMMAND` for details.
  Doctor reports problems and repairs, or `ok` when healthy; JSON retains all
  findings. Remove the color renderer and unused styling dependencies.

- Interactive setup lets you choose agent and tool launchers. New setups
  preselect programs found on the execution machine, show local/server discovery
  hints, and treat configuration directories as hints rather than installations.
  Reruns keep saved choices and remove deselected, unchanged Aft launchers.
  Setup no longer enables the entire known-agent catalog.

## 0.1.0

Aft's first release runs your terminal agents on your server while typing stays
responsive on your own machine. This is an early 0.x release; commands and
configuration may change as we learn from use.

### Included

- Run Claude, Codex, opencode, pi or another terminal program on your server
  through the same command you already use. `aft on` enables a project;
  `aft off` returns its launchers to local execution. Aft does not sync code or
  install agents.
- Sessions survive a lost client and restore the screen on reattach. Input is
  queued asynchronously; local prediction draws printable characters and
  Backspace before the server's echo arrives.
- `aft HOST` opens an independent host shell with prediction and reconnect.
  Shells become listed and attachable only while their current directory is
  inside an enabled project. Existing agent sessions retain their visibility.
- Use QUIC over UDP when reachable, with an SSH fallback. Authentication and
  host trust stay with your OpenSSH configuration.
- Bridge browser URLs, copy text to the local clipboard, read the clipboard
  just after a paste, and upload pasted local files. Mirror remote development
  ports onto local loopback.
- Open scrollback copy mode with Ctrl-] then `[`. Ctrl-] then `d` detaches.
  Prefix keys are configurable and can be disabled.
- Manage multiple hosts, map network-mounted projects, rename sessions and
  choose whether to attach to an already running agent or start another.
- `aft` shows a compact overview; `aft help` and `aft --help` show the full
  reference. `aft doctor` diagnoses setup and can apply safe repairs.
- Setup asks before editing shell configuration and supports managed dotfiles.
  Uninstall removes Aft's files and unchanged shell blocks while preserving
  your code, SSH configuration and agent data.
- Release archives include build provenance. Publishing a release verifies native
  downloads on Linux and macOS before a protected job updates the Homebrew
  formula in Aft's own repository using the shared organization release token.
- Portable binaries for Linux and macOS on amd64 and arm64. Each client embeds
  the servers for all four targets and installs the right one over SSH.

### Build security

- Require Go 1.26.10 or newer so release binaries include standard-library
  security fixes. Source CI and the release workflow audit the code with the
  actual compiler used for their builds.

### Limits

- macOS downloads are not notarized. Archives are unsigned; SHA-256 checksums
  detect corrupted downloads and must come from the same trusted release.
- Sessions survive client disconnects, but do not survive a server reboot.
- Prediction hides typing latency; application responses still depend on the
  network. Windows requires WSL.
- Applications run with your account's permissions on the server. Aft is not
  an application sandbox. Clipboard, image and terminal behavior depend on
  the agent, terminal and operating system; see the guides and release notes.
