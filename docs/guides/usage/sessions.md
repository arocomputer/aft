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
aft attach fox         # reattach to the session called fox
aft attach             # reattach; picks from a list when several are running
aft kill fox           # stop the application and the session
aft server             # a terminal on the host called server
aft server uptime      # run one command there, as ssh does
aft status             # here on/off, hosts, sessions
```

Each session is named with a short animal word, unique on its host. `aft ls`
says what each one is: the program, and the window title it sets. Claude Code
titles its window with the conversation's topic, so its sessions read like
`Claude · Fix the login bug`. Aft only reads the title the program already
sends to the terminal; it never reads an agent's files.

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
running and `aft attach` returns to it.

A session detaches when its terminal goes away and keeps running on the server
with no terminal. That happens when you close the tab or window, when another
computer attaches (one terminal shows a session at a time), and when a laptop
sleeps or drops off the network: the client sends a heartbeat every ten
seconds, and after thirty silent seconds the session lets it go. `aft ls` says
which sessions are attached.

## Over UDP

Like mosh, Aft reaches a session over UDP when it can, and uses SSH only to
get in. The session opens a UDP port in mosh's range, 60000 to 61000, and
tells your machine over SSH which port, its certificate's fingerprint and a
one-time token. Your machine then connects directly with QUIC, pinning that
certificate and presenting the token; nothing else is admitted. UDP brings two
things TCP cannot. A lost packet no longer stalls everything behind it, so a
lossy Wi-Fi link stays responsive. And when your laptop changes networks or
wakes from sleep, the client reconnects straight to the session over UDP, with
no SSH handshake first.

If the port range is blocked, or the host is reached through a jump host, Aft
quietly uses SSH as before. A firewall that already allows mosh allows Aft.
Turn UDP off with `udp = false` in the config, for every host or one.

## A terminal on a host

`aft HOST` opens your login shell on that host in a session, where HOST is the
name of one of your hosts: `aft server`, `aft mini`, `aft studio`. It is like
`ssh HOST`, but it survives disconnects and sleep, reaches the host over UDP
when it can, and predicts your typing locally, as mosh does. It starts in the
project's directory when this project is on for that host. Run it again in the
same place and it comes back to that shell. `aft HOST CMD...` runs one command
there instead and returns its exit code, as `ssh HOST CMD` does.

A host you have not added yet works too. `aft user@box` or `aft box`, when SSH
can reach it, adds it as a host named `box`, installs Aft's server there, and
opens the terminal; from then on `aft box` opens it. A host's name cannot be an
Aft command, so `aft HOST` always means the host: `aft host add` refuses a name
like `ls`.

When the shell or program ends, Aft says so, as ssh says a connection closed:
`• the shell on server exited; session owl is closed`, or the exit code or
signal if it failed. If the connection drops, a line across the top of the
screen says how long it has been and that Ctrl-C leaves; the session keeps
running either way.

## Sessions that end unseen

A session that ends while no terminal is attached does not just disappear.
It keeps its last screen as plain text in `~/.aft/logs/NAME.screen` on its
host, and `aft ls` lists it as `exited 0`, `exited 1` or `lost`, for one whose
server stopped. `aft attach NAME` prints that last screen, says how and when it
ended, and clears it; `aft kill NAME` clears it without showing it. They are
cleared after a week. The program's own state is untouched, so an agent that
keeps its conversations can usually pick up where it left off.

If a project's folder on the host is deleted, starting an agent there says so
and how to fix it, rather than failing on the directory.

Aft never stops a session on its own. `aft kill` does, and so does the
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
  you type meanwhile are sent after it, in order, and Ctrl-C cancels it. A
  file in a mounted folder (see [Mounts](mounts.md)) is already on the server,
  so its server path is pasted without copying it. Only files from where you
  would drop them, your home, a temporary directory or an external drive, are
  copied; a system path such as `/etc/hosts` is sent as typed, since it likely
  names the server's file. Ordinary text, and a path that does not exist on
  your machine, are sent unchanged. If a file cannot be sent, a line across
  the top of the screen says why.

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

Inside a session, `~/.local/share/aft/shims` on the host comes first on `PATH` and
provides `open`, `xdg-open`, `pbpaste`, `wl-paste`, `xclip` and `xsel`. When
the application opens an `http` or `https` URL, such as a login page, it opens
in your local browser. When it reads the clipboard right after you paste, for
example to paste an image, it gets your local clipboard. Anything else is
passed to the real command on the server.

Remote commands run with your full permissions on the server. An agent's
sandbox protects only the machine it runs on.
