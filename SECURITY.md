# Security policy

Aft is in initial development; there are no published releases or supported
maintenance branches yet. Fixes target the current development branch.

## Reporting a vulnerability

Do not disclose vulnerabilities, credentials or private host details in public
issues. Once private vulnerability reporting is enabled for the repository, use
GitHub's **Security → Report a vulnerability** at
[fschrhunt/aft](https://github.com/fschrhunt/aft/security).
If that option is unavailable, open an issue requesting a private reporting
channel without including vulnerability details. No response-time guarantee is
currently offered.

## Scope and boundaries

Aft uses your existing SSH authentication and host trust. Explicit remote commands
run with your remote account's permissions. Workspace path validation is not a
sandbox against hostile concurrent renames or commands you choose to run.
Loopback listeners are reachable by other local users; forwarding does not add
application authentication. Local metadata permissions do not protect against
other processes running as you.

Unexpected command injection, credential exposure, path validation bypasses,
unintended public listeners and unsafe deletion are relevant reports. See the
[guides](docs/guides/usage/README.md) for operational limits. Automated checks and
cross-builds are not a security audit.
