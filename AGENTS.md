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
  invoked under an agent's name. `cmd/aft-helper` is the server binary.
- `internal/vt` is the terminal emulator both sides use: the client's shadow
  screen and the holder's detached screen. It renders a screen back to escape
  sequences and answers terminal queries.
- `internal/holder` runs one application in a pseudo-terminal on the server,
  attaches one client at a time, repaints on reattach, and relays bridge
  requests. `internal/helper` is the server CLI around it plus the bridge shims.
- `internal/client` is the terminal client: input tokenizer, prediction,
  passthrough, reconnect, bridges and port mirroring.
- `internal/transport` reaches a host through the user's OpenSSH with a shared
  master connection, or runs the helper locally for the reserved host `local`.
- `internal/remote` embeds and installs the helper. `internal/launcher` owns
  shims, the on/off decision and the shell PATH block. `internal/config` reads
  settings and state. `internal/bridge` and `internal/ports` are the local
  sides of URL opening, clipboard and port mirroring.
- `internal/proto` is the framed protocol between client and holder.

Tests live beside the code. The helper and end-to-end tests spawn the test
binary itself as the helper and drive the client under a real pseudo-terminal.

## Contracts

- Server output passes through to the terminal byte for byte. Predictions are
  drawn only between escape sequences and are reconciled or repainted.
- Enter, control keys, pastes and escape sequences are never predicted.
- Sessions survive a lost client. Aft never stops a session on its own.
- Only allowlisted environment variables cross to the server.
- Everything Aft writes lives under `~/.aft` on each machine plus the recorded
  shell block, and `aft uninstall` removes exactly that.
- Aft installs nothing but its own helper, edits nothing but the shell block,
  and never runs sudo.

Run `./x check` for changes and `./x release-check` for distribution work.
Cross-build success is not runtime verification; report limits honestly.
