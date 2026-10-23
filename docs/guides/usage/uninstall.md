---
title: Uninstall
description: Remove everything Aft added, on every machine
order: 4
---

# Uninstall

```sh
aft uninstall
```

After confirming, Aft:

1. Ends its sessions and removes `~/.aft` on every configured server.
2. Removes the `# >>> aft >>>` block from each shell file it edited, but only if
   the block is still exactly what Aft wrote. An edited block is left in place
   and reported for you to remove by hand. The fish drop-in file is deleted.
3. Removes `~/.aft` on your machine: launchers, config, state and sockets.
4. Tells you where the `aft` binary is, so you can delete it the way you
   installed it.

Your code, your SSH configuration and known hosts, and each agent's own login
and data are never touched.

Everything Aft writes lives under `~/.aft` on each machine, plus the recorded
shell block, which is what makes this exact.
