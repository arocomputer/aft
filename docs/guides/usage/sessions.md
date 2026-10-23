---
title: Sessions
description: How a remote agent runs, survives and reattaches
order: 1
---

# Sessions

When you start an agent in a project that is on, Aft starts it on the server
inside a session and attaches your terminal to it. A session is a
pseudo-terminal held open by Aft's helper. It keeps the application running
whether or not anyone is attached, remembers its screen, and answers the
terminal queries an application sends while nobody is there, so the agent
never blocks waiting for a terminal.

```sh
aft ls                 # sessions on every host
aft attach ID          # reattach; with one session, the id is optional
aft detach ID          # detach whoever is attached, from another terminal
aft end ID             # stop the application and the session
aft shell              # a login shell on the server in a session
aft status             # here on/off, hosts, sessions
```

## Typing

Aft's client passes everything the application prints straight through to your
terminal, so colors, mouse, hyperlinks, images and the keyboard protocol behind
keys such as Shift+Enter work exactly as they would locally. It keeps a shadow
of the screen, and when you type a printable character or Backspace it draws
the expected result at the caret immediately, then confirms it against the
server's echo. Wrong guesses are repainted and prediction pauses until echoes
match again. Enter, arrow keys, control keys and pastes are never predicted, so
menus and the agent's reply arrive at network speed, which is dominated by
model time anyway. `predict = false` in the config turns prediction off.

## Disconnects

If the connection drops, the terminal title shows "aft · reconnecting" and the
client reconnects with backoff. The session is untouched. On reattach the
helper repaints the screen, restores terminal modes, and asks the application
to redraw. Ctrl-C while disconnected leaves the client; the session keeps
running and `aft attach` returns to it. Closing the terminal window detaches.

Aft never stops a session on its own. `aft end` does, and so does the
application exiting.

## Environment and paths

The application gets the server user's login environment. Only an allowlist
crosses from your machine: `TERM`, `COLORTERM`, `TERM_PROGRAM`, the locale
variables, and anything in `env.pass`. Your `PATH` never reaches the server.

The working directory on the server is the project's mapped directory plus
your current subdirectory. Code lives only on the server and moves with git;
Aft never syncs or copies files.

## Bridges

Inside a session, `~/.aft/shims` on the server comes first on `PATH` and
provides `open`, `xdg-open`, `pbpaste`, `wl-paste`, `xclip` and `xsel`. When
the application opens an `http` or `https` URL, such as a login page, it opens
in your local browser. When it reads the clipboard, for example to paste an
image, it gets your local clipboard. Anything else is passed to the real
command on the server. Copying text from the application reaches your clipboard
through the terminal's own OSC 52 support, with no shim involved.

Remote commands run with your full permissions on the server. An agent's
sandbox protects only the machine it runs on.
