<div align="center">
  <img src="assets/lockup/white.svg#gh-dark-mode-only" alt="aft" height="52">
  <img src="assets/lockup/black.svg#gh-light-mode-only" alt="aft" height="52">

  <h3>Local tools. Remote workspaces.</h3>

  <p>Connect to development directories on your own machines,<br>
  with system OpenSSH and remote tmux. Your code stays remote.</p>

  <p>
    <a href="https://github.com/fschrhunt/aft/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/fschrhunt/aft/ci.yml?branch=main&style=flat-square&color=111111&label=ci" alt="CI"></a>
    <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-111111?style=flat-square" alt="MIT"></a>
  </p>
</div>

<br>

## Start

From a source checkout, build with Go 1.24 or newer:

```sh
./x build
./bin/aft help
```

Linux and macOS are the target client platforms. No public binary release or
installer is available yet. Copy `bin/aft` to a directory on your `PATH` to use
`aft` from anywhere. See [Installation](docs/guides/start/install.md).

Configure your SSH authentication and host trust first. The remote machine needs
a POSIX shell and tmux. Then register an existing remote directory and project:

```sh
aft setup                         # choose an SSH alias and existing remote root
aft add myapp                     # register myapp beneath that root
aft open myapp                    # create or attach to its tmux session
```

<table>
  <tr>
    <td width="33%" valign="top">
      <b>Code stays remote</b><br>
      Work in directories on your own machines. Aft does not upload repositories
      or install remote packages.
    </td>
    <td width="33%" valign="top">
      <b>Sessions keep running</b><br>
      Disconnect and return to the same tmux session with <code>aft open myapp</code>.
      Sessions last until they exit or the remote machine reboots.
    </td>
    <td width="33%" valign="top">
      <b>Your existing SSH</b><br>
      Use system OpenSSH and your configured hosts. Aft does not copy credentials
      or edit shell startup files.
    </td>
  </tr>
</table>

## Work on a project

Run a command over SSH, or start one in a detached tmux session:

```sh
aft exec myapp -- go test ./...
aft run --session myapp-build myapp -- go build ./...
aft project list
```

`exec` returns the remote command's exit status. `run` returns after launching
the session; it does not wait for the command's result. Use `aft new NAME` to
create a new directory beneath your workspace instead of registering an existing
one. More in [Projects and sessions](docs/guides/usage/projects.md).

## Bring a port home

Forward a remote development server to your machine:

```sh
aft ports --project myapp 3000     # localhost:3000, until Ctrl+C
```

Tunnels bind to IPv4 loopback. Port discovery and watching require Linux `ss`
on the remote machine. `aft doctor` checks connectivity and remote tools;
`aft status` inspects sessions and listeners. See
[Ports and services](docs/guides/usage/ports.md) for forwarding, discovery, and
optional background services.

## Docs

Quick reference: `aft help`. The canonical guides live in
[docs/guides](docs/guides/start/README.md); there is no documentation website or
embedded `aft docs` command yet.

| | |
| --- | --- |
| [Installation](docs/guides/start/install.md) · [Workspaces](docs/guides/start/workspaces.md) | Building aft and connecting an existing remote root |
| [Projects and sessions](docs/guides/usage/projects.md) | Registering projects, opening sessions, and running commands |
| [Ports and services](docs/guides/usage/ports.md) | Forwarding ports, diagnostics, and background services |
| [Metadata](docs/guides/usage/metadata.md) | Local records, migration, and cleanup |

For development, see [Contributing](CONTRIBUTING.md),
[testing](docs/contributing/testing.md), [roadmap](docs/contributing/roadmap.md),
and [changelog](CHANGELOG.md). Report vulnerabilities through the
[security policy](SECURITY.md). Brand files are in [assets](assets/README.md).

<br>

<p align="center"><sub><a href="LICENSE">MIT</a> · Copyright © 2026 Fischer Hunt</sub></p>
