# Releases and repository setup

The first official release is **v0.1.0**. Preparing a branch, building local
archives or merging a PR does not publish a release. Publication status and
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

## Shared arocomputer release infrastructure

Like [ask](https://github.com/arocomputer/ask/blob/main/.github/workflows/release.yml)
and [search](https://github.com/arocomputer/search/blob/main/.github/workflows/release.yml),
Aft uses its own repository as its Homebrew tap. The `tap` repository is a
separate application, not the destination for these formulas.

- Archive provenance is attested in a separate job before a draft is created.
  That job only handles built artifacts; repository code does not execute with
  its OIDC/attestation credentials. Verify a downloaded archive with
  `gh attestation verify ARCHIVE --repo arocomputer/aft`.
- Publishing the draft triggers checksum-verified native installation checks on
  Linux and macOS. After those pass, a protected `formula` job generates
  `aft.rb` from the published checksums and opens a PR containing only that file.
  Merge the formula PR after required CI passes; protected `main` rejects direct
  pushes, including release automation.
  The job refuses tags outside `main` and does not replace the latest formula
  with an older release. `RELEASE_ENABLED` also gates these post-publication jobs.
- The formula job uses `secrets.RELEASE_TOKEN`, matching ask and search. The
  normal `github.token` creates the release and reads its assets; the shared
  token is needed to push the formula branch and open its PR so CI runs.

The existing `RELEASE_TOKEN` organization secret is now available to
`arocomputer/aft`, as verified through the inherited-secret endpoint. A protected
preflight checks token availability and the account's repository role before
creating the draft. That role does not prove the token itself has write scopes:
the first publication's formula push returned HTTP 403 despite the role check.
The formula was instead submitted through a checked PR. The workflow now opens
a formula PR because the repository rules require PRs and status checks.
For future repositories, an organization administrator must grant access to the
existing secret instead of creating another credential.
The underlying credential must also include Aft in its repository selection,
with **Contents: read and write** and **Pull requests: read and write**. Granting
the organization secret to a repository does not grant those token permissions.
The formula branch and PR remain subject to repository rules. Do not copy a
secret value into the repository, logs, an issue or a PR. GitHub does not return stored Actions secret values.

Check the names of inherited organization secrets without retrieving any value:

```sh
gh api repos/arocomputer/aft/actions/organization-secrets --jq '.secrets[].name'
```

`RELEASE_TOKEN` must appear in that list. Organization-wide secret listing and
changes to repository grants require an appropriately authorized org-admin
session: classic `admin:org`, or the corresponding organization Actions-secrets
permission for a fine-grained token. A repository-level credential can inspect
the inherited names through the endpoint above, but cannot change the grant.

If `RELEASE_TOKEN` is unavailable, the formula job fails with a clear setup error.

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
