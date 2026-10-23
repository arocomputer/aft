# Scope and roadmap

[The design](plan.md) describes what Aft is and the decisions behind it. The
[guides](../guides/start/README.md) describe the delivered behavior. The
[client roadmap](client-roadmap.md) tracks terminal-client and session
features for power users, copy and paste among them.

## Deliberate limits

- Code lives only on the server and moves with git. There is no sync and none
  is planned until users ask for it.
- Only printable characters and Backspace are predicted. Menus, popups and the
  agent's replies arrive at network speed.
- The transport is TCP over the user's OpenSSH. Lossy links delay output,
  though not typed characters. UDP is a later option.
- Remote commands run with the user's full permissions on the server. There is
  no remote sandbox.
- Agent accounts and configuration live on the server; nothing follows the user
  from the laptop.
- Sessions do not survive a server reboot.
- Windows is supported only through WSL.

## Next verification work

- Hands-on typing checks on real networks with each supported agent, and on
  lossy links.
- Image paste for agents running on macOS servers, where some agents read the
  clipboard in ways the shims cannot intercept.
- Reattach fidelity for applications that draw images.

## Possible follow-ups

Opt-in sync for teams on rented servers, a UDP transport, per-user agent
configuration that follows the user, and native integration with agents that
separate their interface from their engine. None is promised.
