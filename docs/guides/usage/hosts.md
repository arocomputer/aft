---
title: Hosts
description: The machines Aft runs on, the SSH destination, and choosing between them
order: 4
---

# Hosts

A **host** is a machine Aft runs your agents on. You type on your own computer;
the agent, your code and the work all live on the host. You can have one host or
several — a `mini` under your desk, a `studio` on your network, a cloud box — and
point each project at whichever you want.

Hosts live in `~/.aft/config.toml`, each as a `[host.<name>]` table. The name is
whatever you choose to type; the one required setting is `ssh`, the destination.

```toml
[host.mini]
ssh = "mini"
```

## The `ssh` destination

`ssh` is exactly what you would type after `ssh` on the command line. Aft reaches
the host by running your system's OpenSSH with this value, so anything SSH
accepts works here. There are two forms.

**An alias from your `~/.ssh/config` (recommended).** If you have a block like
this in `~/.ssh/config`:

```
Host mini
    HostName 192.168.1.50
    User fschr
    Port 22
    IdentityFile ~/.ssh/id_ed25519
```

then the host is just:

```toml
[host.mini]
ssh = "mini"
```

**A full destination, spelled out inline.** For a machine you have not put in
`~/.ssh/config`:

```toml
[host.studio]
ssh = "fschr@studio.local"
```

You can include SSH options inline too, for example a non-default port:

```toml
[host.box]
ssh = "-p 2222 fschr@box"
```

### Prefer the alias

The alias form is the one to reach for, because Aft deliberately sits on top of
the SSH setup you already have. Your user, port, key, and any `ProxyJump` or
agent forwarding all come from `~/.ssh/config`; Aft never asks for a password,
stores a key, or manages connection details of its own. Keeping those in
`~/.ssh/config` means:

- Aft's config stays short — `ssh = "mini"` — and reads cleanly.
- There is one source of truth. Change a port or key in `~/.ssh/config` once and
  every tool follows, Aft included; you never edit Aft's config for it.
- A `ProxyJump` bastion, a shared `Match` block, or a hardware key just work,
  because they are already configured where SSH looks.

Use the inline `user@host` form when you simply have not made an SSH-config
entry yet; it is a fine shortcut, not a different mechanism.

## Adding, listing and removing hosts

Your first host comes from setup, which also creates the launchers and the PATH
line:

```sh
aft setup --host mini
```

After that, manage hosts without editing the file by hand:

```sh
aft host                  # list your hosts, marking the one used by default
aft host add studio studio   # add [host.studio] and install the server on it
aft host rm studio        # remove a host from your config
```

`aft host add NAME DESTINATION` writes the `[host.NAME]` table and installs
Aft's server on it in one step. `aft host rm` only edits your local config —
whatever Aft installed on that machine is left as is, so run `aft uninstall` on
the machine itself to clean it up.

## Choosing which host runs your work

When you start an agent or open a shell, Aft decides the host in this order:

| Precedence | Source | Example |
| --- | --- | --- |
| 1 (highest) | the `--host` flag | `aft shell --host studio` |
| 2 | a project's committed `aft.toml` | `host = "mini"` |
| 3 | the top-level `use` key | `use = "mini"` |

### The `use` host

`use` names the host Aft falls back to when nothing more specific applies. Set
it with a verb rather than editing the file:

```sh
aft use studio    # use studio by default from now on
aft use           # print the host currently used by default
```

```toml
use = "studio"
```

**With a single host you need no `use` line at all** — that one host is used by
default. The key only appears once you have more than one host to choose
between, so a simple setup stays simple.

## Pinning a project to a host

To make a project always run on the same host no matter which computer you open
it on, commit an `aft.toml` at the repository root:

```toml
# aft.toml — this repo runs on mini wherever it is cloned
host = "mini"
```

Because it is committed with the code, every machine that clones the repo runs
it on `mini` without any per-machine setup. A committed `aft.toml` wins over
your personal `use`, but `--host` still overrides it for a one-off.

## Several machines

Hosts make two multi-machine setups natural.

**Several hosts, one computer.** Name each host, then target them with `use`,
`--host`, or a per-project `aft.toml`. Different projects can live on different
hosts at once.

**Several computers, one host.** Install Aft on each computer you type on — your
laptop and your desktop, say — and point both at the same host. Because a
session lives on the host and survives you disconnecting, you can start work on
one computer, close it, and reattach the very same running session from the
other. Only one computer is attached to a session at a time; a second attach
takes over, and detaching leaves the session running.

## Per-host settings

A `[host.<name>]` table can also override the global defaults for that one host
— its mirror range, launch wrapper, prediction, passed environment, and SSH
keepalive tuning:

```toml
[host.box]
ssh     = "box"
ports   = "3000-3999"
predict = true
```

See [Configuration](config.md#per-host-settings) for the full list of keys and
how global, host and project settings merge.
