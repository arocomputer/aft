---
title: Setup
description: Add a server and turn a project on
order: 2
---

# Setup

Aft uses the SSH configuration you already have. Make sure `ssh box` works
first, then:

```sh
aft setup --host box
```

Setup connects, installs Aft's server under `~/.local/share/aft` on the host,
reports which agents it finds there, creates launchers for your agents under
`~/.local/share/aft/launchers`, and asks how to put that directory first on your `PATH`. If you
manage your dotfiles, choose the option that prints the line for you to add;
otherwise aft adds a marked block to your shell config:

```sh
# >>> aft >>>
export PATH="$HOME/.local/share/aft/launchers:$PATH"
# <<< aft <<<
```

For fish the block goes in `~/.config/fish/conf.d/aft.fish`. Aft backs up any
file before editing it and records exactly what it added, so
[uninstall](../usage/uninstall.md) can remove it. Open a new terminal
afterwards.

## Turn a project on

```sh
cd ~/code/app
aft on
```

The project is exactly the folder you are in. It maps to the same path
relative to home, on the host, here `~/code/app`, and `aft on` creates that
directory there if it is missing. Pass `--dir` for a different server path, or
`--host` for a different server.

From then on, starting `claude`, `codex`, `opencode`, `pi` or any program with a
launcher from inside that directory runs it, on the host. `aft off` turns it
back off. Nothing else changes.

To start from a repository that only exists remotely, make an empty local
directory for it and run `aft on` there. With `git` set up as a tool (see
[Launchers](../usage/launchers.md)), `git clone URL .` then clones it on the
host.

## Settings

`~/.aft/config.toml` holds your hosts and preferences:

```toml
use = "box"
agents = ["claude", "codex", "opencode", "pi"]
predict = true
detach = ""                  # for example "ctrl-]"; off by default
env = []                     # extra variables allowed to cross to the host
ports = "3000-9999"          # mirror range

[host.box]
ssh = "box"                  # the destination given to ssh
```

A repository may commit an optional `aft.toml` with `host`, `dir` and
`ports.mirror` so a team shares the same mapping. Each person maps the host
name to their own SSH destination in their config.

Per-host overrides, the launch wrapper and the PATH choice are covered in
[configuration](../usage/config.md).

Next: [sessions](../usage/sessions.md).
