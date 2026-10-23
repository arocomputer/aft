---
title: Ports
description: Dev servers on the server, previews on your machine
order: 3
---

# Ports

While a client is attached, the server reports TCP ports listening on the
server and the client mirrors the ones in range to the same port on your
machine's loopback interface, through the shared SSH connection. A dev server
started remotely opens at `localhost:3000` in your local browser, and a login
callback to `localhost` reaches the application on the server.

```sh
aft ports              # what is mirrored right now
```

The default range is `3000-9999`; set `ports.mirror` in the config or a
repository's `aft.toml` to change it. Mirrors bind only to `127.0.0.1` and never
displace something already listening locally. They end when the client exits.

On Linux servers listeners are read from `/proc/net/tcp`; on macOS servers from
`lsof`. Only loopback and all-interface listeners are reported.
