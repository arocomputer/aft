package client

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/arocomputer/aft/internal/proto"
	"github.com/arocomputer/aft/internal/transport"
)

// Pasting a file. Terminals paste a file dragged or copied into them as its
// local path, which means nothing on the server. When a paste consists only
// of paths to files on this machine, the client uploads each file to the
// host's ~/.aft/drop over the existing connection and pastes the server paths
// instead, so an agent can read a pasted screenshot, recording or document.
// Keys typed while the upload runs are held and sent after the paste, in
// order; the input loop itself never waits on the network.

// uploadResult is a finished paste upload: the paste to send in place of the
// original, and a failure to show, if any (the original paste is sent then).
type uploadResult struct {
	paste []byte
	err   error
}

// startUpload begins uploading the files named by a paste token and returns
// true, or returns false when the paste is not a list of local files and
// should be sent as is. The result arrives on c.uploads.
func (c *session) startUpload(t Token) bool {
	if c.o.Host.IsLocal() || !bytes.HasSuffix(t.Bytes, pasteEnd) {
		return false
	}
	body := string(t.Bytes[:len(t.Bytes)-len(pasteEnd)])
	paths, ok := pastedFiles(body)
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
		remote, err := uploadFiles(host, paths)
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
	if u.err != nil {
		c.title("aft · paste upload failed: " + u.err.Error())
	} else {
		c.title(c.pred.Title())
	}
	c.send(proto.Input, u.paste)
	held := c.held
	c.held = nil
	return c.handleKeys(held)
}

// uploadFiles copies each local file to the host and returns the server
// paths, shell-escaped the way terminals escape a dropped file.
func uploadFiles(h transport.Host, paths []string) ([]string, error) {
	var out []string
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			return nil, err
		}
		dest, err := transport.Output(h, f, "drop", filepath.Base(p))
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Base(p), err)
		}
		out = append(out, shellEscape(strings.TrimSpace(string(dest))))
	}
	return out, nil
}

// pastedFiles reads a paste as the whitespace-separated paths terminals
// produce for files, honoring backslash escapes and quotes. It returns the
// paths only when every word is an absolute or ~ path to an existing regular
// file (or one this process may not read), so ordinary text that merely
// mentions a path, or a path that exists only on the server, is never touched.
func pastedFiles(body string) ([]string, bool) {
	words, ok := splitWords(body)
	if !ok || len(words) == 0 {
		return nil, false
	}
	var paths []string
	for _, w := range words {
		if strings.HasPrefix(w, "~/") {
			h, err := os.UserHomeDir()
			if err != nil {
				return nil, false
			}
			w = filepath.Join(h, w[2:])
		}
		if !filepath.IsAbs(w) {
			return nil, false
		}
		fi, err := os.Stat(w)
		switch {
		case os.IsPermission(err):
			// macOS privacy rules can hide a file from the terminal. Try the
			// upload anyway so the failure is reported instead of silently
			// pasting a path the server cannot open.
		case err != nil || !fi.Mode().IsRegular():
			return nil, false
		}
		paths = append(paths, w)
	}
	return paths, true
}

// splitWords splits s like a POSIX shell would split plain words: whitespace
// separates, a backslash escapes the next character, and single or double
// quotes group. ok is false for an unterminated quote.
func splitWords(s string) (words []string, ok bool) {
	var cur strings.Builder
	in := false // a word is open, possibly empty ('')
	var quote rune
	esc := false
	for _, r := range s {
		switch {
		case esc:
			cur.WriteRune(r)
			esc = false
		case quote != 0:
			if r == quote {
				quote = 0
			} else if r == '\\' && quote == '"' {
				esc = true
			} else {
				cur.WriteRune(r)
			}
		case r == '\\':
			esc, in = true, true
		case r == '\'' || r == '"':
			quote, in = r, true
		case unicode.IsSpace(r):
			if in {
				words = append(words, cur.String())
				cur.Reset()
				in = false
			}
		default:
			cur.WriteRune(r)
			in = true
		}
	}
	if quote != 0 || esc {
		return nil, false
	}
	if in {
		words = append(words, cur.String())
	}
	return words, true
}

// shellEscape backslash-escapes every character a shell treats specially,
// which is how terminals paste a dropped file's path.
func shellEscape(p string) string {
	var b strings.Builder
	for _, r := range p {
		if unicode.IsSpace(r) || strings.ContainsRune(`\'"$`+"`"+`!&|;<>()[]{}*?#~`, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
