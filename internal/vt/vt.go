// Package vt is a terminal emulator that tracks screen contents, cursor and
// terminal modes from a byte stream. Aft uses it twice: the client keeps a
// shadow of what the server has drawn so it can predict typed characters, and
// the session keeps the agent's screen while nobody is attached so it
// can repaint on reattach and answer the terminal queries an agent sends.
//
// The emulator never renders on its own. Callers read cells, cursor and modes,
// and Render produces the escape sequences that reproduce the current state on
// a real terminal.
package vt

import (
	"github.com/mattn/go-runewidth"
)

// ColorKind says how a Color is specified.
type ColorKind uint8

const (
	ColorDefault ColorKind = iota
	ColorIndexed
	ColorRGB
)

// Color is a foreground or background color.
type Color struct {
	Kind    ColorKind
	Index   uint8
	R, G, B uint8
}

// Attr is the visual style of a cell.
type Attr struct {
	FG, BG    Color
	Bold      bool
	Dim       bool
	Italic    bool
	Underline uint8 // 0 none, 1 single, 2 double, 3 curly, 4 dotted, 5 dashed
	Blink     bool
	Reverse   bool
	Strike    bool
	Hidden    bool
}

// Cell is one character position. Width 0 marks the continuation half of a
// wide character; Gen records the write generation that last changed the cell,
// which the prediction client uses to tell confirmed cells from stale ones.
type Cell struct {
	Rune  rune
	Attr  Attr
	Width uint8
	Gen   uint64
}

// Blank is what an erased cell contains: a space with the given attributes.
func blank(a Attr, gen uint64) Cell {
	return Cell{Rune: ' ', Attr: Attr{BG: a.BG}, Width: 1, Gen: gen}
}

// Cursor is a position plus the attributes new text is written with.
type Cursor struct {
	Row, Col int
	Attr     Attr
	Wrap     bool // pending wrap: the next printed character starts a new line
}

// MouseMode is the active mouse tracking mode, or 0 for none.
type MouseMode int

const (
	MouseOff         MouseMode = 0
	MouseX10         MouseMode = 9
	MouseNormal      MouseMode = 1000
	MouseButtonEvent MouseMode = 1002
	MouseAnyEvent    MouseMode = 1003
)

// MouseEncoding is how mouse events are encoded, or 0 for the legacy form.
type MouseEncoding int

const (
	MouseEncDefault MouseEncoding = 0
	MouseEncUTF8    MouseEncoding = 1005
	MouseEncSGR     MouseEncoding = 1006
	MouseEncURXVT   MouseEncoding = 1015
	MouseEncSGRPix  MouseEncoding = 1016
)

// Modes holds every terminal mode the emulator tracks. Render restores all of
// them, so an application reattached to a fresh terminal keeps working.
type Modes struct {
	CursorVisible  bool
	AltScreen      bool
	AutoWrap       bool
	Origin         bool
	Insert         bool
	AppCursorKeys  bool
	AppKeypad      bool
	ReverseVideo   bool
	FocusEvents    bool
	BracketedPaste bool
	SyncOutput     bool
	Mouse          MouseMode
	MouseEnc       MouseEncoding
	KittyFlags     int
	kittyStack     []int
	CursorStyle    int // DECSCUSR argument, 0 default
}

// charsets is the G0/G1 designation and which of them is shifted in.
type charsets struct {
	g0Graphics, g1Graphics bool
	shiftOut               bool // G1 is in use
}

// graphics reports whether printed ASCII goes through the special graphics set.
func (c charsets) graphics() bool {
	if c.shiftOut {
		return c.g1Graphics
	}
	return c.g0Graphics
}

// decGraphics maps the DEC special graphics set, 0x60 to 0x7E, to Unicode.
var decGraphics = [...]rune{
	'◆', '▒', '␉', '␌', '␍', '␊', '°', '±', '␤', '␋', '┘', '┐', '┌', '└', '┼', '⎺',
	'⎻', '─', '⎼', '⎽', '├', '┤', '┴', '┬', '│', '≤', '≥', 'π', '≠', '£', '·',
}

// Terminal is the emulator. It is not safe for concurrent use.
type Terminal struct {
	cols, rows int
	main, alt  *screen
	cur        Cursor
	savedMain  Cursor
	savedAlt   Cursor
	top, bot   int // scroll region, inclusive rows
	tabs       []bool
	modes      Modes
	title      string
	gen        uint64
	repeat     rune // the last printed character, which CSI b repeats; 0 after anything else

	// Character sets: G0 and G1 may each be designated as the DEC special
	// graphics set, which ncurses uses for box drawing (smacs is ESC ( 0),
	// and SO/SI shift between them. Saved and restored with the cursor.
	charset      charsets
	savedCharset [2]charsets // main, alt

	scrollback    [][]Cell
	maxScrollback int

	// Reply receives responses to terminal queries such as cursor position
	// reports. When nil, queries are tracked but not answered; a real terminal
	// downstream is expected to answer instead.
	Reply func([]byte)

	// Clipboard, when set, is called for an OSC 52 clipboard write with the
	// selection letters and the decoded bytes. The session leaves it nil; the
	// client sets the local clipboard from it, so a copy made on the server
	// reaches the laptop even when the terminal blocks OSC 52.
	Clipboard func(sel string, data []byte)

	// ClipboardRead, when set, answers an OSC 52 read request. Nil ignores
	// reads, the safe default, so a remote application cannot silently read the
	// local clipboard.
	ClipboardRead func(sel string) []byte

	// Bells counts BEL characters seen; callers may reset it.
	Bells int

	p parser
}

type screen struct {
	cells []Cell
}

// New returns an emulator of the given size with default modes.
func New(cols, rows int) *Terminal {
	t := &Terminal{maxScrollback: 5000}
	t.modes = Modes{CursorVisible: true, AutoWrap: true}
	t.resizeTo(cols, rows, true)
	return t
}

// Size returns the current columns and rows.
func (t *Terminal) Size() (cols, rows int) { return t.cols, t.rows }

// Cursor returns the current cursor.
func (t *Terminal) Cursor() Cursor { return t.cur }

// Modes returns the current modes.
func (t *Terminal) Modes() Modes { return t.modes }

// Title returns the window title set with OSC 0 or 2.
func (t *Terminal) Title() string { return t.title }

// Gen returns the write generation, which increases whenever cells change.
func (t *Terminal) Gen() uint64 { return t.gen }

// Cell returns the cell at row and column, or a blank cell when out of range.
func (t *Terminal) Cell(row, col int) Cell {
	if row < 0 || row >= t.rows || col < 0 || col >= t.cols {
		return Cell{Rune: ' ', Width: 1}
	}
	return t.active().cells[row*t.cols+col]
}

// Row returns a copy of one row of cells.
func (t *Terminal) Row(row int) []Cell {
	out := make([]Cell, t.cols)
	if row >= 0 && row < t.rows {
		copy(out, t.active().cells[row*t.cols:(row+1)*t.cols])
	}
	return out
}

// Line returns the text of one row with trailing spaces removed.
func (t *Terminal) Line(row int) string {
	return lineText(t.Row(row))
}

// Scrollback returns lines that scrolled off the top of the main screen,
// oldest first.
func (t *Terminal) Scrollback() [][]Cell { return t.scrollback }

// SnapshotData is a terminal's scrollback and visible screen as rows of cells,
// with the cursor's position within that combined buffer. The session sends it to
// the client for copy mode, which renders and navigates it locally.
type SnapshotData struct {
	Cols, Rows int
	Lines      [][]Cell
	CursorRow  int
	CursorCol  int
}

// snapshotMaxCells bounds a snapshot so its encoded frame stays well under the
// protocol limit even for a full scrollback of wide content.
const snapshotMaxCells = 1 << 20

// Snapshot returns the scrollback followed by the visible screen for copy mode,
// each row trimmed of trailing blank cells and copied fresh, plus the cursor's
// position in the combined buffer. The oldest scrollback is dropped if needed
// to keep the whole snapshot within snapshotMaxCells.
func (t *Terminal) Snapshot() SnapshotData {
	rows := make([][]Cell, 0, len(t.scrollback)+t.rows)
	for _, l := range t.scrollback {
		rows = append(rows, trimTrailingBlank(l))
	}
	for r := 0; r < t.rows; r++ {
		rows = append(rows, trimTrailingBlank(t.active().cells[r*t.cols:(r+1)*t.cols]))
	}
	cursorRow := len(t.scrollback) + t.cur.Row
	// Drop oldest scrollback until within budget, keeping the whole screen.
	total := 0
	for _, r := range rows {
		total += len(r)
	}
	drop := 0
	for total > snapshotMaxCells && drop < len(t.scrollback) {
		total -= len(rows[drop])
		drop++
	}
	rows = rows[drop:]
	cursorRow -= drop
	if cursorRow < 0 {
		cursorRow = 0
	}
	return SnapshotData{Cols: t.cols, Rows: t.rows, Lines: rows, CursorRow: cursorRow, CursorCol: t.cur.Col}
}

// trimTrailingBlank returns a fresh copy of a row with trailing default-blank
// cells removed, which makes a snapshot of ordinary output far smaller.
func trimTrailingBlank(cells []Cell) []Cell {
	end := len(cells)
	for end > 0 {
		c := cells[end-1]
		if c.Width == 1 && (c.Rune == ' ' || c.Rune == 0) && c.Attr == (Attr{}) {
			end--
			continue
		}
		break
	}
	row := make([]Cell, end)
	copy(row, cells[:end])
	return row
}

// LineText returns the trimmed text of a row of cells, for copy mode selection.
func LineText(cells []Cell) string { return lineText(cells) }

// InGround reports whether the parser is between sequences and not inside a
// partial multibyte character. Output may be safely interposed only then.
func (t *Terminal) InGround() bool { return t.p.state == stGround && t.p.utf8n == 0 }

// ReverseCell returns the position of the single reverse-video cell on screen,
// which applications that hide the terminal cursor use as a drawn caret.
func (t *Terminal) ReverseCell() (row, col int, ok bool) {
	cells := t.active().cells
	found := -1
	for i, c := range cells {
		if c.Attr.Reverse && c.Width != 0 {
			if found >= 0 {
				return 0, 0, false
			}
			found = i
		}
	}
	if found < 0 {
		return 0, 0, false
	}
	return found / t.cols, found % t.cols, true
}

func lineText(cells []Cell) string {
	end := len(cells)
	for end > 0 && (cells[end-1].Width == 0 || cells[end-1].Rune == ' ' || cells[end-1].Rune == 0) {
		end--
	}
	var b []rune
	for _, c := range cells[:end] {
		if c.Width == 0 {
			continue
		}
		if c.Rune == 0 {
			b = append(b, ' ')
		} else {
			b = append(b, c.Rune)
		}
	}
	return string(b)
}

func (t *Terminal) active() *screen {
	if t.modes.AltScreen {
		return t.alt
	}
	return t.main
}

// Resize changes the terminal size. Contents are truncated or padded; there is
// no reflow. The scroll region resets to the full screen.
func (t *Terminal) Resize(cols, rows int) { t.resizeTo(cols, rows, false) }

func (t *Terminal) resizeTo(cols, rows int, fresh bool) {
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	t.gen++
	resize := func(s *screen) *screen {
		n := &screen{cells: make([]Cell, cols*rows)}
		for i := range n.cells {
			n.cells[i] = Cell{Rune: ' ', Width: 1, Gen: t.gen}
		}
		if s == nil || fresh {
			return n
		}
		for r := 0; r < rows && r < t.rows; r++ {
			for c := 0; c < cols && c < t.cols; c++ {
				n.cells[r*cols+c] = s.cells[r*t.cols+c]
			}
		}
		return n
	}
	t.main = resize(t.main)
	t.alt = resize(t.alt)
	t.cols, t.rows = cols, rows
	t.top, t.bot = 0, rows-1
	t.tabs = make([]bool, cols)
	for i := 8; i < cols; i += 8 {
		t.tabs[i] = true
	}
	t.clampCursor()
	t.cur.Wrap = false
}

func (t *Terminal) clampCursor() {
	if t.cur.Row >= t.rows {
		t.cur.Row = t.rows - 1
	}
	if t.cur.Row < 0 {
		t.cur.Row = 0
	}
	if t.cur.Col >= t.cols {
		t.cur.Col = t.cols - 1
	}
	if t.cur.Col < 0 {
		t.cur.Col = 0
	}
}

// Write feeds output bytes to the emulator.
func (t *Terminal) Write(b []byte) (int, error) {
	for _, c := range b {
		t.p.feed(t, c)
	}
	return len(b), nil
}

// ---- screen operations ----

func (t *Terminal) set(row, col int, c Cell) {
	if row < 0 || row >= t.rows || col < 0 || col >= t.cols {
		return
	}
	c.Gen = t.gen
	t.active().cells[row*t.cols+col] = c
}

func (t *Terminal) print(r rune) {
	if r >= 0x60 && r <= 0x7E && t.charset.graphics() {
		r = decGraphics[r-0x60]
	}
	w := runewidth.RuneWidth(r)
	if w == 0 {
		return
	}
	t.repeat = r
	if t.cur.Wrap {
		if t.modes.AutoWrap {
			t.cur.Col = 0
			t.index()
		}
		t.cur.Wrap = false
	}
	if w == 2 && t.cur.Col == t.cols-1 {
		// A wide character does not fit in the last column.
		t.set(t.cur.Row, t.cur.Col, blank(t.cur.Attr, t.gen))
		if !t.modes.AutoWrap {
			return
		}
		t.cur.Col = 0
		t.index()
	}
	t.gen++
	if t.modes.Insert {
		t.insertCells(t.cur.Row, t.cur.Col, w)
	}
	// Overwriting half of a wide character blanks the other half.
	t.breakWide(t.cur.Row, t.cur.Col)
	if w == 2 {
		t.breakWide(t.cur.Row, t.cur.Col+1)
	}
	t.set(t.cur.Row, t.cur.Col, Cell{Rune: r, Attr: t.cur.Attr, Width: uint8(w)})
	if w == 2 {
		t.set(t.cur.Row, t.cur.Col+1, Cell{Rune: 0, Attr: t.cur.Attr, Width: 0})
	}
	t.cur.Col += w
	if t.cur.Col >= t.cols {
		t.cur.Col = t.cols - 1
		t.cur.Wrap = true
	}
}

func (t *Terminal) breakWide(row, col int) {
	if col < 0 || col >= t.cols {
		return
	}
	cells := t.active().cells
	i := row*t.cols + col
	switch {
	case cells[i].Width == 0 && col > 0 && cells[i-1].Width == 2:
		cells[i-1] = blank(cells[i-1].Attr, t.gen)
	case cells[i].Width == 2 && col+1 < t.cols:
		cells[i+1] = blank(cells[i+1].Attr, t.gen)
	}
}

func (t *Terminal) insertCells(row, col, n int) {
	cells := t.active().cells
	base := row * t.cols
	for c := t.cols - 1; c >= col+n; c-- {
		cells[base+c] = cells[base+c-n]
		cells[base+c].Gen = t.gen
	}
	for c := col; c < col+n && c < t.cols; c++ {
		cells[base+c] = blank(t.cur.Attr, t.gen)
	}
}

func (t *Terminal) deleteCells(row, col, n int) {
	cells := t.active().cells
	base := row * t.cols
	for c := col; c < t.cols; c++ {
		if c+n < t.cols {
			cells[base+c] = cells[base+c+n]
		} else {
			cells[base+c] = blank(t.cur.Attr, t.gen)
		}
		cells[base+c].Gen = t.gen
	}
}

// index moves the cursor down one line, scrolling the region if at its bottom.
func (t *Terminal) index() {
	if t.cur.Row == t.bot {
		t.scrollUp(1)
	} else if t.cur.Row < t.rows-1 {
		t.cur.Row++
	}
}

func (t *Terminal) reverseIndex() {
	if t.cur.Row == t.top {
		t.scrollDown(1)
	} else if t.cur.Row > 0 {
		t.cur.Row--
	}
}

func (t *Terminal) scrollUp(n int) {
	t.gen++
	cells := t.active().cells
	for i := 0; i < n; i++ {
		if !t.modes.AltScreen && t.top == 0 && t.bot == t.rows-1 {
			line := make([]Cell, t.cols)
			copy(line, cells[0:t.cols])
			t.scrollback = append(t.scrollback, line)
			if len(t.scrollback) > t.maxScrollback {
				t.scrollback = t.scrollback[len(t.scrollback)-t.maxScrollback:]
			}
		}
		copy(cells[t.top*t.cols:], cells[(t.top+1)*t.cols:(t.bot+1)*t.cols])
		for c := 0; c < t.cols; c++ {
			cells[t.bot*t.cols+c] = blank(t.cur.Attr, t.gen)
		}
	}
	for i := t.top * t.cols; i < (t.bot+1)*t.cols; i++ {
		cells[i].Gen = t.gen
	}
}

func (t *Terminal) scrollDown(n int) {
	t.gen++
	cells := t.active().cells
	for i := 0; i < n; i++ {
		copy(cells[(t.top+1)*t.cols:(t.bot+1)*t.cols], cells[t.top*t.cols:t.bot*t.cols])
		for c := 0; c < t.cols; c++ {
			cells[t.top*t.cols+c] = blank(t.cur.Attr, t.gen)
		}
	}
	for i := t.top * t.cols; i < (t.bot+1)*t.cols; i++ {
		cells[i].Gen = t.gen
	}
}

func (t *Terminal) eraseRegion(r0, c0, r1, c1 int) {
	t.gen++
	for r := r0; r <= r1; r++ {
		for c := c0; c <= c1; c++ {
			t.set(r, c, blank(t.cur.Attr, t.gen))
		}
	}
}

func (t *Terminal) eraseDisplay(mode int) {
	switch mode {
	case 0:
		t.eraseRegion(t.cur.Row, t.cur.Col, t.cur.Row, t.cols-1)
		if t.cur.Row+1 < t.rows {
			t.eraseRegion(t.cur.Row+1, 0, t.rows-1, t.cols-1)
		}
	case 1:
		if t.cur.Row > 0 {
			t.eraseRegion(0, 0, t.cur.Row-1, t.cols-1)
		}
		t.eraseRegion(t.cur.Row, 0, t.cur.Row, t.cur.Col)
	case 2:
		t.eraseRegion(0, 0, t.rows-1, t.cols-1)
	case 3:
		t.scrollback = nil
	}
}

func (t *Terminal) eraseLine(mode int) {
	switch mode {
	case 0:
		t.eraseRegion(t.cur.Row, t.cur.Col, t.cur.Row, t.cols-1)
	case 1:
		t.eraseRegion(t.cur.Row, 0, t.cur.Row, t.cur.Col)
	case 2:
		t.eraseRegion(t.cur.Row, 0, t.cur.Row, t.cols-1)
	}
}

func (t *Terminal) insertLines(n int) {
	if t.cur.Row < t.top || t.cur.Row > t.bot {
		return
	}
	saveTop := t.top
	t.top = t.cur.Row
	t.scrollDown(n)
	t.top = saveTop
}

func (t *Terminal) deleteLines(n int) {
	if t.cur.Row < t.top || t.cur.Row > t.bot {
		return
	}
	saveTop := t.top
	t.top = t.cur.Row
	alt := t.modes.AltScreen
	t.modes.AltScreen = true // suppress scrollback capture for region scrolls
	t.scrollUp(n)
	t.modes.AltScreen = alt
	t.top = saveTop
}

func (t *Terminal) moveTo(row, col int) {
	if t.modes.Origin {
		row += t.top
		if row > t.bot {
			row = t.bot
		}
	}
	t.cur.Row, t.cur.Col = row, col
	t.clampCursor()
	t.cur.Wrap = false
}

func (t *Terminal) setAltScreen(on bool, saveCursor bool) {
	if on == t.modes.AltScreen {
		return
	}
	t.gen++
	if on {
		if saveCursor {
			t.savedMain = t.cur
		}
		t.modes.AltScreen = true
		for i := range t.alt.cells {
			t.alt.cells[i] = Cell{Rune: ' ', Width: 1, Gen: t.gen}
		}
	} else {
		t.modes.AltScreen = false
		if saveCursor {
			t.cur = t.savedMain
			t.clampCursor()
		}
	}
	t.cur.Wrap = false
}

func (t *Terminal) saveCursor() {
	if t.modes.AltScreen {
		t.savedAlt, t.savedCharset[1] = t.cur, t.charset
	} else {
		t.savedMain, t.savedCharset[0] = t.cur, t.charset
	}
}

func (t *Terminal) restoreCursor() {
	if t.modes.AltScreen {
		t.cur, t.charset = t.savedAlt, t.savedCharset[1]
	} else {
		t.cur, t.charset = t.savedMain, t.savedCharset[0]
	}
	t.clampCursor()
	t.cur.Wrap = false
}

func (t *Terminal) reset() {
	cols, rows := t.cols, t.rows
	modes := Modes{CursorVisible: true, AutoWrap: true}
	t.modes = modes
	t.cur = Cursor{}
	t.savedMain, t.savedAlt = Cursor{}, Cursor{}
	t.charset, t.savedCharset = charsets{}, [2]charsets{}
	t.repeat = 0
	t.title = ""
	t.resizeTo(cols, rows, true)
}

func (t *Terminal) tab() {
	for c := t.cur.Col + 1; c < t.cols; c++ {
		if t.tabs[c] {
			t.cur.Col = c
			t.cur.Wrap = false
			return
		}
	}
	t.cur.Col = t.cols - 1
	t.cur.Wrap = false
}

func (t *Terminal) backTab(n int) {
	for ; n > 0; n-- {
		c := t.cur.Col - 1
		for c > 0 && !t.tabs[c] {
			c--
		}
		t.cur.Col = c
	}
	t.cur.Wrap = false
}
