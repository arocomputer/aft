package client

import (
	"bytes"
	"strings"

	"github.com/arocomputer/aft/internal/vt"
)

// copyMode is aft's scrollback viewer: a frozen pager over a snapshot of the
// berth's scrollback and screen, navigated and searched with the keyboard and
// copied to the local clipboard. It runs entirely on the client, so scrolling
// and searching are instant, and because it never captures the mouse the
// terminal's own drag-select and Cmd+C still work on the drawn text.
type copyMode struct {
	lines      [][]vt.Cell
	cols, rows int // terminal size; the last row is the status bar
	top        int // index of the first visible line
	cursor     int // the active line, kept on screen; anchors search and selection

	selecting bool
	selStart  int

	searching bool // typing a query into the status bar
	query     []rune
	lastQuery string
	lastDir   int // +1 older (up), -1 newer (down)
}

// newCopyMode builds a pager from a snapshot sized to the terminal, starting at
// the bottom so the most recent output is in view.
func newCopyMode(snap vt.SnapshotData, cols, rows int) *copyMode {
	c := &copyMode{lines: snap.Lines, cols: cols, rows: rows}
	c.cursor = len(c.lines) - 1
	if c.cursor < 0 {
		c.cursor = 0
	}
	c.scrollToCursor()
	return c
}

func (c *copyMode) height() int {
	if c.rows < 2 {
		return 1
	}
	return c.rows - 1 // reserve the last row for the status bar
}

func (c *copyMode) resize(cols, rows int) {
	c.cols, c.rows = cols, rows
	c.scrollToCursor()
}

// clamp keeps top within range and the cursor on a real line.
func (c *copyMode) clamp() {
	if c.cursor < 0 {
		c.cursor = 0
	}
	if c.cursor > len(c.lines)-1 {
		c.cursor = len(c.lines) - 1
	}
	maxTop := len(c.lines) - c.height()
	if maxTop < 0 {
		maxTop = 0
	}
	if c.top > maxTop {
		c.top = maxTop
	}
	if c.top < 0 {
		c.top = 0
	}
}

// scrollToCursor moves the viewport the least amount to keep the cursor visible.
func (c *copyMode) scrollToCursor() {
	h := c.height()
	if c.cursor < c.top {
		c.top = c.cursor
	}
	if c.cursor >= c.top+h {
		c.top = c.cursor - h + 1
	}
	c.clamp()
}

// move changes the active line by delta and follows it with the viewport.
func (c *copyMode) move(delta int) {
	c.cursor += delta
	c.clamp()
	c.scrollToCursor()
}

// key applies one input token. It returns exit=true when copy mode should end,
// and copied bytes when a selection was yanked (the caller sets the clipboard).
func (c *copyMode) key(t Token) (exit bool, copied []byte) {
	if c.searching {
		return c.searchKey(t), nil
	}
	s := string(t.Bytes)
	switch {
	case s == "q" || s == "\x1b": // q or Escape
		return true, nil
	case s == "j" || s == "\x1b[B" || s == "\x1bOB":
		c.move(1)
	case s == "k" || s == "\x1b[A" || s == "\x1bOA":
		c.move(-1)
	case s == " " || s == "\x1b[6~" || t.Bytes[0] == 0x06: // space, PgDn, Ctrl-F
		c.move(c.height())
	case s == "b" || s == "\x1b[5~" || t.Bytes[0] == 0x02: // b, PgUp, Ctrl-B
		c.move(-c.height())
	case t.Bytes[0] == 0x04: // Ctrl-D half page down
		c.move(c.height() / 2)
	case t.Bytes[0] == 0x15: // Ctrl-U half page up
		c.move(-c.height() / 2)
	case s == "g" || s == "\x1b[H":
		c.cursor = 0
		c.scrollToCursor()
	case s == "G" || s == "\x1b[F":
		c.cursor = len(c.lines) - 1
		c.scrollToCursor()
	case s == "/":
		c.searching = true
		c.query = c.query[:0]
	case s == "n":
		c.repeatSearch(c.lastDir)
	case s == "N":
		c.repeatSearch(-c.lastDir)
	case s == "v" || s == "V":
		c.selecting = !c.selecting
		c.selStart = c.cursor
	case s == "y" || s == "\r": // yank selection (or the current line) and exit
		return true, c.selection()
	}
	return false, nil
}

// searchKey edits and runs the search query shown in the status bar.
func (c *copyMode) searchKey(t Token) (exit bool) {
	switch {
	case len(t.Bytes) == 1 && t.Bytes[0] == '\x1b': // cancel the search
		c.searching = false
	case t.Kind == TokBackspace:
		if len(c.query) > 0 {
			c.query = c.query[:len(c.query)-1]
		}
	case len(t.Bytes) == 1 && (t.Bytes[0] == '\r' || t.Bytes[0] == '\n'):
		c.searching = false
		c.lastQuery = string(c.query)
		c.repeatSearch(-1) // search toward older output, which is a lower index
	case t.Kind == TokText:
		c.query = append(c.query, t.Rune)
	}
	return false
}

// repeatSearch jumps to the next line containing the query in the given
// direction (+1 older, -1 newer), case-insensitively, and remembers it for n/N.
func (c *copyMode) repeatSearch(dir int) {
	if c.lastQuery == "" || dir == 0 {
		return
	}
	c.lastDir = dir
	needle := strings.ToLower(c.lastQuery)
	for i := c.cursor + dir; i >= 0 && i < len(c.lines); i += dir {
		if strings.Contains(strings.ToLower(vt.LineText(c.lines[i])), needle) {
			c.cursor = i
			c.scrollToCursor()
			return
		}
	}
}

// selection returns the selected lines as text, or the current line when not
// selecting, with trailing blank lines dropped.
func (c *copyMode) selection() []byte {
	lo, hi := c.cursor, c.cursor
	if c.selecting {
		lo, hi = c.selStart, c.cursor
		if lo > hi {
			lo, hi = hi, lo
		}
	}
	var out []string
	for i := lo; i <= hi && i < len(c.lines); i++ {
		out = append(out, vt.LineText(c.lines[i]))
	}
	for len(out) > 1 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return []byte(strings.Join(out, "\n"))
}

// view renders the visible lines and the status bar as one synchronized frame.
func (c *copyMode) view() []byte {
	c.clamp()
	var b bytes.Buffer
	b.WriteString("\x1b[?2026h\x1b[?25l") // begin synchronized update, hide cursor
	h := c.height()
	selLo, selHi := c.selStart, c.cursor
	if selLo > selHi {
		selLo, selHi = selHi, selLo
	}
	for row := 0; row < h; row++ {
		idx := c.top + row
		b.WriteString(vt.CUP(row, 0))
		b.WriteString("\x1b[0m\x1b[K")
		if idx >= len(c.lines) {
			b.WriteString("\x1b[38;5;240m~\x1b[0m")
			continue
		}
		selected := c.selecting && idx >= selLo && idx <= selHi
		if selected {
			b.WriteString("\x1b[7m")
		}
		writeCells(&b, c.lines[idx], c.cols, selected)
		if selected {
			b.WriteString("\x1b[0m")
		}
	}
	b.WriteString(vt.CUP(c.rows-1, 0))
	b.WriteString("\x1b[0m\x1b[7m")
	b.WriteString(padRight(c.status(), c.cols))
	b.WriteString("\x1b[0m\x1b[?2026l")
	return b.Bytes()
}

// status is the text of the bottom bar: the mode, position and any search.
func (c *copyMode) status() string {
	if c.searching {
		return " search: " + string(c.query)
	}
	pos := ""
	if len(c.lines) > 0 {
		pos = itoa(c.cursor+1) + "/" + itoa(len(c.lines))
	}
	mode := "COPY"
	if c.selecting {
		mode = "SELECT"
	}
	hint := "  ↑↓ scroll · / search · v select · y copy · q quit"
	left := " " + mode + " " + pos
	return left + hint
}

// writeCells draws one row with each cell's own style, padded to width. In a
// selected row the reverse attribute is already set, so cells drop their own
// styling to keep the highlight uniform.
func writeCells(b *bytes.Buffer, cells []vt.Cell, width int, plain bool) {
	n := 0
	var cur vt.Attr
	first := true
	for _, cell := range cells {
		if cell.Width == 0 {
			continue
		}
		if n >= width {
			break
		}
		if !plain && (first || cell.Attr != cur) {
			b.WriteString(vt.SGR(cell.Attr))
			cur = cell.Attr
			first = false
		}
		if cell.Rune == 0 {
			b.WriteByte(' ')
		} else {
			b.WriteRune(cell.Rune)
		}
		n += int(cell.Width)
	}
}

func padRight(s string, width int) string {
	if len([]rune(s)) >= width {
		r := []rune(s)
		if width <= 0 {
			return ""
		}
		return string(r[:width])
	}
	return s + strings.Repeat(" ", width-len([]rune(s)))
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	if neg {
		d = append([]byte{'-'}, d...)
	}
	return string(d)
}
