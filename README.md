<div align="center">
  <img src="assets/lockup/white.svg#gh-dark-mode-only" alt="aft" height="52">
  <img src="assets/lockup/black.svg#gh-light-mode-only" alt="aft" height="52">

  <h3>Your agents on your server. Your terminal stays local.</h3>

  <p>Run Claude, Codex, opencode or any terminal agent on your own machine,<br>
  while you type on your laptop with no network lag. Your code stays remote.</p>

  <p>
    <a href="https://github.com/arocomputer/aft/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/arocomputer/aft/ci.yml?branch=main&style=flat-square&color=111111&label=ci" alt="CI"></a>
    <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-111111?style=flat-square" alt="MIT"></a>
  </p>
</div>

<br>

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
aft server         # a terminal on the host called server: ssh that survives disconnects
```

Close the laptop and the agent keeps working. Open it again and you are
reattached, screen intact. Aft adds no hooks, plugins or settings to any agent,
installs nothing but its own small server on the host, and `aft uninstall`
removes every file and line it added.

## Start

Build from source with Go 1.26 or newer, then copy `bin/aft` to a directory on
your `PATH`:

```sh
./x build
./bin/aft help
```

macOS and Linux are supported as both the laptop and the server. Windows users
run Aft inside WSL.

## Docs

Quick reference: `aft help`. The canonical guides live in
[docs/guides](docs/guides/start/README.md).

| | |
| --- | --- |
| [Installation](docs/guides/start/install.md) · [Setup](docs/guides/start/setup.md) | Building aft and connecting a server |
| [Sessions](docs/guides/usage/sessions.md) · [Launchers](docs/guides/usage/launchers.md) | How a remote agent runs, and how `aft on` routes it |
| [Ports](docs/guides/usage/ports.md) · [Mounted projects](docs/guides/usage/mounts.md) | Previews on localhost, and SMB/NFS code mounts |
| [Configuration](docs/guides/usage/config.md) · [Uninstall](docs/guides/usage/uninstall.md) | Per-host settings and the launch wrapper; clean removal |

For development, see [Contributing](CONTRIBUTING.md), the
[design](docs/contributing/plan.md), [testing](docs/contributing/testing.md),
[roadmap](docs/contributing/roadmap.md) and [changelog](CHANGELOG.md). Report
vulnerabilities through the [security policy](SECURITY.md). Brand files are in
[assets](assets/README.md).

<br>

<p align="center"><sub><a href="LICENSE">MIT</a> · Copyright © 2026 Fischer Hunt</sub></p>
