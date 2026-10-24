package terminal

import (
	"io"
	"strings"
	"sync"
	"time"

	"github.com/mattn/go-runewidth"

	"github.com/arocomputer/aft/internal/vt"
)

// Predictor sits between the session and the user's terminal. Server output
// passes through byte for byte while a shadow emulator tracks what the server
// has drawn. When the user types, the predictor draws the expected result at
// the caret immediately, then reconciles it when the server's echo arrives.
//
// It only ever writes to the terminal between the server's escape sequences,
// and before passing on more server output it moves the real cursor back to
// where the server believes it is, so the two streams never interleave badly.
type Predictor struct {
	mu      sync.Mutex
	shadow  *vt.Terminal
	w       io.Writer
	enabled bool

	// Tracked state of the real terminal between server writes. Only our own
	// predictions change it, so we can emit the fewest bytes: a bare character
	// when the cursor and pen are already right, exactly like native echo.
	// displaced means our predictions moved the real cursor/pen off the shadow,
	// so Output must resync before applying server output.
	displaced bool
	realRow   int
	realCol   int
	realPen   vt.Attr
	posKnown  bool
	penKnown  bool
	pending   []prediction
	confident bool // echoes have been matching; predictions are drawn
	srtt      time.Duration
	lastKey   time.Time

	// The overlay caret: where the next predicted character goes while earlier
	// predictions are still unconfirmed. It is only meaningful while pending
	// is non-empty and is cleared by any key whose effect we cannot foresee.
	caretValid bool
	caretRow   int
	caretCol   int
	// typed counts the characters typed since the last key whose effect was
	// unforeseeable. A Backspace is predicted only over one of them: what is
	// left of the caret otherwise, such as a prompt, may be nothing the
	// application would erase.
	typed int

	onClip func([]byte)
}

type prediction struct {
	row, col int
	r        rune
	gen      uint64
	at       time.Time
	drawn    bool
	erase    bool // a Backspace: the cell is predicted blank, left of the caret
}

// NewPredictor returns a predictor writing passthrough and overlays to w.
func NewPredictor(cols, rows int, w io.Writer, enabled bool) *Predictor {
	return &Predictor{shadow: vt.New(cols, rows), w: w, enabled: enabled, confident: true, srtt: 150 * time.Millisecond}
}

// OnClipboard routes the server's OSC 52 clipboard writes to fn. The callback
// is reapplied after Reset so it survives a reconnect.
func (p *Predictor) OnClipboard(fn func([]byte)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onClip = fn
	p.shadow.Clipboard = func(_ string, data []byte) { fn(data) }
}

// Resize follows the terminal size and drops in-flight predictions.
func (p *Predictor) Resize(cols, rows int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.shadow.Resize(cols, rows)
	p.pending = nil
	p.caretValid = false
	p.displaced = false
	p.posKnown, p.penKnown = false, false
}

// Reset forgets everything, for a fresh attach where the session repaints.
func (p *Predictor) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	cols, rows := p.shadow.Size()
	p.shadow = vt.New(cols, rows)
	if p.onClip != nil {
		fn := p.onClip
		p.shadow.Clipboard = func(_ string, data []byte) { fn(data) }
	}
	p.pending = nil
	p.caretValid = false
	p.displaced = false
	p.posKnown, p.penKnown = false, false
	p.confident = true
}

// Output passes server output to the terminal and updates the shadow.
func (p *Predictor) Output(b []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.displaced && p.shadow.InGround() {
		// Put the real cursor and attributes back where the server left them.
		c := p.shadow.Cursor()
		fix := p.serverCursor() + vt.SGR(c.Attr)
		if _, err := p.w.Write([]byte(fix)); err != nil {
			return err
		}
		p.displaced = false
	}
	if _, err := p.w.Write(b); err != nil {
		return err
	}
	p.shadow.Write(b)
	// The real cursor now matches the shadow; the pen is the server's, unknown
	// to us, so the next prediction re-states it once.
	cur := p.shadow.Cursor()
	p.realRow, p.realCol, p.posKnown = cur.Row, cur.Col, !cur.Wrap
	p.penKnown = false
	p.reconcile()
	return nil
}

// Key handles one input token. The token has already been forwarded to the
// session; Key only decides whether to draw a prediction.
func (p *Predictor) Key(t Token) {
	if !p.enabled {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	p.lastKey = now
	switch t.Kind {
	case TokText:
		p.predictText(t.Rune, now)
	case TokBackspace:
		p.predictBackspace(now)
	default:
		// Anything else may move the caret in ways we cannot foresee.
		p.caretValid = false
		p.typed = 0
	}
}

// shadowCaret returns where the server's caret is: the terminal cursor, or
// the single reverse-video cell when the application hides the cursor and
// draws its own (drawn=true). ok is false when neither can be found.
func (p *Predictor) shadowCaret() (row, col int, drawn bool, ok bool) {
	if p.shadow.Modes().CursorVisible {
		cur := p.shadow.Cursor()
		if cur.Wrap {
			return 0, 0, false, false
		}
		return cur.Row, cur.Col, false, true
	}
	r, c, found := p.shadow.ReverseCell()
	return r, c, true, found
}

// caret returns where the next typed character will appear, or ok=false.
// With no predictions in flight that is the server's caret. With predictions
// in flight it is the overlay caret, which runs ahead of the server's as the
// echoes trail behind; if a key with an unforeseeable effect intervened, or
// the server's caret left the row, there is no safe answer until the pending
// predictions settle.
func (p *Predictor) caret() (row, col int, drawn bool, ok bool) {
	r, c, drawn, ok := p.shadowCaret()
	if !ok {
		return 0, 0, false, false
	}
	if len(p.pending) > 0 {
		if !p.caretValid || p.caretRow != r {
			return 0, 0, false, false
		}
		c = p.caretCol
	}
	cols, _ := p.shadow.Size()
	if c >= cols-1 {
		return 0, 0, false, false
	}
	return r, c, drawn, true
}

func (p *Predictor) predictText(r rune, now time.Time) {
	if !p.shadow.InGround() || runewidth.RuneWidth(r) != 1 {
		p.caretValid = false
		return
	}
	row, col, drawnCaret, ok := p.caret()
	if !ok {
		p.caretValid = false
		return
	}
	cell := p.shadow.Cell(row, col)
	if cell.Width == 0 {
		return
	}
	p.typed++
	pred := prediction{row: row, col: col, r: r, gen: p.shadow.Gen(), at: now, drawn: p.confident}
	if p.confident {
		attr := cell.Attr
		if drawnCaret {
			attr.Reverse = false
		}
		// Emit only what the real terminal does not already have: a cursor move
		// if it is not at the target, a pen change if the style differs, then
		// the character. In steady typing this is a single byte.
		var out []byte
		if !p.posKnown || p.realRow != row || p.realCol != col {
			out = append(out, vt.CUP(row, col)...)
		}
		if !p.penKnown || p.realPen != attr {
			out = append(out, vt.SGR(attr)...)
		}
		out = append(out, string(r)...)
		p.realRow, p.realCol = row, col+1
		p.realPen, p.penKnown, p.posKnown = attr, true, true
		if drawnCaret {
			// Move the drawn caret block one cell right, then force a resync.
			next := p.shadow.Cell(row, col+1)
			na := next.Attr
			na.Reverse = true
			nr := next.Rune
			if nr == 0 {
				nr = ' '
			}
			out = append(out, vt.SGR(na)...)
			out = append(out, string(nr)...)
			p.posKnown, p.penKnown = false, false
		}
		p.w.Write(out)
		p.displaced = true
	}
	p.pending = append(p.pending, pred)
	p.caretValid, p.caretRow, p.caretCol = true, row, col+1
}

func (p *Predictor) predictBackspace(now time.Time) {
	if !p.confident || !p.shadow.InGround() || p.typed == 0 {
		p.caretValid = false
		p.typed = 0
		return
	}
	p.typed--
	// Undo the most recent unconfirmed prediction if there is one on this row.
	if n := len(p.pending); n > 0 && p.pending[n-1].drawn && p.caretValid && p.pending[n-1].row == p.caretRow && p.pending[n-1].col == p.caretCol-1 {
		last := p.pending[n-1]
		p.pending = p.pending[:n-1]
		cell := p.shadow.Cell(last.row, last.col)
		// The cell shows what an earlier prediction put there, if one did,
		// such as a predicted erase; otherwise what the server has.
		for i := len(p.pending) - 1; i >= 0; i-- {
			if pr := p.pending[i]; pr.row == last.row && pr.col == last.col {
				cell.Rune = pr.r
				break
			}
		}
		out := vt.CUP(last.row, last.col) + vt.SGR(cell.Attr) + cellRune(cell)
		if !p.shadow.Modes().CursorVisible {
			// Restore the drawn caret to this cell and clear the moved one.
			a := cell.Attr
			a.Reverse = true
			out = vt.CUP(last.row, last.col) + vt.SGR(a) + cellRune(cell)
			next := p.shadow.Cell(last.row, last.col+1)
			out += vt.SGR(next.Attr) + cellRune(next)
		}
		// Leave the cursor on the cell the next character goes into.
		out += vt.CUP(last.row, last.col)
		p.w.Write([]byte(out))
		p.displaced = true
		p.posKnown, p.penKnown = false, false
		p.caretCol = last.col
		return
	}
	row, col, drawnCaret, ok := p.caret()
	if !ok || col == 0 {
		p.caretValid = false
		return
	}
	target := p.shadow.Cell(row, col-1)
	if target.Width != 1 {
		p.caretValid = false
		return
	}
	attr := target.Attr
	attr.Reverse = false
	out := vt.CUP(row, col-1) + vt.SGR(attr) + " "
	if drawnCaret {
		a := attr
		a.Reverse = true
		out = vt.CUP(row, col-1) + vt.SGR(a) + " "
		cur := p.shadow.Cell(row, col)
		ca := cur.Attr
		ca.Reverse = false
		out += vt.SGR(ca) + cellRune(cur)
	}
	out += vt.CUP(row, col-1) // where the next character goes
	p.w.Write([]byte(out))
	p.displaced = true
	p.posKnown, p.penKnown = false, false
	p.pending = append(p.pending, prediction{row: row, col: col - 1, r: ' ', gen: p.shadow.Gen(), at: now, drawn: true, erase: true})
	p.caretValid, p.caretRow, p.caretCol = true, row, col-1
}

func cellRune(c vt.Cell) string {
	if c.Rune == 0 || c.Width == 0 {
		return " "
	}
	return string(c.Rune)
}

// reconcile checks pending predictions against the shadow after server
// output. A prediction is confirmed when the server writes the predicted
// character into its cell, or, for a typed character, when the server's caret
// moves past a cell that already shows it: some renderers echo a space by
// moving over the blank cell. When the server blanks a cell it has not
// reached yet (at or right of its caret on the same row), it has simply not
// processed that key: applications such as Ink-based agents redraw the whole
// input line on every echo, erasing the cells still in flight. Such
// predictions are kept and redrawn, otherwise the caret visibly snaps back on
// every echo. That holds only while every key since was foreseeable: after
// Enter or Ctrl-C the app may rightly clear the line, so the predictions are
// dropped, not counted against us. A cell filled with something else, as in
// vim's normal mode, is a wrong prediction.
func (p *Predictor) reconcile() {
	if len(p.pending) == 0 {
		return
	}
	cr, cc, drawnCaret, caretOK := p.shadowCaret()
	now := time.Now()
	keep := p.pending[:0]
	var erased []int // indexes into keep of drawn predictions to redraw
	for _, pr := range p.pending {
		cell := p.shadow.Cell(pr.row, pr.col)
		switch {
		case cell.Gen <= pr.gen && !pr.erase && caretOK && pr.row == cr && cc > pr.col && holds(cell, pr.r):
			// The server moved its caret past the cell without rewriting it,
			// which is how some renderers echo a space: it handled the key.
			p.confident = true
		case cell.Gen <= pr.gen:
			keep = append(keep, pr)
		case cell.Rune == pr.r:
			// Confirmed. Learn the echo time.
			rtt := now.Sub(pr.at)
			p.srtt = (p.srtt*7 + rtt) / 8
			p.confident = true
		case !p.caretValid:
			// A key we could not foresee came after it; the app may rightly
			// have cleared it. Drop it without judging.
		case caretOK && pr.row == cr && pr.col >= cc && holds(cell, ' '):
			// Blanked ahead of the server's caret: still in flight.
			if pr.drawn {
				erased = append(erased, len(keep))
			}
			keep = append(keep, pr)
		default:
			// The server drew something else there; passthrough already
			// repainted the cell, so only our trust changes.
			p.confident = false
		}
	}
	p.pending = keep
	if len(p.pending) == 0 {
		p.caretValid = false
		return
	}
	if !p.confident || !p.shadow.InGround() {
		// Without a redraw the screen shows the server's cells, which is
		// always correct; the next output or Tick settles the rest.
		return
	}
	if len(erased) == 0 {
		p.placeCursor(cr, drawnCaret, caretOK)
		return
	}
	var out []byte
	gen := p.shadow.Gen()
	for _, i := range erased {
		pr := &p.pending[i]
		a := p.shadow.Cell(pr.row, pr.col).Attr
		a.Reverse = false
		out = append(out, vt.CUP(pr.row, pr.col)+vt.SGR(a)+string(pr.r)...)
		pr.gen = gen // judge it again only if the server overwrites it again
	}
	if drawnCaret && p.caretValid && p.caretRow == cr {
		// The server's redraw put its caret back at its own position; move
		// the drawn caret block to the end of the predictions again.
		next := p.shadow.Cell(p.caretRow, p.caretCol)
		na := next.Attr
		na.Reverse = true
		out = append(out, vt.CUP(p.caretRow, p.caretCol)+vt.SGR(na)+cellRune(next)...)
	}
	p.w.Write(out)
	p.displaced = true
	p.posKnown, p.penKnown = false, false
	p.placeCursor(cr, drawnCaret, caretOK)
}

// placeCursor puts the terminal's cursor after the predictions still in
// flight. Server output leaves it where the server's input ends, which trails
// what was typed, so an application that redraws on its own clock (Claude
// Code's classic renderer does, about twice a second) would otherwise make
// the cursor jump back and forth. It applies only to a visible cursor on the
// server caret's row; a drawn caret is moved by reconcile itself.
func (p *Predictor) placeCursor(serverRow int, drawnCaret, caretOK bool) {
	if !caretOK || drawnCaret || !p.caretValid || p.caretRow != serverRow || !p.allDrawn() {
		return
	}
	if p.posKnown && p.realRow == p.caretRow && p.realCol == p.caretCol {
		return
	}
	p.w.Write([]byte(vt.CUP(p.caretRow, p.caretCol)))
	p.realRow, p.realCol, p.posKnown = p.caretRow, p.caretCol, true
	p.displaced = true
}

// allDrawn reports whether every pending prediction is on screen, so the
// cursor may go after them without skipping cells that show nothing yet.
func (p *Predictor) allDrawn() bool {
	for _, pr := range p.pending {
		if !pr.drawn {
			return false
		}
	}
	return true
}

// holds reports whether cell already shows r, a blank counting as a space.
func holds(cell vt.Cell, r rune) bool {
	return cell.Rune == r || (r == ' ' && cell.Rune == 0)
}

// Tick expires predictions the server never answered and repaints them from
// the shadow. Callers run it periodically.
func (p *Predictor) Tick() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.pending) == 0 {
		return
	}
	timeout := 3 * p.srtt
	if timeout < 300*time.Millisecond {
		timeout = 300 * time.Millisecond
	}
	if timeout > 2*time.Second {
		timeout = 2 * time.Second
	}
	now := time.Now()
	keep := p.pending[:0]
	var out []byte
	for _, pr := range p.pending {
		if now.Sub(pr.at) < timeout {
			keep = append(keep, pr)
			continue
		}
		if pr.drawn && p.shadow.InGround() {
			cell := p.shadow.Cell(pr.row, pr.col)
			out = append(out, []byte(vt.CUP(pr.row, pr.col)+vt.SGR(cell.Attr)+cellRune(cell))...)
			if next := p.shadow.Cell(pr.row, pr.col+1); !p.shadow.Modes().CursorVisible {
				out = append(out, []byte(vt.SGR(next.Attr)+cellRune(next))...)
			}
		}
		p.confident = false
	}
	p.pending = keep
	if len(p.pending) == 0 {
		p.caretValid = false
	}
	if len(out) > 0 {
		// Repainting left the cursor on the last repainted cell; put it back
		// after the predictions still in flight, or where the server has it.
		cur := p.shadow.Cursor()
		row, col := cur.Row, cur.Col
		if p.caretValid && p.caretRow == cur.Row && p.allDrawn() {
			col = p.caretCol
		}
		if p.shadow.Modes().CursorVisible && !cur.Wrap {
			out = append(out, vt.CUP(row, col)...)
		}
		p.w.Write(out)
		p.displaced = true
		p.posKnown, p.penKnown = false, false
	}
}

// Absorb feeds server output to the shadow without writing to the terminal.
// Copy mode uses it so the shadow stays current while the pager owns the
// screen; Repaint then restores the live screen on exit.
func (p *Predictor) Absorb(b []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.shadow.Write(b)
}

// Repaint clears the screen and redraws the live screen from the shadow,
// dropping any predictions. Copy mode calls it on exit. It paints the current
// screen only, never scrollback, so the terminal's own history is untouched.
func (p *Predictor) Repaint() {
	p.mu.Lock()
	defer p.mu.Unlock()
	// Render clears the screen itself before painting it.
	p.w.Write(p.shadow.Render(vt.RenderOptions{Scrollback: false, Title: true}))
	p.pending = nil
	p.caretValid = false
	p.displaced = false
}

// Notice draws a one-line message in reverse video across the top row, then
// puts the cursor back where it was. It moves the cursor explicitly rather
// than with save and restore (ESC 7 / ESC 8), which would overwrite a cursor
// the application saved itself. RestoreRow takes the notice down.
func (p *Predictor) Notice(text string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.shadow.InGround() {
		return
	}
	// It stops short of the last column, so drawing it never leaves the
	// terminal waiting to wrap, whatever width the terminal gives each
	// character; callers keep notices to plain ASCII punctuation.
	cols, _ := p.shadow.Size()
	width := max(cols-1, 0)
	text = runewidth.Truncate(text, width, "")
	out := vt.CUP(0, 0) + "\x1b[0;7m" + text + strings.Repeat(" ", max(width-runewidth.StringWidth(text), 0)) + "\x1b[0m"
	p.w.Write([]byte(out + p.cursorBack()))
	p.penKnown = false
	p.displaced = true
}

// serverCursor moves the real cursor to the shadow's, wrap state included:
// at the last column with a wrap pending, the next character starts a new
// line, which a plain move would forget. Reprinting the last cell restores
// it, as it is how the terminal got there.
func (p *Predictor) serverCursor() string {
	cur := p.shadow.Cursor()
	if !cur.Wrap {
		return vt.CUP(cur.Row, cur.Col)
	}
	cols, _ := p.shadow.Size()
	col := cols - 1
	cell := p.shadow.Cell(cur.Row, col)
	if cell.Width == 0 && col > 0 {
		col-- // the last cell is the right half of a wide character
		cell = p.shadow.Cell(cur.Row, col)
	}
	return vt.CUP(cur.Row, col) + vt.SGR(cell.Attr) + cellRune(cell)
}

// cursorBack returns the real cursor to where it was before an overlay was
// drawn: where predictions left it, or the server's cursor.
func (p *Predictor) cursorBack() string {
	if p.posKnown {
		return vt.CUP(p.realRow, p.realCol)
	}
	return p.serverCursor()
}

// RestoreRow redraws one row from the shadow, taking back a notice drawn
// over it.
func (p *Predictor) RestoreRow(r int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	cols, rows := p.shadow.Size()
	if r < 0 || r >= rows || !p.shadow.InGround() {
		return
	}
	out := vt.CUP(r, 0)
	for c := 0; c < cols; c++ {
		cell := p.shadow.Cell(r, c)
		if cell.Width == 0 && c > 0 {
			continue // the right half of a wide character drawn just before
		}
		out += vt.SGR(cell.Attr) + cellRune(cell)
	}
	p.w.Write([]byte(out + p.cursorBack()))
	p.penKnown = false
	p.displaced = true
}

// Shadow exposes the emulator for tests.
func (p *Predictor) Shadow() *vt.Terminal { return p.shadow }
