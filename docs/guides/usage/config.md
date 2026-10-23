---
title: Configuration
description: Per-host and per-project settings, and the launch wrapper
order: 5
---

# Configuration

Aft ships with defaults that need no configuration. Everything below is
optional, and every key has a sensible default, so the one-command path stays
the same until you want to change something. Global keys set defaults; a host
overrides them for one server; a repository's `aft.toml` overrides them for one
project. Project wins over host wins over global.

## The launch wrapper

The wrapper is how you bring your own environment. By default aft runs the
agent in a login shell on the server. A wrapper replaces that, running the
agent as `wrapper + command`, so you decide the environment it starts in:

```toml
# ~/.aft/config.toml — a whole server runs agents inside a dev container
[hosts.box]
ssh = "box"
wrapper = ["docker", "exec", "-i", "dev"]
```

```toml
# aft.toml at a project root — this project runs inside its Nix shell
wrapper = ["nix", "develop", "-c"]
```

Other uses are a specific shell, `mise exec --`, `direnv exec .`, or a script
that sets up secrets. Aft does not interpret the wrapper; it runs it. Bridges
keep working when the wrapper inherits the environment.

## Per-host settings

A host entry can override the global defaults for that server:

```toml
[hosts.box]
ssh     = "-p 2222 fschr@box"   # destination, with ssh options if needed
home    = "~/.aft"                 # helper home on the server
wrapper = ["nix", "develop", "-c"] # command wrapper
predict = true                     # typing prediction for this host
ports   = "3000-3999"              # mirror range
env     = ["DATABASE_URL"]         # extra variables to pass
alive   = 5                        # ServerAliveInterval seconds
persist = 600                      # ControlPersist seconds
```

Tune `alive` and `persist` down for a flaky link, or up for a distant one.

## Shell PATH integration

`aft setup` asks how to put its launchers on your `PATH`. If you manage your
dotfiles, pick the option that prints the line and apply it yourself. That is
recorded as:

```toml
path_integration = "manual"   # aft prints the line and never edits a file
```

The default is `"edit"`, which adds a marked block to your shell config and
removes it on `aft uninstall`. Manual mode edits nothing, so there is nothing
to undo.

## Global keys

```toml
default_host = "box"
agents       = ["claude", "codex", "opencode", "pi"]
predict      = true
detach_key   = "ctrl-]"        # off by default
wrapper      = []              # default wrapper for every host
[env]
pass = []                      # variables to pass to every host
[ports]
mirror = "3000-9999"
```
