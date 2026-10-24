---
title: Sessions
description: How a remote agent runs, survives and reattaches
order: 1
---

# Sessions

When you start an agent in a project that is on, Aft starts it on the server
inside a session and attaches your terminal to it. A session is a
pseudo-terminal held open by Aft's server. It keeps the application running
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
server repaints the screen, restores terminal modes, and asks the application
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
Aft never syncs or copies files, except a file you paste (see below).

## Copy and paste

Copy and paste are your terminal's job, and aft keeps them working because it
passes the server's output through to your terminal untouched. Your terminal
owns the screen, the selection and the clipboard; aft does not get in the way.

- **Select and copy.** Drag to select and press Cmd+C (Ctrl+Shift+C on Linux)
  as always. For a full-screen agent such as Claude Code, Codex or opencode,
  the application captures the mouse, so hold Option while dragging in
  iTerm2 and Terminal, or Ctrl in most Linux terminals, to select the screen
  instead. This is standard terminal behavior, the same as plain SSH.
- **Paste.** Cmd+V (Ctrl+Shift+V) pastes as always. aft forwards the paste
  whole, using bracketed paste when the application asked for it, so editors
  and agents treat it as pasted text rather than typed keystrokes.
- **Paste a file.** Drag a file into the terminal, or copy it in Finder and
  paste, and the terminal pastes its local path. When a paste is nothing but
  paths to files on your machine, aft uploads them over the existing
  connection and pastes the server paths instead, so the agent can open a
  screenshot, recording or document. The window title shows the upload; keys
  you type meanwhile are sent after it, in order. A paste of ordinary text, or
  of a path that does not exist on your machine, is sent unchanged.

  Pasted files are temporary. They go in a private `aft-<uid>/clipboard`
  directory in the server's temporary space: `/var/tmp` on Linux, which is on
  disk and survives a reboot, and your own temporary directory on macOS,
  which only you can read. Aft removes them after a week, the system may
  clear them sooner (macOS after three days unused), and `aft uninstall`
  removes them.
- **Copy from the agent.** When the agent or a tool on the server copies to the
  clipboard with the OSC 52 escape, aft sets your laptop clipboard directly,
  so it works even when your terminal blocks OSC 52 by default.
- **Paste an image.** Agents paste an image by reading the clipboard when you
  press Ctrl-V. The server may read your laptop clipboard only in the few
  seconds after you paste or press Ctrl-V; at any other time the read is
  refused, so nothing on the server can watch your clipboard.
- **Scrollback.** For ordinary command output your terminal keeps the history
  and scrolls as usual. A full-screen agent draws its own screen, so scrolling
  is the application's, reached with its keys or the mouse wheel. For history
  that aft itself holds, use copy mode below.

## Copy mode

Copy mode is aft's scrollback viewer. It freezes the screen into a pager you
drive with the keyboard, over the history the server kept for the session, so
it works the same in any terminal and in a full-screen agent.

Open it with the aft prefix, then `[`. The prefix is Ctrl-] by default; set
`prefix` in the config to change or disable it. Inside copy mode:

- **Scroll** with the arrows, PgUp and PgDn, `g` for the top and `G` for the
  bottom.
- **Search** with `/`, then text and Enter; `n` and `N` move to the next and
  previous match toward older and newer output.
- **Copy** by pressing `v` to start selecting, moving to extend, then `y` to
  copy the lines to your clipboard and leave. You can also just drag-select the
  frozen text and press Cmd+C, because copy mode does not capture the mouse.
- **Leave** with `q` or Escape.

The snapshot is taken when you open copy mode, so it is a still view; new output
keeps arriving underneath and is there when you leave. The prefix also detaches:
Ctrl-] then `d` leaves the session running.

## Bridges

Inside a session, `~/.aft/shims` on the host comes first on `PATH` and
provides `open`, `xdg-open`, `pbpaste`, `wl-paste`, `xclip` and `xsel`. When
the application opens an `http` or `https` URL, such as a login page, it opens
in your local browser. When it reads the clipboard right after you paste, for
example to paste an image, it gets your local clipboard. Anything else is
passed to the real command on the server.

Remote commands run with your full permissions on the server. An agent's
sandbox protects only the machine it runs on.
