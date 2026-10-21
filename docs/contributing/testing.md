# Testing

Run `./x check` for formatting, vet, race-enabled tests, infrastructure tests,
documentation checks and a native build. Run `./x release-check` to compile and
inspect Linux/macOS amd64/arm64 archives and their checksums without publishing.
Tests use temporary homes and fake SSH/service executables; they do not need
credentials or access to real development machines.

The Go tests cover literal arguments, exit codes, path containment, collisions,
metadata permissions and symlinks, migration, service state handling, local-only
completion and uninstall, and tunnel child-process cleanup. CI repeats the suite
on Linux and macOS. Cross-compilation does not establish target runtime behavior.

## Manual integration checklist

Use disposable local metadata and a disposable remote workspace with existing
SSH authentication and trusted host keys. Do not overwrite your normal config.
Only enable services deliberately, with a stable test binary path; service files
live outside Aft metadata and must be removed explicitly after the exercise.

1. Exercise interactive setup and picker, workspace defaults/removal, and paths
   with spaces and apostrophes. Confirm unknown host keys fail validation.
2. Create and register projects; check duplicate, traversal, symlink and unknown
   project rejection. Confirm record removal retains remote files.
3. Run literal arguments and nonzero exit codes; attach/detach/reopen tmux, verify
   retained shell state, and reject duplicate detached session names.
4. Load bash/zsh completions and exercise actual completion keys.
5. Forward a disposable HTTP listener. Check responses, occupied-port recovery,
   listener stop/restart, and SIGINT/SIGTERM child/listener cleanup.
6. Inspect doctor/status; distinguish listener observations from forwarding health.
7. On each platform, activate and remove a disposable user service; inspect log
   paths, failed activation, inactive/unloaded states, and unavailable managers.
8. Migrate disposable legacy records; confirm originals remain and repeats fail.
   Confirm uninstall dry-run changes nothing.
9. Exercise network loss, sleep/wake, reboot and extended unattended operation.

Remove test services, sessions, listeners and disposable directories explicitly.
Record OS/tool versions, source commit, checks and outcomes in the review; do not
commit private hosts, logs or machine-specific artifact paths.

## Prior evidence and limits

Development notes from 2026-10-21 report a macOS ARM64 client against Linux passing
28 manual checks, including tmux, forwarding and launchd cleanup. They also report
finding and fixing an explicit-tunnel SIGTERM child leak, now covered by a focused
subprocess regression test. The manual harness is not part of this repository;
these notes are historical evidence, not a reproducible automated suite.

That review did not establish Linux systemd runtime behavior, interactive setup,
actual completion key presses, sleep/wake or network partition recovery, reboot
behavior, hostile concurrent directory replacement, or long-duration operation.
Restarting a listener is not a network-loss test. None of this is a security audit.
