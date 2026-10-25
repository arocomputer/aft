package vt

import (
	"bytes"
	"fmt"
	"strconv"
)

// CUP returns the sequence that moves the cursor to a zero-based row and column.
func CUP(row, col int) string {
	return "\x1b[" + strconv.Itoa(row+1) + ";" + strconv.Itoa(col+1) + "H"
}

// SGR returns a sequence that resets attributes and then applies a.
func SGR(a Attr) string {
	var b bytes.Buffer
	b.WriteString("\x1b[0")
	if a.Bold {
		b.WriteString(";1")
	}
	if a.Dim {
		b.WriteString(";2")
	}
	if a.Italic {
		b.WriteString(";3")
	}
	switch a.Underline {
	case 1:
		b.WriteString(";4")
	case 0:
	default:
		b.WriteString(";4:" + strconv.Itoa(int(a.Underline)))
	}
	if a.Blink {
		b.WriteString(";5")
	}
	if a.Reverse {
		b.WriteString(";7")
	}
	if a.Hidden {
		b.WriteString(";8")
	}
	if a.Strike {
		b.WriteString(";9")
	}
	writeColor(&b, a.FG, 38)
	writeColor(&b, a.BG, 48)
	b.WriteString("m")
	return b.String()
}

func writeColor(b *bytes.Buffer, c Color, base int) {
	switch c.Kind {
	case ColorIndexed:
		if c.Index < 8 {
			fmt.Fprintf(b, ";%d", base-8+int(c.Index))
		} else if c.Index < 16 {
			fmt.Fprintf(b, ";%d", base+44+int(c.Index)) // 90..97 or 100..107
		} else {
			fmt.Fprintf(b, ";%d;5;%d", base, c.Index)
		}
	case ColorRGB:
		fmt.Fprintf(b, ";%d;2;%d;%d;%d", base, c.R, c.G, c.B)
	}
}

// RenderOptions controls what Render includes.
type RenderOptions struct {
	// Scrollback replays lines that scrolled off the main screen before the
	// screen itself, so they land in the real terminal's history. It applies
	// only when the main screen is active.
	Scrollback bool
	// Title emits the window title.
	Title bool
}

// Render returns escape sequences that reproduce the current screen, cursor,
// modes and character sets on a terminal of the same size. Render starts by
// resetting the attributes, modes and character sets it does not set, so it
// may be sent to a terminal an earlier Render left in any state.
func (t *Terminal) Render(o RenderOptions) []byte {
	var b bytes.Buffer
	// Hide the cursor, then reset attributes, modes and character sets the
	// screen below does not set itself; the tail restores the real ones.
	b.WriteString("\x1b[?25l\x1b[0m\x1b[?7h\x1b[?6l\x1b[4l\x1b[r\x0f\x1b(B\x1b)B")
	if t.modes.AltScreen {
		b.WriteString("\x1b[?1049h\x1b[H\x1b[2J")
		t.paintRows(&b, true)
	} else {
		b.WriteString("\x1b[?1049l\x1b[H\x1b[2J")
		if o.Scrollback && len(t.scrollback) > 0 {
			// Printing scrollback and then the screen as one stream scrolls the
			// older lines into the real terminal's history.
			for _, line := range t.scrollback {
				t.paintLine(&b, line)
				b.WriteString("\r\n")
			}
			for r := 0; r < t.rows; r++ {
				t.paintLine(&b, t.Row(r))
				if r < t.rows-1 {
					b.WriteString("\r\n")
				}
			}
		} else {
			t.paintRows(&b, true)
		}
	}
	b.WriteString("\x1b[0m")
	if t.top != 0 || t.bot != t.rows-1 {
		fmt.Fprintf(&b, "\x1b[%d;%dr", t.top+1, t.bot+1)
	}
	m := t.modes
	if m.Origin {
		b.WriteString("\x1b[?6h")
	}
	if !m.AutoWrap {
		b.WriteString("\x1b[?7l")
	}
	if m.Insert {
		b.WriteString("\x1b[4h")
	}
	if m.AppCursorKeys {
		b.WriteString("\x1b[?1h")
	}
	if m.AppKeypad {
		b.WriteString("\x1b=")
	}
	if m.ReverseVideo {
		b.WriteString("\x1b[?5h")
	}
	if m.Mouse != MouseOff {
		fmt.Fprintf(&b, "\x1b[?%dh", int(m.Mouse))
	}
	if m.MouseEnc != MouseEncDefault {
		fmt.Fprintf(&b, "\x1b[?%dh", int(m.MouseEnc))
	}
	if m.FocusEvents {
		b.WriteString("\x1b[?1004h")
	}
	if m.BracketedPaste {
		b.WriteString("\x1b[?2004h")
	}
	if m.KittyFlags != 0 {
		fmt.Fprintf(&b, "\x1b[=%d;1u", m.KittyFlags)
	}
	if m.CursorStyle != 0 {
		fmt.Fprintf(&b, "\x1b[%d q", m.CursorStyle)
	}
	// Character sets, so an ncurses application mid-way through drawing a
	// box keeps drawing lines rather than letters.
	_, restore := t.CharsetSequences()
	b.WriteString(restore)
	if o.Title && t.title != "" {
		b.WriteString("\x1b]2;" + t.title + "\x1b\\")
	}
	// Cursor position is absolute, so origin mode is applied after moving.
	row := t.cur.Row
	if m.Origin {
		row -= t.top
	}
	b.WriteString(CUP(row, t.cur.Col))
	b.WriteString(SGR(t.cur.Attr))
	if t.cur.Attr.Link != 0 {
		// The application is inside a link: what it prints next is part of it.
		b.WriteString("\x1b]8;;" + t.LinkURL(t.cur.Attr.Link) + "\x1b\\")
	}
	if m.CursorVisible {
		b.WriteString("\x1b[?25h")
	}
	return b.Bytes()
}

// RenderRow returns the sequences that redraw one row of the screen in place
// on a terminal in the application's current state, leaving the pen reset
// and the application's character sets as they were.
func (t *Terminal) RenderRow(r int) []byte {
	var b bytes.Buffer
	plain, restore := t.CharsetSequences()
	b.WriteString(plain)
	b.WriteString(CUP(r, 0))
	t.paintLine(&b, t.Row(r))
	b.WriteString("\x1b[0m" + restore)
	return b.Bytes()
}

// gset names a G0/G1 designation: DEC special graphics or ASCII.
func gset(graphics bool) string {
	if graphics {
		return "0"
	}
	return "B"
}

func (t *Terminal) paintRows(b *bytes.Buffer, position bool) {
	for r := 0; r < t.rows; r++ {
		if position {
			b.WriteString(CUP(r, 0))
		}
		t.paintLine(b, t.Row(r))
	}
}

// paintLine writes one row's cells with the fewest attribute changes, erasing
// the trailing blank run instead of printing it.
func (t *Terminal) paintLine(b *bytes.Buffer, cells []Cell) {
	end := len(cells)
	for end > 0 {
		c := cells[end-1]
		if c.Width == 1 && (c.Rune == ' ' || c.Rune == 0) && c.Attr == (Attr{}) {
			end--
			continue
		}
		break
	}
	var cur Attr
	first := true
	var link uint32
	for i := 0; i < end; i++ {
		c := cells[i]
		if c.Width == 0 {
			continue
		}
		if c.Attr.Link != link {
			link = c.Attr.Link
			b.WriteString("\x1b]8;;" + t.LinkURL(link) + "\x1b\\")
		}
		if first || c.Attr != cur {
			b.WriteString(SGR(c.Attr))
			cur = c.Attr
			first = false
		}
		if c.Rune == 0 {
			b.WriteByte(' ')
		} else {
			b.WriteRune(c.Rune)
		}
	}
	if link != 0 {
		b.WriteString("\x1b]8;;\x1b\\")
	}
	if end < len(cells) {
		// Erase the rest in default colors: the pen may still carry the last
		// painted cell's attributes, from this row or the one above, and an
		// erase fills with the current background.
		b.WriteString("\x1b[0m\x1b[K")
	}
}
