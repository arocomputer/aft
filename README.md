# Aft

Aft runs your coding agents on your server while the terminal stays on your
laptop. Typing feels local because Aft predicts and draws each keystroke on your
machine, then reconciles it with the server. Everything else, the agent, its
tools, builds, tests and git, runs where your code lives.

```sh
aft setup          # once: add a server
cd ~/code/app
aft on             # agents started here now run on the server
claude             # same command as always; also codex, opencode, pi, any program
aft off            # back to fully local
```

Close the laptop and the agent keeps working. Open it again and you are
reattached, screen intact. Aft adds no hooks, plugins or settings to any agent,
installs nothing but its own small helper on the server, and `aft uninstall`
removes every file and line it added.

Build from source with Go 1.26 or newer:

```sh
./x build
./bin/aft help
```

macOS and Linux are supported as both the laptop and the server. Windows users
run Aft inside WSL.

The user documentation lives in [docs/guides](docs/guides/start/README.md):

- [Installation](docs/guides/start/install.md) and [setup](docs/guides/start/setup.md)
- [Sessions](docs/guides/usage/sessions.md), [launchers](docs/guides/usage/launchers.md) and [ports](docs/guides/usage/ports.md)
- [Uninstall](docs/guides/usage/uninstall.md)

See [Contributing](CONTRIBUTING.md), the [design](docs/contributing/plan.md),
[testing](docs/contributing/testing.md), [roadmap](docs/contributing/roadmap.md),
[changelog](CHANGELOG.md) and [security policy](SECURITY.md).

[MIT](LICENSE) · Copyright (c) 2026 Fischer Hunt
