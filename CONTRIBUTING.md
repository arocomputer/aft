# Contributing

Keep changes focused and solve the problem with the least machinery needed.
Read [AGENTS.md](AGENTS.md) for the code map and working conventions.

## Setup and verification

Use Go 1.24 or newer, Python 3.9 or newer (standard library only), and a POSIX
shell on Linux or macOS. `./x build` alone needs only Go and a shell.

```sh
./x check
./x release-check
```

`./x` is the shared entry point for local work and CI:

| Command | Purpose |
| --- | --- |
| `./x fmt` | Format all Go source |
| `./x fmt --check` | Check formatting without editing |
| `./x test` | Run Go tests with the race detector and infrastructure tests |
| `./x docs` | Check guide metadata, groups and local Markdown links |
| `./x build` | Build `bin/aft` for the current platform |
| `./x check` | Check formatting, vet, tests, docs and native build |
| `./x release-check [vX.Y.Z]` | Build and inspect all four archives in a temporary directory; optional tag also checks changelog notes |
| `./x dist vX.Y.Z` | Write archives, checksums and release notes to a new `dist/vX.Y.Z` directory; never publish |

CI runs `./x check` on Linux and macOS and `./x release-check` on Linux.
See [testing](docs/contributing/testing.md) for coverage limits and manual checks,
and [releases](docs/contributing/releases.md) for the release process.

## Contributions and review

Bug reports should include the source commit, OS/architecture, reproducible steps,
and expected/actual behavior. Aft has no version flag yet. Sanitize hostnames,
paths, logs and configuration before sharing. Report vulnerabilities through
[the security policy](SECURITY.md), not public issues.

Explain the problem and resulting behavior in your PR. Record actual check
results and skipped checks. Add focused regression tests when behavior changes,
update affected guides, and summarize user-visible changes in
[Unreleased](CHANGELOG.md). Describe compatibility and migration implications.
Use a clear title such as `fix(ports): reap tunnels on termination`.

Maintainer review is required by project policy; CODEOWNERS routes requests but
GitHub settings must enforce approval. AI-assisted contributions are welcome when
the contributor reviews and owns the result, verification and follow-up. Preserve
accurate authorship and attribution.

Contributions are licensed under the [MIT license](LICENSE).
