---
title: Projects and sessions
description: Create or register projects and run remote commands
order: 1
---

# Projects and sessions

Put flags before positional arguments:

```sh
aft new myapp                      # mkdir beneath the default workspace
aft add repos/aft                  # register an existing remote directory
aft add --workspace dev --name api services/api.v2
aft project list
aft open aft
aft exec aft -- go test ./...
aft run --session aft-build aft -- go build ./...
aft run aft -- sh -c 'make && exec sh'
aft project remove api
```

`new` creates exactly one directory; any parent must already exist. It refuses
existing files, directories, and symlinks. `add` never creates remote files.
Paths must be relative to the selected workspace, without empty, `.` or `..`
components. Directory paths must be physical paths beneath the workspace:
symlink components are rejected, including links pointing within the workspace.
Opening or executing a workspace project checks containment again in the same
SSH invocation. This is validation of user-owned directories, not a sandbox
against another process concurrently renaming directories. Explicitly invoked
commands can themselves access paths outside the project.

The default alias comes from the final path component, replacing punctuation
with hyphens and trimming to 60 characters. Use `--name` for a different alias.
Names contain letters, digits, hyphens and underscores, start with a letter or
digit, and are at most 60 characters. Collisions fail without overwriting records.
If directory creation succeeds but the connection or metadata save fails, the
remote directory is retained; inspect it and use `add` to register it. Aft never
rolls back by deleting remote files. Project removal deletes only its local record.

`open` attaches to or creates `aft-NAME`. No arguments presents a numbered local
project picker in a terminal; redirected input/output shows help. Terminal detection
uses the standard `stty -g` read-only check; if unavailable, Aft shows help. `exec` runs a
literal argument vector and returns SSH's exit status, including the remote
command's exit code. Use an explicit `sh -c` for shell operators.

`run` creates a detached tmux session and fails if that session already exists.
Its default is `aft-NAME`, so `aft open NAME` can attach to it. If an interactive
session with that name already exists, choose `--session aft-build`. `run` returns
the launch status, not the eventual command exit code. Sessions end when their
command exits and do not survive server reboots. Detach with tmux's prefix then
`d` (normally Ctrl+B, D). Aft never kills existing sessions.

Explicit host workflows remain available:

```sh
aft connect server                # default session aft-main
aft connect --session custom --dir /srv/app server
aft sessions server
aft exec --host server --dir /srv/app -- go test ./...
aft run --host server --session aft-job --dir /srv/app -- make
aft project add oldapp server /srv/app
```

Direct-host `exec` and `run` require `--host`, omit PROJECT, and still require
`--` before COMMAND. Unknown projects never fall back to SSH hosts. Explicit
session names are preserved; only defaults are Aft-prefixed. `--dir` affects only
new sessions and must refer to an existing remote directory. In host mode relative
paths are relative to the remote home; use an absolute path instead of literal
`~/`. Explicit-host project records are outside workspace containment and are
validated when used by SSH/tmux, as in the original low-level workflow.
