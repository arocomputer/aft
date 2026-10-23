package client

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/fschrhunt/aft/internal/vt"
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
