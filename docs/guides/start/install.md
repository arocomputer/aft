---
title: Installation
description: Build and install Aft from source
order: 1
---

# Installation

Aft is one binary on your laptop. It carries a small helper for each server
platform and installs the right one over SSH the first time you connect, so
there is nothing to install on the server by hand.

No binary release exists yet. Build from a source checkout with Go 1.26 or
newer:

```sh
./x build
./bin/aft help
```

Copy `bin/aft` to a directory on your `PATH`, such as `~/.local/bin`. Then run
[setup](setup.md).

Requirements:

- **Laptop**: macOS or Linux, OpenSSH, and a terminal application.
- **Server**: macOS or Linux, reachable with `ssh` using your existing
  configuration, keys and agent. Your agents must be installed there; Aft
  never installs them.

Windows users run Aft inside WSL.
