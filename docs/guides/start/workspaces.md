---
title: Workspaces
description: Register existing remote roots through SSH
order: 2
---

# Workspaces

Configure authentication and host trust in your existing SSH setup first. Setup
only registers an existing remote root; it does not create roots, install tools,
or change SSH settings. The remote needs a POSIX shell, and tmux for sessions.

```sh
aft setup                         # prompts for SSH alias and remote root
aft setup --name dev --host server --root '~/repos'
aft workspace add spare other-server '~/repos'
aft workspace list
aft workspace default dev
aft workspace remove spare
```

Choose either interactive setup or its flag equivalent, not both for the same
name. Quote `~/...` so your local shell does not expand it. Aft expands it on the
remote and saves `pwd -P`, a canonical absolute directory. `/` is not an allowed
workspace root. Setup and workspace validation use batch SSH, a five-second
connection timeout and a fifteen-second total deadline. Authentication must work
without prompts, with already trusted host keys; validation disables host-key
updates and refuses unknown keys. Failed validation saves nothing. Existing workspace names are
never replaced. The first workspace becomes default; setup explicitly selects
its workspace as default. Removing the default clears the selection. Workspaces
referenced by project records cannot be removed.

Next: [projects and sessions](../usage/projects.md).
