# Contributing

Keep changes focused and solve the problem with the least machinery needed.
Read [AGENTS.md](AGENTS.md) for the code map and contracts, and
[the design](docs/contributing/plan.md) for why things are built the way they are.

## Setup and verification

Use Go 1.26 or newer, Python 3.9 or newer (standard library only), and a POSIX
shell on Linux or macOS.

```sh
./x check
./x release-check
```

| Command | Purpose |
| --- | --- |
| `./x fmt` | Format all Go source |
| `./x fmt --check` | Check formatting without editing |
| `./x test` | Run Go tests with the race detector and the Python script tests |
| `./x try` | Open the interactive test command center (builds aft, runs the guided self-test) |
| `./x bench` | Run the benchmarks (microbenchmarks and the end-to-end typing-latency benchmark) |
| `./x docs` | Check guide metadata and local Markdown links |
| `./x helpers` | Cross-compile the server helper for every platform into the embed directory |
| `./x build` | Build helpers, then `bin/aft` and `bin/aft-helper` |
| `./x check` | Formatting, vet, tests, docs and build |
| `./x release-check [vX.Y.Z]` | Build and inspect all archives in a temporary directory |
| `./x dist vX.Y.Z` | Write archives, checksums and notes to `dist/vX.Y.Z`; never publish |

A plain `go build ./cmd/aft` works for development but embeds no helpers, so it
can only use the host `local`. Run `./x build` for a binary that can install
itself on servers.

Third-party dependencies are allowed when they earn their place; each new one
is justified in review.

## Contributions and review

Bug reports should include `aft version`, both operating systems, the terminal
application, reproducible steps, and expected and actual behavior. Sanitize
hostnames, paths and logs before sharing. Report vulnerabilities through
[the security policy](SECURITY.md), not public issues.

Explain the problem and resulting behavior in your PR, record check results,
add focused regression tests when behavior changes, update affected guides, and
summarize user-visible changes in [Unreleased](CHANGELOG.md). Use a clear title
such as `fix(client): repaint stale predictions after resize`.

Contributions are licensed under the [MIT license](LICENSE).
