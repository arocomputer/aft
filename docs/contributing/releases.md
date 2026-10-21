# Releases and repository setup

Aft has no public binary release yet. Build from source as described in
[installation](../guides/start/install.md). No installer, package registry,
website deployment or public preview service is maintained.

## Initial publication checklist

Before publishing the repository, independently review source and documentation
for secrets and private data, confirm the MIT copyright, and run `./x check` and
`./x release-check`. These commands neither commit nor publish. The Go module is
`github.com/fschrhunt/aft`; `go.mod` owns the minimum Go version.

After creating the repository, the maintainer should configure GitHub:

- Use `main` as the default branch, enable Actions, and enable private vulnerability
  reporting so the reporting path in [SECURITY.md](../../SECURITY.md) is available.
- Require maintainer/code-owner review and the `check (ubuntu-latest)`,
  `check (macos-latest)` and `release-check` CI statuses before merging.
  CODEOWNERS alone does not enforce review.
- Keep workflow token permissions read-only by default. All action references
  are pinned to full commit SHAs, following
  [GitHub's security guidance](https://docs.github.com/en/actions/reference/security/secure-use).
  Dependabot proposes action updates monthly; review SHA/version changes together.
- Leave `RELEASE_ENABLED` unset or false until ready for a binary release.
  Create a `release` environment and configure required reviewer approval and
  branch/tag restrictions appropriate to `main` and version tags before enabling it.
  Restrict who can create or move version tags; never move a published tag.

These are manual repository settings, not claims about an existing public repo.
No commands in the local task entry point change them.

## Qualify a release

1. Move user-visible entries from `## Unreleased` in [CHANGELOG.md](../../CHANGELOG.md)
   into an exact `## X.Y.Z` section with a date and nonempty notes. Keep a fresh
   Unreleased section. Use New features, Improvements and Fixes groups as needed.
   For the first binary release, update source-only availability statements in
   the README, installation guide, changelog and security policy to match what
   will actually be shipped and supported.
2. Run `./x check` and `./x release-check vX.Y.Z` on the final source. The latter
   validates notes and builds all four archives in temporary storage, then removes
   them. It does not require a tag or clean checkout, and does not publish.
3. Review and merge the release commit to `main`. When explicitly ready, enable
   `RELEASE_ENABLED=true`, create an annotated `vX.Y.Z` tag on that commit and push
   that tag. Alternatively, manually dispatch `release` from `main` with an existing
   tag. These are deliberate maintainer actions; they are not part of initial setup.

Only stable `vX.Y.Z` tags are accepted. The resolver requires the tagged commit to
be reachable from `main`, pins subsequent jobs to its SHA, and checks the pushed
source when triggered by a tag. A manual run from any other branch is skipped.

## What the workflow does

The workflow reruns `./x check` on Linux and macOS at the selected commit. A
read-only build job runs `./x dist vX.Y.Z` and uploads the resulting assets. This
builds CGO-disabled executables for Linux and macOS, each on amd64 and arm64,
using Go from the selected commit's `go.mod`. There is no dependency cache.

Each `aft-vX.Y.Z-OS-ARCH.tar.gz` contains the executable, MIT license, README and
documentation. `checksums.txt` records SHA-256 for the four archives. Notes come
from the selected changelog section. The archive filename carries the version;
Aft has no runtime version flag. `go version -m PATH/TO/aft` can inspect Go build
metadata, including the source revision in a clean tagged workflow checkout.
Archives are unsigned; macOS binaries are not signed or notarized. Cross-builds
verify compilation and packaging, not runtime behavior on all four targets.

A separate job behind the `release` environment downloads the same artifacts,
checks their hashes and verifies the remote tag still points to the selected
commit. Only this job has `contents: write`; it executes no repository code and
uses GitHub CLI to create a **draft** release with the archives and checksums.
It never publishes the draft automatically. The maintainer must inspect assets,
notes and test evidence and explicitly publish through GitHub when ready.

No build, tag, release or GitHub operation is executed merely by adding this
workflow to a checkout. With `RELEASE_ENABLED` unset, even tag/manual triggers skip
release jobs.

## Local packaging and recovery

`./x dist vX.Y.Z` is optional local packaging. It requires matching changelog notes
and writes to a new ignored `dist/vX.Y.Z` directory. It refuses to overwrite an
existing directory; inspect and remove only that generated directory before
retrying. It does not check tag ancestry or publish; those checks belong to the
workflow resolver. Local archives may contain uncommitted source.

The workflow refuses to replace an existing draft or published release. If a
failure occurs before draft creation, rerun the failed workflow. If a draft exists,
inspect its assets: a failed upload can leave it incomplete. To rebuild, explicitly
delete only the unpublished draft and rerun; do not move the tag. For an already
published release, make a new version instead. Workflow artifacts expire in seven
days. Builds are not promised to be byte-for-byte reproducible.

After an actual release exists, download its archives and `checksums.txt` together.
Verify with `sha256sum --check checksums.txt` on Linux or
`shasum -a 256 --check checksums.txt` on macOS (download all four archives for that
command). A checksum detects corruption; it is not an independent signature.
