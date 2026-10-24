package terminal

import (
	"strings"
	"testing"

	"github.com/arocomputer/aft/internal/vt"
)

func cellsOf(s string) []vt.Cell {
	var row []vt.Cell
	for _, r := range s {
		row = append(row, vt.Cell{Rune: r, Width: 1})
	}
	return row
}

func snapOf(lines ...string) vt.SnapshotData {
	sd := vt.SnapshotData{Cols: 40, Rows: 5}
	for _, l := range lines {
		sd.Lines = append(sd.Lines, cellsOf(l))
	}
	return sd
}

func text(r rune) Token    { return Token{Kind: TokText, Rune: r, Bytes: []byte(string(r))} }
func other(s string) Token { return Token{Kind: TokOther, Bytes: []byte(s)} }

// TestCopyModeStartsAtBottom pins that the pager opens on the newest line.
func TestCopyModeStartsAtBottom(t *testing.T) {
	c := newCopyMode(snapOf("one", "two", "three", "four", "five", "six"), 40, 5)
	if c.cursor != 5 {
		t.Fatalf("cursor = %d, want 5 (last line)", c.cursor)
	}
	// Height is rows-1 = 4, so the top line keeps the cursor visible.
	if c.top != 2 {
		t.Fatalf("top = %d, want 2", c.top)
	}
}

// TestCopyModeSearch finds a match toward older output and repeats it.
func TestCopyModeSearch(t *testing.T) {
	c := newCopyMode(snapOf("error here", "ok", "error again", "ok", "ok"), 40, 5)
	// Type "/error" then Enter.
	for _, tok := range []Token{text('/'), text('e'), text('r'), text('r'), text('o'), text('r'), other("\r")} {
		if exit, _ := c.key(tok); exit {
			t.Fatal("search should not exit copy mode")
		}
	}
	if c.cursor != 2 {
		t.Fatalf("first match cursor = %d, want 2", c.cursor)
	}
	c.key(text('n')) // next, toward older
	if c.cursor != 0 {
		t.Fatalf("second match cursor = %d, want 0", c.cursor)
	}
}

// TestCopyModeSelectionCopies yanks a line-wise selection as joined text.
func TestCopyModeSelectionCopies(t *testing.T) {
	c := newCopyMode(snapOf("alpha", "beta", "gamma", "delta"), 40, 5)
	c.cursor = 1 // beta
	c.key(text('v'))
	c.key(text('j')) // extend to gamma
	exit, copied := c.key(text('y'))
	if !exit {
		t.Fatal("y should exit copy mode")
	}
	if string(copied) != "beta\ngamma" {
		t.Fatalf("copied = %q, want %q", copied, "beta\ngamma")
	}
}

// TestCopyModeViewHasStatusAndText renders a frame with the content and bar.
func TestCopyModeViewHasStatusAndText(t *testing.T) {
	c := newCopyMode(snapOf("hello world", "second line"), 40, 5)
	out := string(c.view())
	if !strings.Contains(out, "hello world") || !strings.Contains(out, "second line") {
		t.Fatalf("view missing content:\n%q", out)
	}
	if !strings.Contains(out, "COPY") || !strings.Contains(out, "1/2") && !strings.Contains(out, "2/2") {
		t.Fatalf("view missing status bar:\n%q", out)
	}
	if !strings.Contains(out, "\x1b[?2026h") || !strings.Contains(out, "\x1b[?2026l") {
		t.Fatal("view should be a synchronized update")
	}
}

// TestCopyModeQuitAndEscape exit without copying.
func TestCopyModeQuitAndEscape(t *testing.T) {
	c := newCopyMode(snapOf("x"), 40, 5)
	if exit, copied := c.key(text('q')); !exit || copied != nil {
		t.Fatalf("q: exit=%v copied=%q", exit, copied)
	}
	if exit, _ := c.key(other("\x1b")); !exit {
		t.Fatal("Escape should exit")
	}
}
