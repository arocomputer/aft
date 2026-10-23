---
title: Mounted projects
description: Projects that already live on the server over SMB or NFS
order: 5
---

# Mounted projects

Many setups keep code on the server and view it on the Mac over a network
mount, such as an SMB share or NFS export. The files are the server's; the Mac
only sees them. Aft handles this directly: there is nothing to clone and
nothing to sync, because a command on the server and your editor on the Mac
touch the same files.

Aft needs one fact, which mountpoint on your machine corresponds to which
directory on the server. It tries to work this out for you. The first time you
run `aft on` inside a mounted folder, Aft notices the mount, finds the server
it comes from, asks that server where the share lives, and offers to record the
mapping:

```
This directory is on /Volumes/Room, a SMB mount of server:/home/fschr/room.
Map it so aft uses the files in place (no clone, no sync)? [y/N]
```

Say yes once and every project under that mount maps straight to its real path
on the server. For an SMB share the server path comes from `smb.conf`; for NFS
it is the export path. Aft matches the mount's server to your configured host by
resolved address, so a host named through Tailscale MagicDNS still matches a
mount that names an IP.

If detection cannot tie the mount to a host, record it by hand:

```sh
aft mount add --local /Volumes/Room --host server --remote /home/fschr/room
aft mount list
aft mount remove /Volumes/Room
```

The mapping lives in `~/.aft/config.toml`:

```toml
[[mounts]]
local = "/Volumes/Room"
host = "server"
remote = "/home/fschr/room"
```

Because the files are shared, not copied, editing in a Mac GUI editor and
running builds or tests on the server stay in step with no sync step. This is
the smoothest way to use Aft when your code already lives on the server.
