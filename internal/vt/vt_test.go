package vt

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func feed(t *testing.T, term *Terminal, s string) {
	t.Helper()
	if _, err := term.Write([]byte(s)); err != nil {
		t.Fatal(err)
	}
}

// TestPrintWrapScroll pins basic text placement, wrapping and scrollback.
func TestPrintWrapScroll(t *testing.T) {
	term := New(10, 3)
	feed(t, term, "abcdefghijKL\r\nline2\r\nline3\r\nline4")
	if got := term.Line(0); got != "line2" {
		t.Fatalf("row0 %q", got)
	}
	if got := term.Line(2); got != "line4" {
		t.Fatalf("row2 %q", got)
	}
	sb := term.Scrollback()
	if len(sb) != 2 || lineText(sb[0]) != "abcdefghij" || lineText(sb[1]) != "KL" {
		t.Fatalf("scrollback %d %q", len(sb), lineText(sb[0]))
	}
	if c := term.Cursor(); c.Row != 2 || c.Col != 5 {
		t.Fatalf("cursor %+v", c)
	}
}

// TestPendingWrap checks the last-column behavior that prediction depends on.
func TestPendingWrap(t *testing.T) {
	term := New(5, 2)
	feed(t, term, "abcde")
	if c := term.Cursor(); c.Col != 4 || !c.Wrap || c.Row != 0 {
		t.Fatalf("after fill %+v", c)
	}
	feed(t, term, "\rX")
	if term.Line(0) != "Xbcde" || term.Cursor().Wrap {
		t.Fatalf("CR must clear pending wrap: %q", term.Line(0))
	}
}

// TestEraseAndAttributes checks ED/EL and SGR tracking including extended colors.
func TestEraseAndAttributes(t *testing.T) {
	term := New(20, 4)
	feed(t, term, "\x1b[1;31mred\x1b[0m \x1b[38;5;208mo\x1b[48:2::10:20:30mb\x1b[0m")
	if c := term.Cell(0, 0); !c.Attr.Bold || c.Attr.FG != (Color{Kind: ColorIndexed, Index: 1}) {
		t.Fatalf("red cell %+v", c)
	}
	if c := term.Cell(0, 4); c.Attr.FG != (Color{Kind: ColorIndexed, Index: 208}) {
		t.Fatalf("256 color %+v", c.Attr)
	}
	if c := term.Cell(0, 5); c.Attr.BG != (Color{Kind: ColorRGB, R: 10, G: 20, B: 30}) {
		t.Fatalf("rgb bg %+v", c.Attr)
	}
	feed(t, term, "\x1b[2;1Hsecond\x1b[1;2H\x1b[0J")
	if term.Line(0) != "r" || term.Line(1) != "" {
		t.Fatalf("ED 0: %q / %q", term.Line(0), term.Line(1))
	}
}

// TestAltScreenAndModes checks mode tracking that reattach must restore.
func TestAltScreenAndModes(t *testing.T) {
	term := New(10, 3)
	feed(t, term, "main\x1b[?1049h\x1b[?25l\x1b[?1000h\x1b[?1006h\x1b[?2004h\x1b[>1u\x1b[=7;1u\x1b[Halt")
	m := term.Modes()
	if !m.AltScreen || m.CursorVisible || m.Mouse != MouseNormal || m.MouseEnc != MouseEncSGR || !m.BracketedPaste || m.KittyFlags != 7 {
		t.Fatalf("modes %+v", m)
	}
	if term.Line(0) != "alt" {
		t.Fatalf("alt row %q", term.Line(0))
	}
	feed(t, term, "\x1b[<u\x1b[?1049l")
	m = term.Modes()
	if m.AltScreen || m.KittyFlags != 0 || term.Line(0) != "main" || term.Cursor().Col != 4 {
		t.Fatalf("after leave %+v %q %+v", m, term.Line(0), term.Cursor())
	}
}

// TestQueriesAnswered checks the responses the session gives while detached.
func TestQueriesAnswered(t *testing.T) {
	term := New(80, 24)
	var replies []string
	term.Reply = func(b []byte) { replies = append(replies, string(b)) }
	feed(t, term, "\x1b[3;5H\x1b[6n\x1b[c\x1b[?u\x1b[?2004$p\x1b]11;?\x1b\\\x1bP+q524742;5463\x1b\\\x1b[>q")
	want := []string{"\x1b[3;5R", "\x1b[?62;1;2;4;6;9;15;22c", "\x1b[?0u", "\x1b[?2004;2$y", "\x1b]11;rgb:1e1e/1e1e/1e1e\x1b\\", "\x1bP1+r524742;5463\x1b\\", "\x1bP>|aft 2.0\x1b\\"}
	if strings.Join(replies, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q\nwant %q", replies, want)
	}
	term.Reply = nil
	feed(t, term, "\x1b[6n") // no panic, no reply when downstream answers
}

// TestWideCharacters checks width-2 placement and the continuation cell.
func TestWideCharacters(t *testing.T) {
	term := New(6, 2)
	feed(t, term, "a你b")
	if term.Cell(0, 1).Rune != '你' || term.Cell(0, 1).Width != 2 || term.Cell(0, 2).Width != 0 || term.Cell(0, 3).Rune != 'b' {
		t.Fatalf("wide layout: %q", term.Line(0))
	}
	feed(t, term, "\x1b[1;2HX")
	if term.Cell(0, 2).Rune != ' ' {
		t.Fatal("overwriting half a wide char must blank the other half")
	}
}

// TestReverseCaret finds the drawn caret apps like pi use.
func TestReverseCaret(t *testing.T) {
	term := New(10, 2)
	feed(t, term, "\x1b[?25l> \x1b[7m \x1b[0m")
	r, c, ok := term.ReverseCell()
	if !ok || r != 0 || c != 2 {
		t.Fatalf("caret %d %d %v", r, c, ok)
	}
}

// TestRenderRoundTrip replays Render into a fresh terminal and expects the same
// screen, cursor and modes. It runs on recorded agent output, which exercises
// every sequence the real apps use.
func TestRenderRoundTrip(t *testing.T) {
	files, _ := filepath.Glob("testdata/*.bin")
	if len(files) == 0 {
		t.Skip("no recordings")
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		a := New(100, 30)
		a.Write(data)
		b := New(100, 30)
		b.Write(a.Render(RenderOptions{Scrollback: true, Title: true}))
		for r := 0; r < 30; r++ {
			ra, rb := a.Row(r), b.Row(r)
			for c := range ra {
				ra[c].Gen, rb[c].Gen = 0, 0
				if ra[c] != rb[c] {
					t.Errorf("%s row %d col %d: %+v != %+v", f, r, c, ra[c], rb[c])
					break
				}
			}
		}
		ca, cb := a.Cursor(), b.Cursor()
		ca.Wrap, cb.Wrap = false, false
		if ca != cb {
			t.Errorf("%s cursor %+v != %+v", f, ca, cb)
		}
		ma, mb := a.Modes(), b.Modes()
		ma.kittyStack, mb.kittyStack = nil, nil
		if fmt.Sprint(ma) != fmt.Sprint(mb) {
			t.Errorf("%s modes %+v != %+v", f, ma, mb)
		}
		if a.Title() != b.Title() {
			t.Errorf("%s title %q != %q", f, a.Title(), b.Title())
		}
	}
}

// TestAgainstPyte compares screen text and cursor with an independent emulator
// on recorded agent output. Lines containing wide characters are skipped because
// pyte counts them as one column.
func TestAgainstPyte(t *testing.T) {
	files, _ := filepath.Glob("testdata/*.json")
	for _, f := range files {
		var oracle struct {
			Display []string `json:"display"`
			Cursor  [2]int   `json:"cursor"`
			Hidden  bool     `json:"hidden"`
		}
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &oracle); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(strings.TrimSuffix(f, ".json") + ".bin")
		if err != nil {
			t.Fatal(err)
		}
		term := New(100, 30)
		term.Write(data)
		for r, want := range oracle.Display {
			want = strings.TrimRight(want, " ")
			if hasWide(want) {
				continue
			}
			if got := term.Line(r); got != want {
				t.Errorf("%s row %d:\n got %q\nwant %q", f, r, got, want)
			}
		}
		if c := term.Cursor(); c.Row != oracle.Cursor[0] || c.Col != oracle.Cursor[1] {
			t.Errorf("%s cursor got %d,%d want %v", f, c.Row, c.Col, oracle.Cursor)
		}
		if term.Modes().CursorVisible == oracle.Hidden {
			t.Errorf("%s cursor hidden: got %v want %v", f, !term.Modes().CursorVisible, oracle.Hidden)
		}
	}
}

func hasWide(s string) bool {
	for _, r := range s {
		if r > 0x1100 {
			return true
		}
	}
	return false
}

// TestOSC52Clipboard decodes an OSC 52 write to the callback and ignores a
// read when no reader is set.
func TestOSC52Clipboard(t *testing.T) {
	term := New(20, 3)
	var got []byte
	var gotSel string
	term.Clipboard = func(sel string, data []byte) { gotSel, got = sel, data }
	// "hello" base64 is aGVsbG8=
	feed(t, term, "\x1b]52;c;aGVsbG8=\x1b\\")
	if gotSel != "c" || string(got) != "hello" {
		t.Fatalf("clipboard write: sel=%q data=%q", gotSel, got)
	}
	// A read request with no reader set must not panic or reply.
	var replied bool
	term.Reply = func([]byte) { replied = true }
	feed(t, term, "\x1b]52;c;?\x1b\\")
	if replied {
		t.Fatal("answered a clipboard read with no reader set")
	}
	// With a reader, the read is answered as base64.
	term.ClipboardRead = func(string) []byte { return []byte("hi") }
	var reply string
	term.Reply = func(b []byte) { reply = string(b) }
	feed(t, term, "\x1b]52;c;?\x1b\\")
	if reply != "\x1b]52;c;aGk=\x1b\\" {
		t.Fatalf("clipboard read reply: %q", reply)
	}
}

// TestOSC52Chunked checks a clipboard write split across writes still decodes.
func TestOSC52Chunked(t *testing.T) {
	term := New(20, 3)
	var got []byte
	term.Clipboard = func(_ string, data []byte) { got = data }
	for _, part := range []string{"\x1b]52;c;aGVs", "bG8=", "\x1b\\"} {
		feed(t, term, part)
	}
	if string(got) != "hello" {
		t.Fatalf("chunked clipboard: %q", got)
	}
}

// TestSnapshot returns scrollback plus the screen with the cursor located in
// the combined buffer.
func TestSnapshot(t *testing.T) {
	term := New(10, 2)
	feed(t, term, "one\r\ntwo\r\nthree\r\nfourAB")
	snap := term.Snapshot()
	// Two lines scrolled off (one, two) plus two visible rows (three, fourAB).
	if snap.Cols != 10 || snap.Rows != 2 {
		t.Fatalf("size %d x %d", snap.Cols, snap.Rows)
	}
	if len(snap.Lines) != 4 {
		t.Fatalf("lines = %d, want 4", len(snap.Lines))
	}
	if LineText(snap.Lines[0]) != "one" || LineText(snap.Lines[3]) != "fourAB" {
		t.Fatalf("content: %q .. %q", LineText(snap.Lines[0]), LineText(snap.Lines[3]))
	}
	// Cursor is on the last visible row (index 3) after "fourAB".
	if snap.CursorRow != 3 || snap.CursorCol != 6 {
		t.Fatalf("cursor at %d,%d", snap.CursorRow, snap.CursorCol)
	}
}
