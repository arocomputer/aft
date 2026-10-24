---
title: Configuration
description: Hosts, per-project settings, and the launch wrapper
order: 5
---

# Configuration

Aft ships with defaults that need no configuration. Everything below is
optional, and every key has a sensible default, so the one-command path stays
the same until you want to change something. Top-level keys set defaults; a
`[host.<name>]` table overrides them for one host; a repository's `aft.toml`
overrides them for one project. Project wins over host wins over global.

Keys are flat and `snake_case`. A host is addressed by name, so its table is
singular: `[host.mini]`, not `[hosts.mini]`. Naming hosts, the `ssh`
destination, choosing between several, and using more than one machine all live
in [Hosts](hosts.md); this page is the settings reference.

## The launch wrapper

The wrapper is how you bring your own environment. By default aft runs the
agent in a login shell on the host. A wrapper replaces that, running the agent
as `wrapper + command`, so you decide the environment it starts in:

```toml
# ~/.aft/config.toml — a whole host runs agents inside a dev container
[host.box]
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

A `[host.<name>]` table can override the global defaults for that host:

```toml
[host.box]
ssh     = "-p 2222 fschr@box"      # destination, with ssh options if needed
wrapper = ["nix", "develop", "-c"] # command wrapper
predict = true                     # typing prediction for this host
udp     = true                     # reach sessions over UDP 60000-61000 when allowed
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
edit_dotfiles = false   # aft prints the line and never edits a file
```

The default is `true`, which adds a marked block to your shell config and
removes it on `aft uninstall`. With it off aft edits nothing, so there is
nothing to undo.

## Global keys

```toml
use          = "mini"          # host used when --host is omitted (optional with one host)
existing     = "ask"           # an agent launched where one runs: ask, attach or new
agents       = ["claude", "codex", "opencode", "pi"]
tools        = ["git", "gh"]   # run once on the host where aft is on; none by default
predict      = true
udp          = true            # sessions over UDP when the host allows it, as mosh does
prefix       = "ctrl-]"        # opens copy mode (prefix then [) and detach (then d); "" disables
detach       = ""              # a second, direct detach key; off by default
wrapper      = []              # default wrapper for every host
env          = []              # variables to pass to every host
ports        = "3000-9999"     # default mirror range
```
