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
// character goes after a predicted Backspace, both when it erases a typed
// character the server has echoed and when it takes back a prediction.
func TestBackspaceLeavesCursorOnErasedCell(t *testing.T) {
	var out bytes.Buffer
	p := NewPredictor(40, 5, &out, true)
	p.Output([]byte("\x1b[1;1H> "))
	typeKeys(p, "abc")
	p.Output([]byte("abc"))
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

// TestEnterLetsTheAppClearTheLine types into an Ink-style prompt and presses
// Enter: the app clears the line, and the typed text must not come back.
func TestEnterLetsTheAppClearTheLine(t *testing.T) {
	var out bytes.Buffer
	p := NewPredictor(40, 5, &out, true)
	p.Output([]byte("\x1b[?25l\x1b[1;1H> \x1b[7m \x1b[0m"))
	typeKeys(p, "abc")
	p.Key(Token{Kind: TokOther, Bytes: []byte("\r")}) // Enter
	p.Output([]byte("\r\x1b[2K> \x1b[7m \x1b[0m"))
	if s := screenOf(&out, 40, 5); s.Line(0) != ">" || len(p.pending) != 0 {
		t.Fatalf("after Enter: %q pending %d", s.Line(0), len(p.pending))
	}
}

// TestNothingIsDrawnAfterEnterUntilAnEcho types after Enter: the next line
// may be a password prompt, so the characters stay off the screen until the
// application echoes one, and are drawn again from then on.
func TestNothingIsDrawnAfterEnterUntilAnEcho(t *testing.T) {
	var out bytes.Buffer
	p := NewPredictor(40, 5, &out, true)
	p.Output([]byte("\x1b[1;1H$ "))
	typeKeys(p, "sudo")
	p.Output([]byte("sudo"))
	p.Key(Token{Kind: TokOther, Bytes: []byte("\r")})
	p.Output([]byte("\r\nPassword: "))
	typeKeys(p, "hunter2")
	if s := screenOf(&out, 40, 5); s.Line(1) != "Password:" {
		t.Fatalf("a password was drawn: %q", s.Line(1))
	}
	p.Output([]byte("\r\n$ "))
	p.Key(Token{Kind: TokOther, Bytes: []byte("\x1b[13u")}) // Enter under the kitty protocol, then a prompt that echoes
	typeKeys(p, "l")
	p.Output([]byte("l"))
	typeKeys(p, "s")
	if s := screenOf(&out, 40, 5); s.Line(2) != "$ ls" {
		t.Fatalf("after an echo the next key was not drawn: %q", s.Line(2))
	}
}

// TestKeysAfterAResetWaitForTheRepaint feeds keys typed during an outage
// right after Reset: nothing lands on the empty shadow, and the typed count
// does not let a Backspace erase what the repaint shows.
func TestKeysAfterAResetWaitForTheRepaint(t *testing.T) {
	var out bytes.Buffer
	p := NewPredictor(40, 5, &out, true)
	p.Output([]byte("\x1b[1;1H> "))
	typeKeys(p, "ab")
	p.Output([]byte("ab"))
	p.Reset()
	n := out.Len()
	typeKeys(p, "ls")
	p.Key(Token{Kind: TokBackspace})
	if out.Len() != n {
		t.Fatalf("drew %q on an empty shadow", out.String()[n:])
	}
}

// TestNoticeAfterAStreamStoppedMidSequence drops the sequence a dead stream
// left open, so the reconnecting notice can be drawn.
func TestNoticeAfterAStreamStoppedMidSequence(t *testing.T) {
	var out bytes.Buffer
	p := NewPredictor(40, 5, &out, true)
	p.Output([]byte("hello\x1b[3"))
	p.Notice("aft - reconnecting")
	if strings.Contains(out.String(), "reconnecting") {
		t.Fatal("a notice was drawn into an open sequence")
	}
	p.DropSequence()
	p.Notice("aft - reconnecting")
	if !strings.Contains(out.String(), "reconnecting") {
		t.Fatal("no notice after the sequence was dropped")
	}
}

// TestRepaintReplaysAnOpenSequence absorbs output that ends inside a sequence
// while the pager owns the screen; the repaint replays that start, so the
// rest of the sequence completes on the real terminal instead of printing.
func TestRepaintReplaysAnOpenSequence(t *testing.T) {
	var out bytes.Buffer
	p := NewPredictor(40, 5, &out, true)
	p.Output([]byte("\x1b[1;1Hhello"))
	p.Absorb([]byte("\x1b[2"))
	p.Repaint()
	p.Output([]byte("J"))
	if s := screenOf(&out, 40, 5); s.Line(0) != "" {
		t.Fatalf("the screen was not cleared: %q", s.Line(0))
	}
}

// TestNonEchoingKeyIsAMisprediction presses x in an app that does not echo
// it (vim's normal mode deletes a character instead): the redraw wins and
// no ghost x is drawn back.
func TestNonEchoingKeyIsAMisprediction(t *testing.T) {
	var out bytes.Buffer
	p := NewPredictor(40, 5, &out, true)
	p.Output([]byte("\x1b[1;1Hhello\x1b[1;1H"))
	typeKeys(p, "x")
	p.Output([]byte("\r\x1b[2Kello\x1b[1;1H"))
	if s := screenOf(&out, 40, 5); s.Line(0) != "ello" || p.confident {
		t.Fatalf("after delete: %q confident %v", s.Line(0), p.confident)
	}
}

// TestErasedSpaceWaitsForTheServer presses Backspace over a typed space,
// then the app redraws something else before handling the key: the erase
// stays in flight, so the next key goes where the space was.
func TestErasedSpaceWaitsForTheServer(t *testing.T) {
	var out bytes.Buffer
	p := NewPredictor(40, 5, &out, true)
	p.Output([]byte("\x1b[1;1H> "))
	typeKeys(p, "hi ")
	p.Output([]byte("hi "))
	p.Key(Token{Kind: TokBackspace})
	p.Output([]byte("\x1b[3;1Hstatus\x1b[1;6H"))
	typeKeys(p, "x")
	if s := screenOf(&out, 40, 5); s.Line(0) != "> hix" {
		t.Fatalf("got %q, want the x where the space was", s.Line(0))
	}
}

// TestCursorWaitsForUndrawnPredictions types while predictions are not being
// drawn: when trust comes back, the cursor stays with the text the screen
// shows instead of jumping over blank cells.
func TestCursorWaitsForUndrawnPredictions(t *testing.T) {
	var out bytes.Buffer
	p := NewPredictor(40, 5, &out, true)
	p.Output([]byte("\x1b[1;1H> "))
	p.confident = false
	typeKeys(p, "abc")
	p.Output([]byte("a"))
	if s := screenOf(&out, 40, 5); s.Cursor().Col != 3 {
		t.Fatalf("cursor at col %d over %q, want 3", s.Cursor().Col, s.Line(0))
	}
}

// TestNoticeKeepsTheAppsCursor draws a notice on the top row without the
// terminal's save and restore, which the application may be using itself,
// and puts the cursor back; RestoreRow takes the notice down.
func TestNoticeKeepsTheAppsCursor(t *testing.T) {
	var out bytes.Buffer
	p := NewPredictor(40, 5, &out, true)
	p.Output([]byte("\x1b[1;1Htop line\x1b[3;5H"))
	out.Reset()
	p.Notice("aft · reconnecting")
	if bytes.Contains(out.Bytes(), []byte("\x1b7")) || bytes.Contains(out.Bytes(), []byte("\x1b8")) {
		t.Fatal("notice used save and restore cursor")
	}
	p.RestoreRow(0)
	full := vt.New(40, 5)
	full.Write([]byte("\x1b[1;1Htop line\x1b[3;5H"))
	full.Write(out.Bytes())
	if full.Line(0) != "top line" || full.Cursor().Row != 2 {
		t.Fatalf("after: %q cursor %+v", full.Line(0), full.Cursor())
	}
}

// TestNoticeKeepsAPendingWrap draws a notice while the cursor waits at the
// last column to wrap: afterwards the next character still starts the next
// line, as it would have without the notice.
func TestNoticeKeepsAPendingWrap(t *testing.T) {
	var out bytes.Buffer
	p := NewPredictor(10, 5, &out, true)
	p.Output([]byte("\x1b[3;1H0123456789"))
	p.Notice("note")
	p.Output([]byte("X"))
	if s := screenOf(&out, 10, 5); s.Line(2) != "0123456789" || s.Line(3) != "X" {
		t.Fatalf("rows: %q / %q", s.Line(2), s.Line(3))
	}
}

// TestNoticeOnTheWrappingRow draws a notice while the cursor waits to wrap at
// the end of the top row: the notice fills all but the last column, and the
// next character still starts the second row.
func TestNoticeOnTheWrappingRow(t *testing.T) {
	var out bytes.Buffer
	p := NewPredictor(10, 5, &out, true)
	p.Output([]byte("\x1b[1;1H0123456789"))
	p.Notice("aft notice")
	s := screenOf(&out, 10, 5)
	if s.Line(0) != "aft notic9" {
		t.Fatalf("notice row: %q", s.Line(0))
	}
	p.Output([]byte("X"))
	if s := screenOf(&out, 10, 5); s.Line(1) != "X" {
		t.Fatalf("second row %q, want X", s.Line(1))
	}
}

// TestBackspaceErasesOnlyWhatWasTyped presses Backspace at an empty prompt:
// the prompt is the application's, so nothing is predicted and it stays.
// Once characters have been typed, Backspace is predicted over them, and
// only as many of them as were typed.
func TestBackspaceErasesOnlyWhatWasTyped(t *testing.T) {
	var out bytes.Buffer
	p := NewPredictor(40, 5, &out, true)
	p.Output([]byte("\x1b[1;1H> "))
	p.Key(Token{Kind: TokBackspace})
	if s := screenOf(&out, 40, 5); s.Line(0) != ">" {
		t.Fatalf("the prompt was erased: %q", s.Line(0))
	}
	typeKeys(p, "ab")
	p.Output([]byte("ab")) // echoed
	for i := 0; i < 3; i++ {
		p.Key(Token{Kind: TokBackspace})
	}
	if s := screenOf(&out, 40, 5); s.Line(0) != ">" || s.Cursor().Col != 2 {
		t.Fatalf("after three Backspaces over two typed characters: %q cursor col %d", s.Line(0), s.Cursor().Col)
	}
}

// TestAbsorbKeepsOnlyTheOpenTail replays, after a repaint, just the start of
// what was left open across chunks: a sequence or a multibyte character.
func TestAbsorbKeepsOnlyTheOpenTail(t *testing.T) {
	var out bytes.Buffer
	p := NewPredictor(40, 5, &out, true)
	p.Output([]byte("\x1b[1;1Hx"))
	p.Absorb([]byte("ab\x1b[2;1Hcd\x1b[3"))
	p.Absorb([]byte(";1"))
	if string(p.tail) != "\x1b[3;1" {
		t.Fatalf("tail %q", p.tail)
	}
	p.Absorb([]byte("Hé"[:2])) // completes the sequence, then the first byte of é
	if string(p.tail) != "\xc3" {
		t.Fatalf("tail after a split character: %q", p.tail)
	}
}

// TestNoticeInGraphicsMode draws the notice in plain letters while the
// application has the graphics set shifted in, and leaves that set shifted
// in afterwards.
func TestNoticeInGraphicsMode(t *testing.T) {
	var out bytes.Buffer
	p := NewPredictor(40, 5, &out, true)
	p.Output([]byte("\x1b[2;1Hlq\x1b(0"))
	p.Notice("aft - reconnecting")
	s := screenOf(&out, 40, 5)
	if s.Line(0) != "aft - reconnecting" {
		t.Fatalf("notice drawn as %q", s.Line(0))
	}
	p.Output([]byte("q"))
	if s := screenOf(&out, 40, 5); s.Line(1) != "lq─" {
		t.Fatalf("the application's charset was not restored: %q", s.Line(1))
	}
}
