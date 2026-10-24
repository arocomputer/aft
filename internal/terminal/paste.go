package terminal

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/arocomputer/aft/internal/bridge"
	"github.com/arocomputer/aft/internal/bridge/proto"
)

// Pasting, the terminal's side of the bridge clipboard. Two things happen
// here: a paste of local files is uploaded and pasted as server paths, and
// each paste or paste key opens a short window in which the server may read
// the local clipboard, which is how an agent pastes an image on Ctrl-V.

// pasteWindow is how long after a paste the server may read the clipboard.
const pasteWindow = 5 * time.Second

// uploadResult is a finished paste upload: the paste to send in place of the
// original, and a failure to show, if any (the original paste is sent then).
type uploadResult struct {
	paste []byte
	err   error
}

// notePaste records a paste or paste key, opening the clipboard window.
func (c *session) notePaste(t Token) {
	if t.Kind == TokPaste || isPasteKey(t.Bytes) {
		c.pastedAt = time.Now()
	}
}

// clipboardOK reports whether the server may read the clipboard now.
func (c *session) clipboardOK() bool {
	return !c.pastedAt.IsZero() && time.Since(c.pastedAt) < pasteWindow
}

// isPasteKey reports whether b is Ctrl-V or Cmd-V, the keys agents bind to
// paste an image, in the legacy encoding or the kitty keyboard protocol
// (CSI 118 ; modifiers u, where modifiers-1 has the ctrl or super bit).
func isPasteKey(b []byte) bool {
	if len(b) == 1 && b[0] == 0x16 {
		return true
	}
	s := string(b)
	if !strings.HasPrefix(s, "\x1b[118;") || !strings.HasSuffix(s, "u") {
		return false
	}
	mods := strings.TrimSuffix(strings.TrimPrefix(s, "\x1b[118;"), "u")
	if i := strings.IndexByte(mods, ':'); i >= 0 {
		mods = mods[:i]
	}
	m, err := strconv.Atoi(mods)
	return err == nil && m > 0 && (m-1)&(4|8) != 0
}

// startUpload begins uploading the files named by a paste token and returns
// true, or returns false when the paste is not a list of local files and
// should be sent as is. The result arrives on c.uploads; keys typed meanwhile
// are held, so the input loop never waits on the network.
func (c *session) startUpload(t Token) bool {
	if c.o.Host.IsLocal() || !bytes.HasSuffix(t.Bytes, pasteEnd) {
		return false
	}
	body := string(t.Bytes[:len(t.Bytes)-len(pasteEnd)])
	paths, ok := bridge.PastedFiles(body)
	if !ok {
		return false
	}
	c.holding = true
	if len(paths) == 1 {
		c.title("aft · uploading " + filepath.Base(paths[0]))
	} else {
		c.title(fmt.Sprintf("aft · uploading %d files", len(paths)))
	}
	host := c.o.Host
	go func() {
		remote, err := bridge.Upload(host, paths)
		if err != nil {
			c.uploads <- uploadResult{paste: t.Bytes, err: err}
			return
		}
		trimmed := strings.TrimSpace(body)
		i := strings.Index(body, trimmed)
		paste := body[:i] + strings.Join(remote, " ") + body[i+len(trimmed):]
		c.uploads <- uploadResult{paste: append([]byte(paste), pasteEnd...)}
	}()
	return true
}

// finishUpload sends the paste an upload produced and then the keys held
// while it ran. It returns false when a held key asks the client to leave.
func (c *session) finishUpload(u uploadResult) bool {
	c.holding = false
	c.title(c.pred.Title())
	if u.err != nil {
		// The original paste goes through; say why it is a local path.
		msg := "aft · could not send the pasted file to " + c.o.Host.Name + ": " + u.err.Error()
		if errors.Is(u.err, fs.ErrPermission) {
			msg = "aft · macOS would not let this terminal read the pasted file · give the terminal app Full Disk Access, or paste the saved file"
		}
		c.flash(msg)
	}
	c.pastedAt = time.Now() // the agent may read the pasted file's clipboard now
	c.send(proto.Input, u.paste)
	held := c.held
	c.held = nil
	return c.handleKeys(held)
}
