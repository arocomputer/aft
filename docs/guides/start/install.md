---
title: Installation
description: Build and install from source
order: 1
---

# Installation

Aft uses local system OpenSSH and remote tmux. Code stays on your remote machine.
There are no hosted accounts or third-party Go dependencies. Linux and macOS
are the target client platforms; port discovery requires Linux `ss` remotely.

No public binary release or installer is available yet. From a source checkout,
build with Go 1.24 or newer:

```sh
./x build
./bin/aft help
```

To install manually, copy `bin/aft` to a directory on your `PATH`, such as
`~/.local/bin`. Aft never installs itself or edits your shell startup files.
See [workspaces](workspaces.md) to configure an existing remote directory.
