package terminal

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/arocomputer/aft/internal/vt"
)

// screenOf replays everything the predictor wrote into a reference terminal,
// which is exactly what the user's terminal would show.
func screenOf(out *bytes.Buffer, cols, rows int) *vt.Terminal {
	t := vt.New(cols, rows)
	t.Write(out.Bytes())
	return t
}

// TestPredictionDrawsThenConfirms pins the core promise: a typed character is
// visible before the server echoes it, and the echo leaves the screen exactly
// as if there had been no prediction.
func TestPredictionDrawsThenConfirms(t *testing.T) {
	var out bytes.Buffer
	p := NewPredictor(40, 5, &out, true)
	p.Output([]byte("\x1b[2;1H> ")) // a prompt with the cursor after it
	p.Key(Token{Kind: TokText, Rune: 'h', Bytes: []byte("h")})
	p.Key(Token{Kind: TokText, Rune: 'i', Bytes: []byte("i")})
	if s := screenOf(&out, 40, 5); s.Line(1) != "> hi" {
		t.Fatalf("prediction not visible: %q", s.Line(1))
	}
	if p.Shadow().Line(1) != ">" {
		t.Fatal("shadow must not contain predictions")
	}
	// The server echoes exactly what was predicted.
	p.Output([]byte("hi"))
	s := screenOf(&out, 40, 5)
	if s.Line(1) != "> hi" || s.Cursor().Col != 4 {
		t.Fatalf("after echo: %q cursor %+v", s.Line(1), s.Cursor())
	}
	if len(p.pending) != 0 || !p.confident {
		t.Fatalf("pending %d confident %v", len(p.pending), p.confident)
	}
}

// TestCursorRestoredBeforeServerOutput checks that server output landing after
// an overlay goes where the server intended, not where the overlay left the
// cursor.
func TestCursorRestoredBeforeServerOutput(t *testing.T) {
	var out bytes.Buffer
	p := NewPredictor(40, 5, &out, true)
	p.Output([]byte("\x1b[1;1Hprompt: "))
	p.Key(Token{Kind: TokText, Rune: 'x', Bytes: []byte("x")})
	// The server redraws the status line elsewhere without echoing yet,
	// using relative output that depends on the cursor position.
	p.Output([]byte("\x1b[3;1Hstatus\x1b[1;9H"))
	s := screenOf(&out, 40, 5)
	if s.Line(2) != "status" || s.Line(0) != "prompt: x" {
		t.Fatalf("rows: %q / %q", s.Line(0), s.Line(2))
	}
	if c := p.Shadow().Cursor(); c.Row != 0 || c.Col != 8 {
		t.Fatalf("shadow cursor %+v", c)
	}
}

// TestWrongPredictionRepaints checks that a prediction the server never
// confirms is removed from the screen and lowers confidence.
func TestWrongPredictionRepaints(t *testing.T) {
	var out bytes.Buffer
	p := NewPredictor(40, 5, &out, true)
	p.Output([]byte("\x1b[1;1H> "))
	p.Key(Token{Kind: TokText, Rune: 'z', Bytes: []byte("z")})
	if screenOf(&out, 40, 5).Line(0) != "> z" {
		t.Fatal("prediction should be drawn")
	}
	// Age the prediction past any timeout, then tick.
	p.pending[0].at = time.Now().Add(-10 * time.Second)
	p.Tick()
	if s := screenOf(&out, 40, 5); s.Line(0) != ">" {
		t.Fatalf("stale prediction not repainted: %q", s.Line(0))
	}
	if p.confident {
		t.Fatal("confidence should drop after a miss")
	}
	// With confidence gone, the next key is forwarded but not drawn; once
	// the server echoes it, confidence returns.
	p.Key(Token{Kind: TokText, Rune: 'a', Bytes: []byte("a")})
	if screenOf(&out, 40, 5).Line(0) != ">" {
		t.Fatal("no prediction should be drawn while unconfident")
	}
	p.Output([]byte("a"))
	if !p.confident {
		t.Fatal("confidence should return after a matching echo")
	}
}

// TestBackspaceUndoesPrediction checks Backspace erases a predicted character
// and that the server's eventual redraw agrees.
func TestBackspaceUndoesPrediction(t *testing.T) {
	var out bytes.Buffer
	p := NewPredictor(40, 5, &out, true)
	p.Output([]byte("\x1b[1;1H> "))
	p.Key(Token{Kind: TokText, Rune: 'a', Bytes: []byte("a")})
	p.Key(Token{Kind: TokText, Rune: 'b', Bytes: []byte("b")})
	p.Key(Token{Kind: TokBackspace, Bytes: []byte{0x7f}})
	if s := screenOf(&out, 40, 5).Line(0); s != "> a" {
		t.Fatalf("after backspace: %q", s)
	}
	p.Output([]byte("a"))
	if s := screenOf(&out, 40, 5).Line(0); s != "> a" {
		t.Fatalf("after echo: %q", s)
	}
}

// TestDrawnCaret covers apps like pi that hide the cursor and draw a reverse
// block as the caret.
func TestDrawnCaret(t *testing.T) {
	var out bytes.Buffer
	p := NewPredictor(40, 5, &out, true)
	p.Output([]byte("\x1b[?25l\x1b[1;1H> \x1b[7m \x1b[0m"))
	p.Key(Token{Kind: TokText, Rune: 'q', Bytes: []byte("q")})
	s := screenOf(&out, 40, 5)
	if s.Line(0) != "> q" {
		t.Fatalf("drawn-caret prediction: %q", s.Line(0))
	}
	if r, c, ok := s.ReverseCell(); !ok || r != 0 || c != 3 {
		t.Fatalf("caret block should move right: %d %d %v", r, c, ok)
	}
	if !strings.Contains(out.String(), "q") {
		t.Fatal("character not written")
	}
}

// TestNoPredictionWhenDisabled checks pure passthrough.
func TestNoPredictionWhenDisabled(t *testing.T) {
	var out bytes.Buffer
	p := NewPredictor(40, 5, &out, false)
	p.Output([]byte("abc"))
	p.Key(Token{Kind: TokText, Rune: 'x', Bytes: []byte("x")})
	if out.String() != "abc" {
		t.Fatalf("passthrough changed output: %q", out.String())
	}
}

// TestPredictionIsSynchronous pins item 2's core claim: a predicted character
// is written before Key returns, with no wait on the network, so typing is
// drawn at local speed regardless of round-trip time.
func TestPredictionIsSynchronous(t *testing.T) {
	var out bytes.Buffer
	p := NewPredictor(40, 5, &out, true)
	p.Output([]byte("\x1b[1;1H> "))
	start := time.Now()
	p.Key(Token{Kind: TokText, Rune: 'x', Bytes: []byte("x")})
	elapsed := time.Since(start)
	if out.Len() == 0 {
		t.Fatal("Key produced no output; prediction was not drawn synchronously")
	}
	if elapsed > 2*time.Millisecond {
		t.Fatalf("Key took %v; prediction must be local-speed", elapsed)
	}
}

// TestPredictionMinimalBytes checks that steady typing emits a bare character
// per key after the first, like native echo, with no cursor-move or style bytes.
func TestPredictionMinimalBytes(t *testing.T) {
	var out bytes.Buffer
	p := NewPredictor(40, 5, &out, true)
	p.Output([]byte("\x1b[1;1H$ ")) // prompt, cursor after it, default pen
	out.Reset()
	p.Key(Token{Kind: TokText, Rune: 'a', Bytes: []byte("a")})
	first := out.Len()
	out.Reset()
	p.Key(Token{Kind: TokText, Rune: 'b', Bytes: []byte("b")})
	p.Key(Token{Kind: TokText, Rune: 'c', Bytes: []byte("c")})
	rest := out.String()
	if rest != "bc" {
		t.Fatalf("steady typing should emit bare characters, got %q", rest)
	}
	if first > 6 {
		t.Fatalf("first keystroke emitted %d bytes, expected a short pen+char", first)
	}
	// The reference terminal still shows the right line.
	if screenOf(&out, 40, 5); p.Shadow().Line(0) != "$" {
		t.Fatalf("shadow unexpectedly changed: %q", p.Shadow().Line(0))
	}
}

func typeKeys(p *Predictor, s string) {
	for _, r := range s {
		p.Key(Token{Kind: TokText, Rune: r, Bytes: []byte(string(r))})
	}
}

// TestCaretRunsAheadOfTrailingEchoes pins the fix for the caret snapping
// back: a key typed while earlier echoes are still arriving goes after the
// earlier predictions, not at the server's lagging cursor.
func TestCaretRunsAheadOfTrailingEchoes(t *testing.T) {
	var out bytes.Buffer
	p := NewPredictor(40, 5, &out, true)
	p.Output([]byte("\x1b[1;1H> "))
	typeKeys(p, "abc")
	p.Output([]byte("a")) // only the first echo has arrived
	typeKeys(p, "d")
	if s := screenOf(&out, 40, 5); s.Line(0) != "> abcd" || s.Cursor().Col != 6 {
		t.Fatalf("mid-echo: %q cursor col %d", s.Line(0), s.Cursor().Col)
	}
	p.Output([]byte("bcd"))
	if s := screenOf(&out, 40, 5); s.Line(0) != "> abcd" || len(p.pending) != 0 || !p.confident {
		t.Fatalf("after echo: %q pending %d confident %v", s.Line(0), len(p.pending), p.confident)
	}
}

// TestLineRedrawKeepsInflightPredictions covers Ink-style agents, which hide
// the cursor, draw a reverse-video caret and rewrite the whole input line on
// each echo. Predictions the server has not reached survive the redraw, and
// the drawn caret stays at their end.
func TestLineRedrawKeepsInflightPredictions(t *testing.T) {
	var out bytes.Buffer
	p := NewPredictor(40, 5, &out, true)
	p.Output([]byte("\x1b[?25l\x1b[1;1H> \x1b[7m \x1b[0m"))
	typeKeys(p, "abc")
	p.Output([]byte("\r\x1b[2K> a\x1b[7m \x1b[0m")) // echo of "a" only
	s := screenOf(&out, 40, 5)
	if s.Line(0) != "> abc" || !p.confident {
		t.Fatalf("after redraw: %q confident %v", s.Line(0), p.confident)
	}
	if r, c, ok := s.ReverseCell(); !ok || r != 0 || c != 5 {
		t.Fatalf("drawn caret at %d,%d ok=%v, want 0,5", r, c, ok)
	}
	p.Output([]byte("\r\x1b[2K> abc\x1b[7m \x1b[0m"))
	if s := screenOf(&out, 40, 5); s.Line(0) != "> abc" || len(p.pending) != 0 {
		t.Fatalf("after echo: %q pending %d", s.Line(0), len(p.pending))
	}
}

// TestRedrawKeepsCursorAtPredictions covers an app that shows the terminal
// cursor and redraws other parts of the screen on its own clock, as Claude
// Code's classic renderer does: each redraw ends by putting the cursor where
// the server's input ends, which trails what was typed. The cursor must stay
// after the predictions, not jump back to the server's position.
func TestRedrawKeepsCursorAtPredictions(t *testing.T) {
	var out bytes.Buffer
	p := NewPredictor(40, 5, &out, true)
	p.Output([]byte("\x1b[2;1H> "))
	typeKeys(p, "abc")
	// A status line redraw two rows down, then back to the server's caret.
	p.Output([]byte("\x1b[4;1H\x1b[2K-- status --\x1b[2;3H"))
	s := screenOf(&out, 40, 5)
	if s.Line(1) != "> abc" || s.Cursor().Row != 1 || s.Cursor().Col != 5 {
		t.Fatalf("after redraw: %q cursor %+v, want the cursor at 1,5", s.Line(1), s.Cursor())
	}
	// The echoes arrive; nothing is left over.
	p.Output([]byte("abc"))
	s = screenOf(&out, 40, 5)
	if s.Line(1) != "> abc" || s.Cursor().Col != 5 || len(p.pending) != 0 {
		t.Fatalf("after echo: %q cursor %+v pending %d", s.Line(1), s.Cursor(), len(p.pending))
	}
}

// TestSkippedSpaceIsConfirmed covers renderers that echo a typed space by
// moving the cursor over the blank cell instead of printing it, as Claude
// Code's does. Once the server's cursor has passed the cell, the space is
// confirmed; left pending, it would expire and pull the cursor back to it.
func TestSkippedSpaceIsConfirmed(t *testing.T) {
	var out bytes.Buffer
	p := NewPredictor(40, 5, &out, true)
	p.Output([]byte("\x1b[1;1H> hi"))
	typeKeys(p, " x")
	p.Output([]byte("\x1b[1C" + "x")) // over the space, then the x
	if len(p.pending) != 0 || !p.confident {
		t.Fatalf("pending %d confident %v", len(p.pending), p.confident)
	}
}

// TestExpiryLeavesCursorInPlace lets a prediction the server never answers
// expire: its cell is repainted from the server's screen, and the cursor goes
// back where the server has it rather than staying on the repainted cell.
func TestExpiryLeavesCursorInPlace(t *testing.T) {
	var out bytes.Buffer
	p := NewPredictor(40, 5, &out, true)
	p.Output([]byte("\x1b[1;1H> "))
	typeKeys(p, "a")
	p.pending[0].at = time.Now().Add(-time.Hour)
	p.Tick()
	s := screenOf(&out, 40, 5)
	if s.Line(0) != ">" || s.Cursor().Col != 2 {
		t.Fatalf("after expiry: %q cursor col %d, want 2", s.Line(0), s.Cursor().Col)
	}
}

// TestBackspaceLeavesCursorOnErasedCell keeps the cursor where the next
// character goes after a predicted Backspace, both when it erases a server
// character and when it takes back a prediction.
func TestBackspaceLeavesCursorOnErasedCell(t *testing.T) {
	var out bytes.Buffer
	p := NewPredictor(40, 5, &out, true)
	p.Output([]byte("\x1b[1;1H> abc"))
	p.Key(Token{Kind: TokBackspace})
	if s := screenOf(&out, 40, 5); s.Line(0) != "> ab" || s.Cursor().Col != 4 {
		t.Fatalf("erasing: %q cursor col %d, want 4", s.Line(0), s.Cursor().Col)
	}
	typeKeys(p, "x")
	p.Key(Token{Kind: TokBackspace})
	if s := screenOf(&out, 40, 5); s.Line(0) != "> ab" || s.Cursor().Col != 4 {
		t.Fatalf("taking back x: %q cursor col %d, want 4", s.Line(0), s.Cursor().Col)
	}
}
