package client

import (
	"io"
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

	// The overlay caret: where the next predicted character goes once earlier
	// predictions are still unconfirmed.
	caretValid bool
	caretRow   int
	caretCol   int
	baseRow    int
	baseCol    int

	onClip func([]byte)
}

type prediction struct {
	row, col int
	r        rune
	gen      uint64
	at       time.Time
	drawn    bool
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

// Reset forgets everything, for a fresh attach where the berth repaints.
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

// Title returns the title the application set, if any.
func (p *Predictor) Title() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.shadow.Title()
}

// Output passes server output to the terminal and updates the shadow.
func (p *Predictor) Output(b []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.displaced && p.shadow.InGround() {
		// Put the real cursor and attributes back where the server left them.
		c := p.shadow.Cursor()
		fix := vt.CUP(c.Row, c.Col) + vt.SGR(c.Attr)
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
	}
}

// caret returns where the next typed character will appear, or ok=false.
func (p *Predictor) caret() (row, col int, drawn bool, ok bool) {
	var r, c int
	if p.shadow.Modes().CursorVisible {
		cur := p.shadow.Cursor()
		if cur.Wrap {
			return 0, 0, false, false
		}
		r, c = cur.Row, cur.Col
	} else {
		var found bool
		r, c, found = p.shadow.ReverseCell()
		if !found {
			return 0, 0, false, false
		}
		drawn = true
	}
	// Earlier predictions still waiting for their echo push the caret right.
	if p.caretValid && p.caretRow == r && p.caretCol > c && p.baseRow == r && p.baseCol == c && len(p.pending) > 0 {
		c = p.caretCol
	} else {
		p.baseRow, p.baseCol = r, c
	}
	cols, _ := p.shadow.Size()
	if c >= cols-1 {
		return 0, 0, false, false
	}
	return r, c, drawn, true
}

func (p *Predictor) predictText(r rune, now time.Time) {
	if !p.shadow.InGround() || runewidth.RuneWidth(r) != 1 || p.shadow.Modes().AltScreen && false {
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
	if !p.confident || !p.shadow.InGround() {
		p.caretValid = false
		return
	}
	// Undo the most recent unconfirmed prediction if there is one on this row.
	if n := len(p.pending); n > 0 && p.pending[n-1].drawn && p.caretValid && p.pending[n-1].row == p.caretRow && p.pending[n-1].col == p.caretCol-1 {
		last := p.pending[n-1]
		p.pending = p.pending[:n-1]
		cell := p.shadow.Cell(last.row, last.col)
		out := vt.CUP(last.row, last.col) + vt.SGR(cell.Attr) + cellRune(cell)
		if !p.shadow.Modes().CursorVisible {
			// Restore the drawn caret to this cell and clear the moved one.
			a := cell.Attr
			a.Reverse = true
			out = vt.CUP(last.row, last.col) + vt.SGR(a) + cellRune(cell)
			next := p.shadow.Cell(last.row, last.col+1)
			out += vt.SGR(next.Attr) + cellRune(next)
		}
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
	p.w.Write([]byte(out))
	p.displaced = true
	p.posKnown, p.penKnown = false, false
	p.pending = append(p.pending, prediction{row: row, col: col - 1, r: ' ', gen: p.shadow.Gen(), at: now, drawn: true})
	p.caretValid, p.caretRow, p.caretCol = true, row, col-1
}

func cellRune(c vt.Cell) string {
	if c.Rune == 0 || c.Width == 0 {
		return " "
	}
	return string(c.Rune)
}

// reconcile checks pending predictions against the shadow after server output.
func (p *Predictor) reconcile() {
	if len(p.pending) == 0 {
		return
	}
	now := time.Now()
	keep := p.pending[:0]
	for _, pr := range p.pending {
		cell := p.shadow.Cell(pr.row, pr.col)
		switch {
		case cell.Gen > pr.gen && cell.Rune == pr.r:
			// Confirmed. Learn the echo time.
			rtt := now.Sub(pr.at)
			p.srtt = (p.srtt*7 + rtt) / 8
			p.confident = true
		case cell.Gen > pr.gen:
			// The server drew something else there; passthrough already
			// repainted the cell, so only our trust changes.
			p.confident = false
		default:
			keep = append(keep, pr)
		}
	}
	p.pending = keep
	if len(p.pending) == 0 {
		p.caretValid = false
	}
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
	if len(out) > 0 {
		p.w.Write(out)
		p.displaced = true
		p.posKnown, p.penKnown = false, false
	}
	if len(p.pending) == 0 {
		p.caretValid = false
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
	p.w.Write([]byte("[H[2J"))
	p.w.Write(p.shadow.Render(vt.RenderOptions{Scrollback: false, Title: true}))
	p.pending = nil
	p.caretValid = false
	p.displaced = false
}

// Shadow exposes the emulator for tests.
func (p *Predictor) Shadow() *vt.Terminal { return p.shadow }
