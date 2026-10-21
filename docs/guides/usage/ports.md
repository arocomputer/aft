---
title: Ports and services
description: Forward loopback ports and inspect remote services
order: 2
---

# Ports and services

```sh
aft ports --project aft 3000
aft ports --project aft --local 8080 3000
aft ports list --project aft
aft ports watch --project aft --min 3000 --max 3999
aft ports server 3000              # explicit host syntax remains supported
aft doctor                        # configured default workspace host
aft doctor server
aft status                        # configured default workspace host
aft status --project aft
aft status server
aft service install --min 3000 --max 3999 server
aft service status server
aft service remove server
```

Ports bind only IPv4 loopback. Explicit tunnels use private SSH connections rather
than existing multiplexed masters, and Aft reaps its SSH child on SIGINT/SIGTERM.
Both port numbers must be 1024–65535; an occupied
local port is never silently replaced. `ports` stays in the foreground until
Ctrl+C. `list` and `watch` require Linux `ss` remotely and include IPv4 wildcard
and loopback listeners, not container-only or IPv6-only bindings. Watch polls
every three seconds, skips occupied local ports, retries failed discovery/tunnels,
and uses separate SSH connections. Its default range is 3000–9999, excluding
5900 and 7377. Choose a narrow range on shared machines. Other local users can
access loopback services; a tunnel is not application authentication or health.

Doctor performs bounded read-only SSH, tmux, `ss`, and disk checks. Status uses
bounded batch SSH for connectivity, sessions and remote listener inspection, plus
a bounded local `ss` (Linux) or `lsof` (macOS) listener inspection. It does not open
tunnels or probe applications. Listener output does not prove Aft ownership or
end-to-end forwarding; unavailable inspection tools are reported explicitly.

Services are opt-in user units. Install the executable at its stable path first;
the unit records the current executable's resolved absolute path. Linux units live
in the platform user config directory under `systemd/user`, macOS units in
`~/Library/LaunchAgents`. Both log to `~/.aft/logs`; logs are not rotated. Linux
requires systemd support for `StandardOutput=append:`. Linux user sessions and
macOS GUI login are prerequisites; Aft does not enable lingering or root services.

Existing units are never overwritten. Failed activation retains the unit for
inspection. Removal accepts failed/inactive or explicitly unloaded services after
checking the manager's state, but refuses cleanup if it cannot confirm a safe stop.
Manager access errors are not treated as proof that a service is stopped. Removal
leaves logs intact. An existing SSH configuration or agent must support batch
(noninteractive) authentication for background operation.
