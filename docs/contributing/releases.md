# Releases and repository setup

Aft has no public binary release yet. Build from source as described in
[installation](../guides/start/install.md).

## Repository setup

Use `main` as the default branch, enable Actions, and enable private
vulnerability reporting so the path in [SECURITY.md](../../SECURITY.md) works.
Require review and the `check (ubuntu-latest)`, `check (macos-latest)` and
`release-check` CI statuses before merging. Pin action references to full
commit SHAs. Leave `RELEASE_ENABLED` unset until ready for a binary release,
then create a `release` environment with required reviewers.

## Qualify a release

1. Move user-visible entries from `## Unreleased` in
   [CHANGELOG.md](../../CHANGELOG.md) into an exact `## X.Y.Z` section with a
   date, keeping a fresh Unreleased section.
2. Run `./x check` and `./x release-check vX.Y.Z`. The latter cross-compiles the
   helper for every platform, builds `aft` for Linux and macOS on amd64 and
   arm64 with those helpers embedded, and inspects each archive.
3. Complete the [manual checks](testing.md#manual-checks-before-a-release).
4. Merge, enable `RELEASE_ENABLED=true`, and push an annotated `vX.Y.Z` tag, or
   dispatch the workflow manually with an existing tag.

## What the workflow does

The workflow reruns `./x check` on Linux and macOS, runs `./x dist vX.Y.Z` and
uploads the archives, then, behind the `release` environment, verifies hashes
and the tag and creates a **draft** release. Nothing is published
automatically.

Each archive contains `aft`, which embeds the helpers, plus the license,
README and documentation. Archives are unsigned; macOS binaries are not
notarized. `aft version` prints the build version.

## Local packaging

`./x dist vX.Y.Z` writes archives, `checksums.txt` and release notes to a new
`dist/vX.Y.Z` directory and refuses to overwrite an existing one. Verify
downloads with `sha256sum --check checksums.txt` or
`shasum -a 256 --check checksums.txt`.
