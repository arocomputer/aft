# Releases and repository setup

The first release candidate is **v0.1.0**. It is not published by preparing this
branch, building local archives or merging a PR. Publication status and
available downloads are on [GitHub Releases](https://github.com/arocomputer/aft/releases).

## v0.1.0 qualification

This is an initial 0.x release, with no promise of a frozen CLI or configuration
format. The release notes are the exact `## 0.1.0` section of
[CHANGELOG.md](../../CHANGELOG.md).

Evidence gathered on 2026-10-25:

| Check | Evidence or remaining work |
| --- | --- |
| Linux and macOS source CI | Passed on merged PR #16; [main CI run](https://github.com/arocomputer/aft/actions/runs/40148257486) |
| Archive versions, embedded servers, platform metadata and checksums | Verified by `./x release-check v0.1.0`; rerun on the final release commit |
| Native artifact sessions and isolated uninstall | Verified locally on Linux; macOS artifact execution must pass the preparation PR's CI |
| Typing on a real host | Maintainer reports checked; OS, agent and terminal versions were not recorded |
| Sleep and reconnect | Maintainer reports checked |
| Clipboard and image paste | Maintainer reports checked |
| Browser login | Still needs a real-host check |
| Uninstall on a real remote host | Still needs a check; isolated local uninstall is covered automatically |
| Private vulnerability reports | Enabled in repository settings |
| Dependency integrity and vulnerability scan | `go mod verify` passed; `govulncheck v1.9.0` found no vulnerable imported packages or reachable symbols. `x/net` and `x/crypto` updated to patched versions; the unused, unmaintained `x/crypto/openpgp` advisory has no fixed version |
| Release publication | Not authorized by preparation; `RELEASE_ENABLED` is unset and no `release` environment is configured |

The maintainer's reports are hands-on evidence, not an independently reproduced
OS/agent matrix. Before tagging, record the OS, agent and terminal versions and
outcomes for browser login and uninstall in the release review. Use a disposable
account or machine for uninstall; keep credentials, hostnames and private paths
out of committed evidence. See the [manual checks](testing.md#manual-checks-before-a-release).

## Final release steps

1. Merge the preparation PR after `check (ubuntu-latest)`,
   `check (macos-latest)` and `release-check` pass. CI runs the versioned archive
   check natively on Linux and macOS.
2. Complete the outstanding real-host checks above. Confirm the final source
   commit and `v0.1.0` notes. Run `./x check` and
   `./x release-check v0.1.0` from that commit.
3. With explicit maintainer approval, configure a `release` environment with a
   required reviewer and set `RELEASE_ENABLED=true`. Keep `main` as the default
   branch and pin Actions to full commit SHAs.
4. With explicit approval to tag, create an annotated `v0.1.0` tag on the
   qualified commit reachable from `main`, then push that tag. Alternatively,
   dispatch the release workflow from `main` with the existing tag.
5. The workflow reruns source and native artifact checks on Linux and macOS,
   builds the archives and waits for the `release` environment approval. It
   verifies the selected tag, source commit and hashes, then creates a **draft**.
6. Review the draft's version, source commit, notes, four archives and
   `checksums.txt`. Download and verify a native archive. Publishing the draft
   is a separate, explicit maintainer action.

No command in `./x` publishes, creates tags or changes repository settings.

## What users receive

Archives are provided for Linux and macOS on amd64 and arm64. Each contains
`aft`, all four embedded servers, the license, README, brand assets and guides.
Every client and server reports the selected release version and the source
commit time used for server upgrade ordering. `release-notes.md` records the
source commit. SHA-256 checksums cover the four archives.

Archives are unsigned, and macOS binaries are not notarized. Explain this in
the [installation guide](../guides/start/install.md); never tell users to disable
system-wide protections. Cross-build success verifies packaging, not runtime
behavior on every CPU/OS combination.

## Local packaging

`./x release-check [vX.Y.Z]` uses a temporary directory and removes its archives.
Without a version it qualifies a development build; a version selects the exact
changelog section and verifies that version in the native binaries.

`./x dist v0.1.0` requires a clean committed checkout and writes archives,
`checksums.txt` and release notes to a new `dist/v0.1.0` directory. It refuses to
overwrite one. These local archives are for qualification; the release workflow
rebuilds the published candidate from the selected tag.

Download checksums from the trusted release and verify with
`sha256sum --check checksums.txt` or `shasum -a 256 --check checksums.txt` when
all four archives are present. For a single archive, use the filename-filtered
commands in the installation guide.
