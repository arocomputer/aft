# Contributing

Keep changes focused and solve the problem with the least machinery needed.
Read [AGENTS.md](AGENTS.md) for the code map and contracts, and
[the design](docs/contributing/plan.md) for why things are built the way they are.

## Setup and verification

Use Go 1.26.10 or newer, Python 3.9 or newer (standard library only), and a POSIX
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
| `./x setup [FLAGS]` | Build and preview setup in a temporary home, with SSH disabled |
| `./x try [TEST]` | Build and open the test menu, or run one named check |
| `./x bench` | Run the benchmarks (microbenchmarks and the end-to-end typing-latency benchmark) |
| `./x docs` | Check guide metadata and local Markdown links |
| `./x servers [VERSION]` | Cross-compile and gzip the servers; packaging passes its exact release version |
| `./x build` | Cross-compile the embedded servers, then build `bin/aft` |
| `./x check` | Formatting, vet, tests, docs and build |
| `./x audit` | Check reachable vulnerabilities with the active Go toolchain and dependency graph |
| `./x release-check [vX.Y.Z]` | Build all archives, inspect metadata and exercise the native artifact in a temporary directory |
| `./x dist vX.Y.Z` | Write archives, checksums and notes to `dist/vX.Y.Z`; never publish |

A plain `go build ./cmd/aft` works for development but embeds no servers, so it
can only use the host `local`. Run `./x build` for a binary that can install
itself on remote hosts.

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

## Dependency updates

GitHub Actions checks for Go patch updates every Monday at 09:00 UTC. The
[dependency workflow](.github/workflows/deps.yml) can also be dispatched from
`main`. It uses `go get -u=patch ./...` and `go mod tidy`, then runs `./x check`
and `./x audit` in a read-only job. It requests patch updates; Go may raise other module versions to satisfy
dependency constraints, so the PR lists the actual version changes. The Go
compiler, major module migrations and pinned Actions are reviewed separately.

A separate job publishes only `go.mod` and `go.sum` to one bot-owned PR on
`chore/dependency-updates`. It refuses to overwrite an unrelated branch or
human commits. The PR stays a draft until the full Linux/macOS CI and native
release checks pass on the exact update commit. Failed runs leave the draft
available for inspection and the next update. Review and merge the PR normally;
updates are never merged automatically.

The publisher uses `GITHUB_TOKEN` with job-specific Contents, Pull requests and
Actions write permissions. Repository settings must allow GitHub Actions to
create PRs. CI is dispatched explicitly, and the PR links to the successful
validation run. GitHub separately requires maintainer approval for the
PR-triggered workflow created by `GITHUB_TOKEN`; approve that run when reviewing
the PR so its required checks can run on the merge commit. This is distinct from
the updater's automatic validation of the dependency commit. See
[GitHub's bot-PR policy](https://github.blog/changelog/2026-06-11-bot-created-pull-requests-can-run-workflows-if-approved/).
No release token or Dependabot setup is needed, and the default workflow
permission stays read-only.
