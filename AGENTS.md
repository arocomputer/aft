# Working on Aft

Aft is a small Go CLI around system OpenSSH and remote tmux. Keep it small.

- Preserve unrelated changes. Never revert or reformat code outside the task.
- Never expose secrets or add real credentials, host inventories or machine paths.
- Prefer simple, direct implementations. Add abstractions only when needed.
- Comment functions, classes and modules with a concise purpose and contract.
  Update affected comments and docs with each behavior change.
- Add focused tests for meaningful behavior; avoid redundant smoke tests.
- Do not publish, push, tag, run releases or change user configuration without
  explicit authorization. Local checks and builds do not publish.

## Code map

`cmd/aft/main.go` and `cli.go` dispatch commands and construct SSH invocations.
`workspace.go` and `projects.go` manage local metadata and remote containment.
`ports.go` owns forwarding processes; `service.go` integrates user service managers.
`doctor.go` and `diagnostics.go` inspect connectivity and listeners.
Tests live beside the code and use temporary homes and fake executables.

`docs/guides/` is canonical user documentation. `docs/contributing/` holds testing,
roadmap and release practices. No website or embedded docs are implemented.
`x` is the developer entry point; `scripts/` contains standard-library Python
checks and archive packaging. `.github/workflows/` calls the same commands.

## Contracts and checks

Preserve literal remote arguments, private atomic metadata, no-overwrite behavior,
workspace path validation, loopback-only tunnels and owned child cleanup. Remote
files are never removed as rollback. Uninstall is dry-run only. Treat service
manager uncertainty as an error, not proof that a service has stopped.

Run `./x check` for changes and `./x release-check` for distribution work.
Cross-build success is not runtime verification; report limits honestly.
See [CONTRIBUTING.md](CONTRIBUTING.md) for review and task commands.
