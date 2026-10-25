---
title: Installation
description: Install a release archive or build Aft from source
order: 1
---

# Installation

Aft is one binary on your laptop. It carries a small server for each host
platform and installs the right one over SSH the first time you connect, so
there is nothing to install on the server by hand.

## Homebrew

After the first release is published and its formula job succeeds, this
repository also acts as its own Homebrew tap, like the other arocomputer CLIs:

```sh
brew tap arocomputer/aft https://github.com/arocomputer/aft
brew install aft
```

The formula installs the checksum-verified prebuilt binary for your platform.
It does not run setup or edit shell configuration; run `aft setup` yourself.
Until `aft.rb` exists on the published repository's `main` branch, use an archive
or source build below.

Upgrade with `brew update` then `brew upgrade aft`. For full removal, run
`aft uninstall` to remove Aft's state and servers, then `brew uninstall aft` to
remove the package-managed client binary.

## Release archives

Download an archive and `checksums.txt` from the same published
[GitHub release](https://github.com/arocomputer/aft/releases). If there is no
published release yet, use the source build below. You do not need Go or Python
to use a release binary.

Choose the archive for the machine where your terminal runs:

| Machine | Archive suffix |
| --- | --- |
| Linux, Intel or AMD 64-bit | `linux-amd64.tar.gz` |
| Linux, ARM 64-bit | `linux-arm64.tar.gz` |
| Mac, Apple silicon | `darwin-arm64.tar.gz` |
| Mac, Intel | `darwin-amd64.tar.gz` |

The first release version is `v0.1.0`; for example,
`aft-v0.1.0-darwin-arm64.tar.gz`. Check your machine with `uname -sm` if unsure.
WSL users select the Linux archive matching their WSL architecture.

Verify the downloaded archive before extracting it. From the directory
containing it and `checksums.txt`, use the appropriate command:

```sh
# Linux
grep ' aft-v0.1.0-linux-amd64.tar.gz$' checksums.txt | sha256sum --check -

# Apple silicon Mac
grep ' aft-v0.1.0-darwin-arm64.tar.gz$' checksums.txt | shasum -a 256 --check -
```

Use your chosen filename in both the checksum and extraction commands. Both
checksum tools must report `OK`. Download checksums from the trusted release;
they detect corruption, but are not an independent signature.

Extract into a new directory and install only the `aft` executable:

```sh
mkdir aft-release
tar -xzf aft-v0.1.0-darwin-arm64.tar.gz -C aft-release
mkdir -p "$HOME/.local/bin"
install -m 755 aft-release/aft "$HOME/.local/bin/aft"
"$HOME/.local/bin/aft" version
```

If `aft` is not found after installation, add `~/.local/bin` to your shell's
`PATH` using your usual shell configuration. Archives include the license,
README, brand assets and guides for offline reading.

macOS binaries are not notarized, and the archives are unsigned. macOS may ask
you to approve a downloaded executable in System Settings → Privacy & Security.
Only approve the binary after verifying its source and checksum; do not disable
Gatekeeper globally.

## Build from source

From a source checkout, use Go 1.26.10 or newer:

```sh
./x build
./bin/aft version
```

Copy `bin/aft` to a directory on your `PATH`, such as `~/.local/bin`. Use
`./x build`, rather than a plain `go build`, to include all server binaries.

## Requirements

- **Laptop**: macOS or Linux, OpenSSH, and a terminal application.
- **Server**: macOS or Linux, reachable with `ssh` using your existing
  configuration, keys and agent. Your agents must be installed there; Aft
  never installs them.
- **UDP**: optional. A direct path to UDP ports 60000–61000 lets Aft use QUIC;
  it falls back to SSH when that path is unavailable.

Windows users run Aft inside WSL. Sessions survive client disconnects, but not
server reboots.

## Upgrade

For an archive installation, replace the installed `aft` executable with one
from a verified newer release. Homebrew installations should use `brew upgrade aft`.
Aft updates an older server on the next connection; it does not replace a newer
server with an older client build. Running sessions keep their existing server
process until they end. Keep the previous client binary if you need to roll
back; a rollback does not downgrade the server.

Then run [setup](setup.md), or `aft doctor` if upgrading an existing setup.
