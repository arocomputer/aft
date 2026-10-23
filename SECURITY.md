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

Aft uses your existing SSH authentication and host trust. Applications in a
session run with your remote account's full permissions; an agent's sandbox
protects only the machine it runs on. Aft's own files live under `~/.aft` on
each machine with private permissions, and its sockets are Unix sockets owned
by you, so other processes running as you can reach your sessions. Mirrored
ports listen on loopback only but are reachable by other local users. The
bridge shims let an application on the server open URLs in your browser and
read your clipboard; they are available only inside sessions you started.

Relevant reports include: command or escape-sequence injection through the
protocol or the predictor, a way for one user's session to reach another's,
leaking environment variables beyond the allowlist, unintended public
listeners, and `aft uninstall` removing something Aft did not add. Automated
checks and cross-builds are not a security audit.
