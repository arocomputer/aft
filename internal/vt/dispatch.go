package vt

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

// control handles a C0 control character in the ground state.
func (t *Terminal) control(c byte) {
	t.repeat = 0
	switch c {
	case 0x07:
		t.Bells++
	case 0x0E:
		t.charset.shiftOut = true
	case 0x0F:
		t.charset.shiftOut = false
	case 0x08:
		if t.cur.Col > 0 {
			t.cur.Col--
		}
		t.cur.Wrap = false
	case 0x09:
		t.tab()
	case 0x0A, 0x0B, 0x0C:
		t.index()
		t.cur.Wrap = false
	case 0x0D:
		t.cur.Col = 0
		t.cur.Wrap = false
	}
}

// escDispatch handles ESC sequences that are not CSI, OSC or DCS.
func (t *Terminal) escDispatch(inter []byte, final byte) {
	t.repeat = 0
	if len(inter) > 0 {
		switch {
		case inter[0] == '#' && final == '8': // DECALN
			t.gen++
			for r := 0; r < t.rows; r++ {
				for c := 0; c < t.cols; c++ {
					t.set(r, c, Cell{Rune: 'E', Width: 1})
				}
			}
		case inter[0] == '(': // designate G0: 0 is DEC special graphics, anything else ASCII
			t.charset.g0Graphics = final == '0'
		case inter[0] == ')':
			t.charset.g1Graphics = final == '0'
		}
		return
	}
	switch final {
	case '7':
		t.saveCursor()
	case '8':
		t.restoreCursor()
	case 'D':
		t.index()
	case 'E':
		t.cur.Col = 0
		t.index()
	case 'H':
		if t.cur.Col < len(t.tabs) {
			t.tabs[t.cur.Col] = true
		}
	case 'M':
		t.reverseIndex()
	case 'c':
		t.reset()
	case '=':
		t.modes.AppKeypad = true
	case '>':
		t.modes.AppKeypad = false
	}
}

func param(params [][]int, i, def int) int {
	if i >= len(params) || len(params[i]) == 0 || params[i][0] == 0 {
		return def
	}
	return params[i][0]
}

// csiDispatch handles a complete CSI sequence.
func (t *Terminal) csiDispatch(private byte, params [][]int, inter []byte, final byte) {
	if private == '?' {
		t.csiPrivate(params, inter, final)
		return
	}
	if private == '>' || private == '<' || private == '=' {
		t.csiKitty(private, params, inter, final)
		return
	}
	if len(inter) > 0 {
		switch {
		case inter[0] == ' ' && final == 'q': // DECSCUSR
			t.modes.CursorStyle = param(params, 0, 0)
		case inter[0] == '$' && final == 'p': // DECRQM, ANSI modes
			t.reply(fmt.Sprintf("\x1b[%d;%d$y", param(params, 0, 0), t.ansiModeState(param(params, 0, 0))))
		}
		return
	}
	n := param(params, 0, 1)
	if final != 'b' {
		t.repeat = 0
	}
	switch final {
	case '@':
		t.gen++
		t.insertCells(t.cur.Row, t.cur.Col, n)
	case 'A':
		t.cur.Row = t.up(n)
		t.cur.Wrap = false
	case 'B':
		t.cur.Row = t.down(n)
		t.cur.Wrap = false
	case 'C', 'a':
		t.cur.Col = min(t.cols-1, t.cur.Col+n)
		t.cur.Wrap = false
	case 'D':
		t.cur.Col = max(0, t.cur.Col-n)
		t.cur.Wrap = false
	case 'E':
		t.cur.Row = t.down(n)
		t.cur.Col = 0
		t.cur.Wrap = false
	case 'F':
		t.cur.Row = t.up(n)
		t.cur.Col = 0
		t.cur.Wrap = false
	case 'G', '`':
		t.cur.Col = min(t.cols-1, n-1)
		t.cur.Wrap = false
	case 'H', 'f':
		t.moveTo(param(params, 0, 1)-1, param(params, 1, 1)-1)
	case 'I':
		for ; n > 0; n-- {
			t.tab()
		}
	case 'J':
		t.eraseDisplay(param(params, 0, 0))
	case 'K':
		t.eraseLine(param(params, 0, 0))
	case 'L':
		t.insertLines(n)
	case 'M':
		t.deleteLines(n)
	case 'P':
		t.gen++
		t.deleteCells(t.cur.Row, t.cur.Col, n)
	case 'S':
		t.scrollUp(n)
	case 'T':
		t.scrollDown(n)
	case 'X':
		t.eraseRegion(t.cur.Row, t.cur.Col, t.cur.Row, min(t.cols-1, t.cur.Col+n-1))
	case 'Z':
		t.backTab(n)
	case 'b': // REP: ncurses uses it for runs of one character
		if t.repeat != 0 {
			r := t.repeat
			for ; n > 0; n-- {
				t.print(r)
			}
		}
	case 'c':
		t.reply("\x1b[?62;1;2;4;6;9;15;22c")
	case 'd':
		t.moveTo(n-1, t.cur.Col) // within the region under origin mode
	case 'g':
		if param(params, 0, 0) == 3 {
			for i := range t.tabs {
				t.tabs[i] = false
			}
		} else if t.cur.Col < len(t.tabs) {
			t.tabs[t.cur.Col] = false
		}
	case 'h', 'l':
		on := final == 'h'
		for i := range params {
			switch params[i][0] {
			case 4:
				t.modes.Insert = on
			}
		}
	case 'm':
		t.sgr(params)
	case 'n':
		switch param(params, 0, 0) {
		case 5:
			t.reply("\x1b[0n")
		case 6:
			t.reply(fmt.Sprintf("\x1b[%d;%dR", t.cur.Row+1, t.cur.Col+1))
		}
	case 'r':
		top := param(params, 0, 1) - 1
		bot := param(params, 1, t.rows) - 1
		if top < 0 {
			top = 0
		}
		if bot >= t.rows {
			bot = t.rows - 1
		}
		if top < bot {
			t.top, t.bot = top, bot
			t.moveTo(0, 0)
		}
	case 's':
		t.saveCursor()
	case 'u':
		t.restoreCursor()
	case 't':
		// Window manipulation: answer size queries so apps do not hang.
		switch param(params, 0, 0) {
		case 18:
			t.reply(fmt.Sprintf("\x1b[8;%d;%dt", t.rows, t.cols))
		case 14:
			t.reply(fmt.Sprintf("\x1b[4;%d;%dt", t.rows*16, t.cols*8))
		case 16:
			t.reply("\x1b[6;16;8t")
		}
	}
}

// up and down move the cursor by n rows, stopping at the scroll region's edge
// when the cursor is inside the region and at the screen's edge when it is
// outside, as a VT does.
func (t *Terminal) up(n int) int {
	if t.cur.Row >= t.top {
		return max(t.top, t.cur.Row-n)
	}
	return max(0, t.cur.Row-n)
}

func (t *Terminal) down(n int) int {
	if t.cur.Row <= t.bot {
		return min(t.bot, t.cur.Row+n)
	}
	return min(t.rows-1, t.cur.Row+n)
}

func (t *Terminal) ansiModeState(mode int) int {
	switch mode {
	case 4:
		if t.modes.Insert {
			return 1
		}
		return 2
	}
	return 0
}

// csiPrivate handles CSI ? sequences: DEC private modes and their queries.
func (t *Terminal) csiPrivate(params [][]int, inter []byte, final byte) {
	if len(inter) > 0 && inter[0] == '$' && final == 'p' {
		m := param(params, 0, 0)
		t.reply(fmt.Sprintf("\x1b[?%d;%d$y", m, t.decModeState(m)))
		return
	}
	switch final {
	case 'h', 'l':
		on := final == 'h'
		for i := range params {
			t.setDECMode(params[i][0], on)
		}
	case 'n':
		if param(params, 0, 0) == 6 {
			t.reply(fmt.Sprintf("\x1b[?%d;%dR", t.cur.Row+1, t.cur.Col+1))
		}
	case 'u':
		// Kitty keyboard protocol query.
		t.reply(fmt.Sprintf("\x1b[?%du", t.modes.KittyFlags))
	case 'J':
		t.eraseDisplay(param(params, 0, 0)) // DECSED approximated as ED
	case 'K':
		t.eraseLine(param(params, 0, 0))
	}
}

func (t *Terminal) setDECMode(mode int, on bool) {
	switch mode {
	case 1:
		t.modes.AppCursorKeys = on
	case 5:
		t.modes.ReverseVideo = on
	case 6:
		t.modes.Origin = on
		t.moveTo(0, 0)
	case 7:
		t.modes.AutoWrap = on
	case 12:
		// cursor blink; ignored
	case 25:
		t.modes.CursorVisible = on
	case 47, 1047:
		t.setAltScreen(on, false)
	case 1048:
		if on {
			t.saveCursor()
		} else {
			t.restoreCursor()
		}
	case 1049:
		t.setAltScreen(on, true)
	case 9, 1000, 1002, 1003:
		if on {
			t.modes.Mouse = MouseMode(mode)
		} else if t.modes.Mouse == MouseMode(mode) {
			t.modes.Mouse = MouseOff
		}
	case 1005, 1006, 1015, 1016:
		if on {
			t.modes.MouseEnc = MouseEncoding(mode)
		} else if t.modes.MouseEnc == MouseEncoding(mode) {
			t.modes.MouseEnc = MouseEncDefault
		}
	case 1004:
		t.modes.FocusEvents = on
	case 2004:
		t.modes.BracketedPaste = on
	case 2026:
		t.modes.SyncOutput = on
	}
}

func (t *Terminal) decModeState(mode int) int {
	b := func(v bool) int {
		if v {
			return 1
		}
		return 2
	}
	switch mode {
	case 1:
		return b(t.modes.AppCursorKeys)
	case 5:
		return b(t.modes.ReverseVideo)
	case 6:
		return b(t.modes.Origin)
	case 7:
		return b(t.modes.AutoWrap)
	case 25:
		return b(t.modes.CursorVisible)
	case 47, 1047, 1049:
		return b(t.modes.AltScreen)
	case 9, 1000, 1002, 1003:
		return b(t.modes.Mouse == MouseMode(mode))
	case 1005, 1006, 1015, 1016:
		return b(t.modes.MouseEnc == MouseEncoding(mode))
	case 1004:
		return b(t.modes.FocusEvents)
	case 2004:
		return b(t.modes.BracketedPaste)
	case 2026:
		return b(t.modes.SyncOutput)
	}
	return 0
}

// csiKitty handles the kitty keyboard protocol stack (CSI > / < / = ... u) and
// the secondary device attributes and XTVERSION queries that share prefixes.
func (t *Terminal) csiKitty(private byte, params [][]int, inter []byte, final byte) {
	switch {
	case private == '>' && final == 'u':
		t.modes.kittyStack = append(t.modes.kittyStack, t.modes.KittyFlags)
		if len(t.modes.kittyStack) > 16 {
			t.modes.kittyStack = t.modes.kittyStack[1:]
		}
		t.modes.KittyFlags = param(params, 0, 0)
	case private == '<' && final == 'u':
		n := param(params, 0, 1)
		for ; n > 0; n-- {
			if len(t.modes.kittyStack) == 0 {
				t.modes.KittyFlags = 0
				break
			}
			t.modes.KittyFlags = t.modes.kittyStack[len(t.modes.kittyStack)-1]
			t.modes.kittyStack = t.modes.kittyStack[:len(t.modes.kittyStack)-1]
		}
	case private == '=' && final == 'u':
		flags, mode := param(params, 0, 0), param(params, 1, 1)
		switch mode {
		case 1:
			t.modes.KittyFlags = flags
		case 2:
			t.modes.KittyFlags |= flags
		case 3:
			t.modes.KittyFlags &^= flags
		}
	case private == '>' && final == 'c':
		t.reply("\x1b[>41;380;0c")
	case private == '>' && final == 'q':
		t.reply("\x1bP>|aft 2.0\x1b\\")
	case private == '=' && final == 'c':
		t.reply("\x1bP!|00000000\x1b\\")
	case private == '>' && (final == 'm' || final == 'n' || final == 'p'):
		// XTMODKEYS and friends: accepted silently
	}
}

// sgr applies character attributes.
func (t *Terminal) sgr(params [][]int) {
	a := &t.cur.Attr
	// A hyperlink is not a character attribute: SGR 0 leaves it in place.
	if len(params) == 0 {
		*a = Attr{Link: a.Link}
		return
	}
	for i := 0; i < len(params); i++ {
		p := params[i]
		switch p[0] {
		case 0:
			*a = Attr{Link: a.Link}
		case 1:
			a.Bold = true
		case 2:
			a.Dim = true
		case 3:
			a.Italic = true
		case 4:
			a.Underline = 1
			if len(p) > 1 {
				a.Underline = uint8(min(p[1], 5))
			}
		case 5, 6:
			a.Blink = true
		case 7:
			a.Reverse = true
		case 8:
			a.Hidden = true
		case 9:
			a.Strike = true
		case 21:
			a.Underline = 2
		case 22:
			a.Bold, a.Dim = false, false
		case 23:
			a.Italic = false
		case 24:
			a.Underline = 0
		case 25:
			a.Blink = false
		case 27:
			a.Reverse = false
		case 28:
			a.Hidden = false
		case 29:
			a.Strike = false
		case 30, 31, 32, 33, 34, 35, 36, 37:
			a.FG = Color{Kind: ColorIndexed, Index: uint8(p[0] - 30)}
		case 39:
			a.FG = Color{}
		case 40, 41, 42, 43, 44, 45, 46, 47:
			a.BG = Color{Kind: ColorIndexed, Index: uint8(p[0] - 40)}
		case 49:
			a.BG = Color{}
		case 90, 91, 92, 93, 94, 95, 96, 97:
			a.FG = Color{Kind: ColorIndexed, Index: uint8(p[0] - 90 + 8)}
		case 100, 101, 102, 103, 104, 105, 106, 107:
			a.BG = Color{Kind: ColorIndexed, Index: uint8(p[0] - 100 + 8)}
		case 38, 48, 58:
			var col Color
			var used int
			if len(p) > 1 {
				col, _ = extendedColor(p[1:], true)
			} else {
				col, used = extendedColor(flatten(params[i+1:]), false)
				i += used
			}
			switch p[0] {
			case 38:
				a.FG = col
			case 48:
				a.BG = col
			}
		case 59:
			// underline color reset; not tracked
		}
	}
}

func flatten(params [][]int) []int {
	out := make([]int, 0, len(params))
	for _, p := range params {
		out = append(out, p[0])
	}
	return out
}

// extendedColor decodes 5;n or 2;r;g;b (and the colon forms 5:n, 2::r:g:b).
// It returns the color and how many semicolon parameters it consumed.
func extendedColor(v []int, colon bool) (Color, int) {
	if len(v) == 0 {
		return Color{}, 0
	}
	switch v[0] {
	case 5:
		if len(v) >= 2 {
			return Color{Kind: ColorIndexed, Index: uint8(v[1])}, 2
		}
	case 2:
		rgb := v[1:]
		if colon && len(rgb) >= 4 {
			rgb = rgb[1:] // skip the color-space id
		}
		if len(rgb) >= 3 {
			return Color{Kind: ColorRGB, R: uint8(rgb[0]), G: uint8(rgb[1]), B: uint8(rgb[2])}, 4
		}
	}
	return Color{}, len(v)
}

// oscDispatch handles operating system commands.
func (t *Terminal) oscDispatch(s []byte) {
	str := string(s)
	code, rest, _ := strings.Cut(str, ";")
	switch code {
	case "0", "2":
		t.title = rest
	case "4":
		// Palette query: OSC 4 ; index ; ?
		idx, q, ok := strings.Cut(rest, ";")
		if ok && q == "?" {
			var n int
			if _, err := fmt.Sscanf(idx, "%d", &n); err == nil && n >= 0 && n < 256 {
				r, g, b := palette(n)
				t.reply(fmt.Sprintf("\x1b]4;%d;rgb:%02x%02x/%02x%02x/%02x%02x\x1b\\", n, r, r, g, g, b, b))
			}
		}
	case "10":
		if rest == "?" {
			t.reply("\x1b]10;rgb:e5e5/e5e5/e5e5\x1b\\")
		}
	case "11":
		if rest == "?" {
			t.reply("\x1b]11;rgb:1e1e/1e1e/1e1e\x1b\\")
		}
	case "12":
		if rest == "?" {
			t.reply("\x1b]12;rgb:e5e5/e5e5/e5e5\x1b\\")
		}
	case "52":
		t.clipboard(rest)
	case "8":
		// Hyperlink: OSC 8 ; params ; url opens one, an empty url ends it.
		_, url, _ := strings.Cut(rest, ";")
		t.cur.Attr.Link = t.linkID(url)
	}
}

// clipboard handles OSC 52: "<selection>;<base64>" writes the clipboard, and
// "<selection>;?" reads it (only when a reader is set).
func (t *Terminal) clipboard(rest string) {
	sel, data, ok := strings.Cut(rest, ";")
	if !ok {
		return
	}
	if data == "?" {
		if t.ClipboardRead != nil {
			if b := t.ClipboardRead(sel); b != nil {
				t.reply("\x1b]52;" + sel + ";" + base64.StdEncoding.EncodeToString(b) + "\x1b\\")
			}
		}
		return
	}
	if t.Clipboard == nil {
		return
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(data))
	if err != nil {
		return
	}
	t.Clipboard(sel, raw)
}

// dcsDispatch handles device control strings; only XTGETTCAP gets an answer.
func (t *Terminal) dcsDispatch(private byte, params [][]int, inter []byte, final byte, s []byte) {
	if len(inter) == 1 && inter[0] == '+' && final == 'q' {
		var ok, bad []string
		for _, name := range strings.Split(string(s), ";") {
			raw, err := hex.DecodeString(name)
			if err != nil {
				continue
			}
			switch string(raw) {
			case "RGB", "Tc", "colors", "cols", "lines":
				ok = append(ok, name)
			default:
				bad = append(bad, name)
			}
		}
		if len(ok) > 0 {
			t.reply("\x1bP1+r" + strings.Join(ok, ";") + "\x1b\\")
		} else if len(bad) > 0 {
			t.reply("\x1bP0+r" + strings.Join(bad, ";") + "\x1b\\")
		}
	}
}

func (t *Terminal) reply(s string) {
	if t.Reply != nil {
		t.Reply([]byte(s))
	}
}

// palette returns the xterm default color for a 256-color index.
func palette(n int) (r, g, b uint8) {
	base := [16][3]uint8{
		{0, 0, 0}, {205, 0, 0}, {0, 205, 0}, {205, 205, 0}, {0, 0, 238}, {205, 0, 205}, {0, 205, 205}, {229, 229, 229},
		{127, 127, 127}, {255, 0, 0}, {0, 255, 0}, {255, 255, 0}, {92, 92, 255}, {255, 0, 255}, {0, 255, 255}, {255, 255, 255},
	}
	switch {
	case n < 16:
		return base[n][0], base[n][1], base[n][2]
	case n < 232:
		n -= 16
		steps := [6]uint8{0, 95, 135, 175, 215, 255}
		return steps[n/36], steps[(n/6)%6], steps[n%6]
	default:
		v := uint8(8 + (n-232)*10)
		return v, v, v
	}
}
