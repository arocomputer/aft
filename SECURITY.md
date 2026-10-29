# Security policy

Aft is in early 0.x development. Security fixes target the latest published
version and `main`; older 0.x versions have no separate maintenance branches.
If no version has been published yet, fixes target `main`. Commands and
configuration may change during 0.x development.

## Reporting a vulnerability

Do not disclose vulnerabilities, credentials or private host details in public
issues. Private vulnerability reporting is enabled for the repository. Use
GitHub's **Security → Report a vulnerability** at
[fschrhunt/aft](https://github.com/fschrhunt/aft/security).
No response-time guarantee is currently offered.

## Scope and boundaries

Aft uses your existing SSH authentication and host trust. Applications in a
session run with your remote account's full permissions; an agent's sandbox
protects only the machine it runs on. Aft's own files live under `~/.aft` and
`~/.local/share/aft` on each machine with private permissions, and its sockets are Unix sockets owned
by you, so other processes running as you can reach your sessions. Mirrored
ports listen on loopback only but are reachable by other local users. A
session reached over UDP listens on one port in 60000-61000 on every interface
of the host, like mosh; it admits only a client that pins the session's
certificate and presents the token sent over SSH, and closes with the session.
The
bridge shims let an application on the server open URLs in your browser and
read your clipboard; they are available only inside sessions you started.

Relevant reports include: command or escape-sequence injection through the
protocol or the predictor, a way for one user's session to reach another's,
leaking environment variables beyond the allowlist, unintended public
listeners, and `aft uninstall` removing something Aft did not add. Automated
checks and cross-builds are not a security audit.
