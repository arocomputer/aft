# Client and session roadmap

Where the feature-rich terminal client and session go next, and why.
This is a design note, not a promise. It starts from what a power user expects
from a remote terminal and names what aft has, what it just gained, and what is
worth building, in priority order.

## The division of labor

Two processes do the work, and it matters which one owns what.

- **The client, on the laptop, owns everything that touches the local machine:**
  the real terminal, keystroke prediction, the clipboard, port forwards to
  localhost, opening URLs, and reconnection. It passes the server's output to
  the terminal byte for byte, so the terminal's own abilities, color, mouse,
  selection, hyperlinks, images, keep working.
- **The session, on the server, owns the application:** a pseudo-terminal
  running the agent, the screen while nobody is attached, the answers to
  terminal queries, and a scrollback buffer it can replay on reattach.

So copy and paste belong to the client and the local terminal, not the session.
The session only relays what the application emits. This is the opposite of
SSH-plus-tmux, where the remote tmux owns selection and cannot reach the local
clipboard without OSC 52.

## Done

### Copy and paste

- **Selection and Cmd+C** work through passthrough: the terminal has the text
  because aft drew it there. For a mouse-capturing full-screen agent the user
  holds Option (or Ctrl on Linux) to select, which is the terminal's standard
  override, nothing aft can or should change.
- **Cmd+V** works: the terminal sends the text and the client forwards it,
  preserving bracketed paste.
- **OSC 52 copy from the server now sets the laptop clipboard** even when the
  terminal blocks OSC 52, because the client decodes it and writes the local
  clipboard itself. Clipboard reads from the server stay disabled by default.

### Copy mode (scrollback)

The session holds the session's scrollback and sends a snapshot when the client
asks. Copy mode, opened with the prefix then `[`, renders that snapshot as a
frozen pager in the client, with keyboard scroll, search and line selection that
copies to the local clipboard, and native drag-select because it does not
capture the mouse. The prefix (Ctrl-] by default, configurable, disableable)
also detaches with `d`.

## Missing, in priority order

### 1. Explicit port forwards (high, small)

Today aft mirrors the server's listeners in a range to localhost automatically.
Power users also want to choose forwards: a fixed local port to a remote
service (a database on 5432), a remote port back to a local service (`-R`), and
a SOCKS proxy (`-D`). The transport already opens and cancels forwards on the
shared connection; expose `aft forward [-R|-D] LOCAL[:HOST:REMOTE]` and list and
cancel them. Small, because the plumbing exists.

### 2. File transfer (medium)

Getting a file to or from the server without leaving aft. Two shapes: `aft cp`
over the existing connection, and, richer, dragging a file into the terminal
uploads it and pastes the server path, and an OSC 52-style or helper path for
the reverse. The clipboard-image bridge already proves the transport can carry
bytes both ways. Start with `aft cp`.

### 3. Activity and completion signals (medium, small)

A long agent run finishes while the user is in another window. The session sees
the bell (OSC 7 already passes through) and can see the pane go idle. A desktop
notification on bell, and optionally when a detached session's agent exits or
goes quiet, through the existing URL/clipboard bridge pattern, is a small, loved
feature. Ties into the next item for detached sessions.

### 4. Detached-session awareness (medium)

`aft ls` lists sessions, but a user with several agents running wants to know
which need attention: which are waiting for input, which finished, which hit an
error. The session can watch for the bell and for the screen going idle and
record a per-session status that `aft ls` and `aft status` show, and that a
notification can fire on. No new transport, just state in the session.

### 5. SSH power features the user already has, surfaced (low, mostly docs)

Because aft drives the user's own OpenSSH, agent forwarding (`-A`), jump hosts
(`ProxyJump`), and connection multiplexing already work through their SSH
config, and a host's `ssh` field can carry options like `-J jump box` or
`-p 2222`. This mostly needs documentation, plus a `doctor` note when a forward
or agent socket is expected but missing. Agent forwarding carries a real risk on
an untrusted server and should be documented, not encouraged.

### 6. A UDP transport for lossy links (low, large)

The client predicts typing, so keystrokes feel local even on a bad link, but
output still waits on TCP, which stalls on loss where mosh's UDP does not. A
datagram transport that syncs screen state, the mosh approach, would close that
gap. Large, and only worth it if real use on cellular or bad Wi-Fi proves TCP
stalls matter. Measure before building.

### Explicitly not planned

- **Panes and windows.** aft runs one agent per session; splitting is the
  terminal's job or the agent's. A multiplexer UI is scope aft does not want.
- **Clipboard read from the server by default.** A remote application silently
  reading the laptop clipboard is a leak; it stays opt-in.
- **Its own scrollback for ordinary output.** The terminal already does this
  well for the normal screen; copy mode covers the rest.

## Sources

- [Eternal Terminal discussion: scrollback, OSC 52, control mode](https://news.ycombinator.com/item?id=22811119)
- [Fixing mosh scrollback](https://getmoshi.app/articles/fix-mosh-scrollback)
- [mosh vs SSH](https://getmoshi.app/articles/mosh-vs-ssh)
- [Advanced SSH techniques: forwarding, jump hosts, multiplexing](https://deadman.org/beyond-basics-advanced-ssh-techniques-for-power-users-and-administrators/)
- [Terminal multiplexers: tmux vs Zellij](https://dasroot.net/posts/2026/02/terminal-multiplexers-tmux-vs-zellij-comparison/)
- [iTerm2 Option-drag to bypass mouse reporting](https://upd.dev/warpdotdev/warp/issues/3280)
