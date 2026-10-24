---
title: Launchers
description: How claude, codex and others become remote without noticing
order: 2
---

# Launchers

`~/.local/share/aft/launchers` holds one symlink per agent name, each pointing at `aft`. When
`aft` is invoked as `claude`, it checks whether the current directory is inside
a project that is on. If so, it starts `claude` on that project's server, in
the matching directory, passing your arguments through literally. If not, it
runs the real local `claude`, found on `PATH` after the shim directory, and
adds a few milliseconds.

So `aft on` and `aft off` take effect on the next launch, without restarting
anything, and the launchers are safe to leave installed everywhere. Agents are
not configured, hooked or wrapped; any program that runs in a terminal works.

```sh
aft agents add gemini      # a launcher for another program
aft exec -- make test      # one command on the server, no session
```

Setup creates launchers for `claude`, `codex`, `opencode`, `pi`, `gemini`,
`amp`, `aider` and `goose`. Edit `agents` in the config to change the list.

Agent accounts and configuration live on the server, including logins,
`CLAUDE.md`, skills and MCP servers. The first launch on a server goes through
the agent's normal login flow; the login URL opens in your local browser and
any callback port is mirrored to your machine.

`aft doctor` checks that the launchers are first on `PATH`, that each server
has the server and the agents you expect, and that the shell block is in
place.
