# Aft

Aft connects local tools to development directories on your own machines using
system OpenSSH and remote tmux. Code stays remote. Aft does not upload repositories,
install remote packages, copy credentials, or edit shell startup files.

Build from source with Go 1.24 or newer (no third-party Go dependencies):

```sh
./x build
./bin/aft help
```

Linux and macOS are the target client platforms. No public binary release or
installer is available yet.

The canonical user documentation lives in [docs/guides](docs/guides/start/README.md):

- [Installation](docs/guides/start/install.md) and [workspaces](docs/guides/start/workspaces.md)
- [Projects and sessions](docs/guides/usage/projects.md)
- [Ports, diagnostics and services](docs/guides/usage/ports.md)
- [Metadata, migration and cleanup](docs/guides/usage/metadata.md)

Aft currently has no documentation website or embedded `aft docs` command.

See [Contributing](CONTRIBUTING.md), [testing](docs/contributing/testing.md),
[roadmap](docs/contributing/roadmap.md), [changelog](CHANGELOG.md), and
[security policy](SECURITY.md).

[MIT](LICENSE) · Copyright (c) 2026 Fischer Hunt
