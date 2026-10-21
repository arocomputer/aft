# Scope and roadmap

The [guides](../guides/start/README.md) describe the delivered CLI contract.
Aft focuses on existing remote machines, OpenSSH and tmux; it has no repository
synchronization, hosted account, remote package installation or credential copying.

## Deliberate limits

- Uninstall remains dry-run only: there is no trustworthy executable ownership
  record. Service removal is explicit and leaves logs.
- Workspace roots and parents must exist. Symlink components are refused.
  A failed registration can leave a newly created remote directory; inspect it
  and register it with `add`. Aft never deletes remote files as rollback.
- Metadata publication is atomic, but parent-directory crash durability is not
  guaranteed. A crashed writer may leave a lock; migration can partially complete.
- Detached jobs report launch status, not eventual exit status, and tmux sessions
  do not survive reboots. Durable job history and terminal reconnect management
  are not implemented.
- Discovery is Linux IPv4-only with a fixed polling interval and one SSH connection
  per port. There is no forwarding registry or authenticated health probe.
- Services require a working user manager. Logs are not rotated, existing directory
  permissions are not changed, and completion is local and basic.

## Next verification work

Prioritize real Linux systemd activation/removal, network interruption and
sleep/wake recovery, and unattended operation. See the
[manual test checklist](testing.md#manual-integration-checklist).

A first binary release, runtime version reporting, broader discovery, and more
robust job lifecycle handling are possible follow-ups, not promised features.
No website, embedded docs, package registry or installer is currently maintained.
