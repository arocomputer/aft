# Working on Aft

Aft runs agents on a server while the terminal stays on the user's machine.
Read [the design](docs/contributing/plan.md) before changing architecture.

- Preserve unrelated changes. Never revert or reformat code outside the task.
- Never add real credentials, host names or machine paths.
- Prefer simple, direct implementations. Add abstractions only when needed.
- Comment functions, types and packages with a concise purpose and contract.
  Update affected comments and docs with each behavior change.
- Add focused tests for meaningful behavior; avoid redundant smoke tests.
- Do not publish, push, tag, run releases or change the user's machine without
  explicit authorization. Local checks and builds do not publish.

## Code map

- `cmd/aft` is the user binary: dispatch, commands, and the launcher entry when
  invoked under an agent's name. `cmd/server` is the server binary, cross-compiled per platform into `internal/remote/embed` (gzipped) and carried inside aft for install (there is no standalone server binary).
- `internal/vt` is the terminal emulator both sides use: the client's shadow
  screen and the session's detached screen. It renders a screen back to escape
  sequences and answers terminal queries.
- `internal/session` runs one application in a pseudo-terminal on the host,
  attaches one client at a time, repaints on reattach, and relays bridge
  requests. `internal/server` is the server CLI around it plus the bridge shims.
- `internal/client` is the terminal client: input tokenizer, prediction,
  passthrough, reconnect, bridges, port mirroring, and copy mode (the
  scrollback pager, in copymode.go). `cmd/aft/selftest.go` is the guided
  hands-on harness, isolated in a throwaway home.
- `internal/transport` reaches a host through the user's OpenSSH with a shared
  master connection, or runs the server locally for the reserved host `local`.
- `internal/remote` embeds and installs the server. `internal/launcher` owns
  launchers, the on/off decision and the shell PATH block. `internal/config` reads
  settings and state. `internal/bridge` and `internal/ports` are the local
  sides of URL opening, clipboard and port mirroring.
- `internal/ui` holds raw-mode interactive prompts for setup and config. It
  never uses a TUI framework, because Bubble Tea probes the terminal on import,
  which would corrupt the session client that shares it.
- `internal/config` resolves settings as project over host over global, and
  carries the launch wrapper that lets a user run the agent inside any
  environment (a container, a nix or mise shell).
- `internal/proto` is the framed protocol between client and session.

Tests live beside the code. The server and end-to-end tests spawn the test
binary itself as the server and drive the client under a real pseudo-terminal.

## Contracts

- Server output passes through to the terminal byte for byte. Predictions are
  drawn only between escape sequences and are reconciled or repainted.
- Enter, control keys, pastes and escape sequences are never predicted.
- The input loop never writes to the network; sends are queued to a writer
  goroutine, so prediction and typing are never gated by a slow link.
- The prefix key (default Ctrl-], configurable, disableable) is the only key
  aft takes from the application, and only while enabled; prefix then [ opens
  copy mode, prefix then d detaches. Copy mode works on a snapshot and never
  captures the mouse, so native selection still works.
- Sessions survive a lost client. Aft never stops a session on its own.
- Only allowlisted environment variables cross to the server.
- Everything Aft writes lives under `~/.aft` on each machine plus the recorded
  shell block, and `aft uninstall` removes exactly that.
- Aft installs nothing but its own server, edits nothing but the shell block,
  and never runs sudo. Setup asks before editing dotfiles and can print the
  line instead.
- Nothing on the client path may write to the terminal at startup. Interactive
  prompts live in internal/ui and run only when called; no imported package may
  probe the terminal, or it corrupts the passthrough client.

Run `./x check` for changes and `./x release-check` for distribution work.
Cross-build success is not runtime verification; report limits honestly.
