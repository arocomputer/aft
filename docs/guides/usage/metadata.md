---
title: Metadata and cleanup
description: Local records, migration, completion and removal
order: 3
---

# Metadata and cleanup

## Local files

All new Aft metadata lives under `~/.aft`:

- `config.json`: named workspaces and `default_workspace`.
- `projects/NAME.json`: remote project records.
- `logs/dev.aft.ports.ID.log`: background service stdout/stderr.
- `config.lock`: temporary writer lock directory; after a crashed writer, inspect
  that no writer remains before manually removing it. No persistent state store
  is currently needed.

Records and config are written atomically with mode 0600; new metadata directories
use 0700. Config updates serialize through a lock and project writes never replace
existing names. Symlink metadata files and directory ancestors are refused.
Existing directory permissions are not changed. `aft paths` shows effective paths,
the running executable, OS service paths, and the legacy migration source.

## Legacy migration

Migration is explicit, local, and does not connect to SSH:

```sh
aft project migrate
aft project migrate --from /absolute/path/to/legacy/projects
```

The default source is the old platform config location: `$XDG_CONFIG_HOME/aft/projects`
(or `~/.config/aft/projects`) on Linux and `~/Library/Application Support/aft/projects`
on macOS. All source records are validated and all destinations checked before
copying. Existing destinations, symlink records, and malformed records cause
failure. Originals remain untouched. A concurrent conflict or I/O failure during
copying may leave a partial migration; copied records remain and are never silently
replaced. Migrated records retain their explicit host/path and do not automatically
acquire a workspace. To adopt one, remove its record and use `aft add`.

## Completion and cleanup

Load completions explicitly into your current shell; Aft never edits startup files:

```sh
source <(aft completion bash)       # bash
source <(aft completion zsh)        # zsh, after compinit
aft uninstall --dry-run
```

Project and workspace candidates are loaded dynamically from local metadata,
without SSH. Completion is intentionally basic; it does not complete remote paths
or SSH destinations.

Uninstall is **dry-run only**: it inventories metadata and Aft-named user service
files, identifies paths and cleanup constraints, and changes nothing. `--yes` is
rejected because there is no trustworthy executable installation ownership record.
Use `service remove HOST` for managed services and review remaining files manually.
Repositories, remote files, tmux sessions, arbitrary executables, SSH/shell config,
legacy metadata and symlink targets are never deleted by uninstall.
