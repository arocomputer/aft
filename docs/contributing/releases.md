# Releases and repository setup

The first official release is **v0.1.0**. Preparing a branch, building local
archives or merging a PR does not publish a release. Publication status and
available downloads are on [GitHub Releases](https://github.com/fschrhunt/aft/releases).

## v0.1.0 qualification

This is an initial 0.x release, with no promise of a frozen CLI or configuration
format. The release notes are the exact `## 0.1.0` section of
[CHANGELOG.md](../../CHANGELOG.md).

Evidence gathered on 2026-10-25:

| Check | Evidence or remaining work |
| --- | --- |
| Linux and macOS source CI | Passed on merged PR #16; [main CI run](https://github.com/fschrhunt/aft/actions/runs/40148257486) |
| Archive versions, embedded servers, platform metadata and checksums | Verified by `./x release-check v0.1.0`; rerun on the final release commit |
| Native artifact sessions and isolated uninstall | Passed on Linux and macOS in preparation PR #17 and infrastructure PR #18 CI |
| Typing on a real host | Maintainer reports checked; OS, agent and terminal versions were not recorded |
| Sleep and reconnect | Maintainer reports checked |
| Clipboard and image paste | Maintainer reports checked |
| Browser login | Still needs a real-host check |
| Uninstall on a real remote host | Still needs a check; isolated local uninstall is covered automatically |
| Private vulnerability reports | Enabled in repository settings |
| Dependency integrity and vulnerability scan | `go mod verify` passed; `govulncheck v1.9.0` found no vulnerable imported packages or reachable symbols. `x/net` and `x/crypto` updated to patched versions; the unused, unmaintained `x/crypto/openpgp` advisory has no fixed version |
| Release publication | Maintainer authorized v0.1.0 after verifying the organization-secret grant and release checks; browser login and real-host uninstall remain unverified |

The maintainer's reports are hands-on evidence, not an independently reproduced
OS/agent matrix. The maintainer authorized v0.1.0 with browser login and real-host
uninstall still unverified; those checks must not be represented as completed.
Record the OS, agent and terminal versions when those checks are performed. Use a disposable
account or machine for uninstall; keep credentials, hostnames and private paths
out of committed evidence. See the [manual checks](testing.md#manual-checks-before-a-release).

The initial candidate was stopped before publication because the pinned Go
1.26.0 compiler introduced reachable standard-library vulnerabilities. Builds
now require Go 1.26.10 or newer, and both CI and release qualification run
`./x audit` under their actual compiler. A scan with a different, newer local
compiler does not qualify binaries built by CI.

## Final release steps

1. Merge the preparation PR after `check (ubuntu-latest)`,
   `check (macos-latest)` and `release-check` pass. CI runs the versioned archive
   check natively on Linux and macOS.
2. Review the outstanding real-host checks above and the maintainer's explicit
   release decision. Confirm the final source commit and `v0.1.0` notes. Run `./x check` and
   `./x audit` and `./x release-check v0.1.0` from that commit.
3. With explicit maintainer approval, configure a `release` environment with a
   required reviewer and set `RELEASE_ENABLED=true`. Keep `main` as the default
   branch and pin Actions to full commit SHAs.
4. With explicit approval to tag, create an annotated `v0.1.0` tag on the
   qualified commit reachable from `main`, then push that tag. Alternatively,
   dispatch the release workflow using the same tag as its workflow ref:
   `gh workflow run release.yml --ref v0.1.0 -f tag=v0.1.0`. A manual run from
   `main` is not allowed: the run's source identity must match the built tag
   for its provenance to be accurate.
5. The workflow reruns source and native artifact checks on Linux and macOS,
   builds and attests the archives and waits for the `release` environment approval. It
   verifies the selected tag, source commit and hashes, then creates a **draft**.
6. Review the draft's version, source commit, notes, four archives and
   `checksums.txt` and provenance. Download and verify a native archive. Publishing
   the draft is a separate, explicit maintainer action; it starts the install
   checks and protected Homebrew formula update described below.

No command in `./x` publishes, creates tags or changes repository settings.

## Repository release infrastructure

Aft uses `fschrhunt/aft` as its own Homebrew tap. This pattern was also used by
[ask](https://github.com/arocomputer/ask/blob/main/.github/workflows/release.yml)
and [search](https://github.com/arocomputer/search/blob/main/.github/workflows/release.yml);
those projects are separate integrations, not release-token probe targets.

- Archive provenance is attested in a separate job before a draft is created.
  That job only handles built artifacts; repository code does not execute with
  its OIDC/attestation credentials. Verify a downloaded archive with
  `gh attestation verify ARCHIVE --repo fschrhunt/aft`.
- Publishing the draft triggers checksum-verified native installation checks on
  Linux and macOS. After those pass, the protected `formula` job generates
  `aft.rb` from the published checksums and opens a bot-owned PR against `main`.
  Review and merge it after required checks pass; automation never merges it.
  The job refuses tags outside `main` and does not replace the latest formula
  with an older release. `RELEASE_ENABLED` gates these post-publication jobs.
- The formula job uses `secrets.RELEASE_TOKEN`; the normal `github.token`
  creates the draft release and reads its assets. Token availability and the
  account's repository role are checked before creating the draft, but a role
  check alone does not prove token write authorization.

### Transfer audit (2026-10-29)

Read-only inspection after the transfer found:

- `main` remains the default branch, all three workflows are active, and
  default workflow permissions remain read-only. Actions may create PRs.
- The active main ruleset requires checked PRs and has **no bypass actors**.
  The existing direct formula push therefore cannot satisfy that ruleset.
  Formula publication now uses a checked PR without a ruleset bypass.
- `RELEASE_ENABLED` is still `true`. No repository or `release` environment
  secrets were listed, so `RELEASE_TOKEN` is unavailable. The old organization
  secret grant does not follow a repository into a personal account; the
  organization-secret endpoint is not applicable here. The maintainer must
  supply an authorized repository or environment secret with access to
  `fschrhunt/aft` and Contents write permission before release automation can
  work. Do not configure or copy credentials as part of this audit.
- The `release` environment retains its required reviewer and allows `main`
  branches and `v*` tags. Private vulnerability reporting remains enabled.
- The repository homepage still points to `aro.computer/aft`; the maintainer
  should update it to the intended current project URL. No webhooks were listed.
  Installed-app inspection was unavailable; review app installations and any
  external integrations separately for access to `fschrhunt/aft`.

The maintainer should set `RELEASE_ENABLED=false` until missing credentials
and formula publication authorization are resolved. Settings are not changed
by this PR.
Never print or copy secret values into source, logs, issues or PRs. To inspect
only secret names without retrieving values:

```sh
gh api repos/fschrhunt/aft/actions/secrets --jq '.secrets[].name'
gh api repos/fschrhunt/aft/environments/release/secrets --jq '.secrets[].name'
```

To retry the formula for an already published release without rebuilding,
retagging or publishing anything, dispatch from `main`:

```sh
gh workflow run release.yml --ref main -f tag=v0.1.0 -F formula_only=true
```

This path verifies the native published downloads on Linux and macOS before
the protected formula job. The `release` environment must permit `main` branch
deployments as well as `v*` tags. An unchanged formula creates no branch or PR.
Each release uses its own branch; retries reuse only an open bot-owned PR with
one unchanged automation commit that modifies only `aft.rb` and matches the
generated formula. Human edits and closed PRs require manual review and are
never overwritten.
If branch creation succeeds but PR creation fails, retrying opens the PR only
when that branch passes the same content and automation ownership checks.
GitHub may require maintainer approval of bot PR workflows before checks run.

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
