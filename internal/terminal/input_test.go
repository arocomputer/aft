package terminal

import (
	"testing"
)

func kinds(toks []Token) []TokenKind {
	out := make([]TokenKind, len(toks))
	for i, t := range toks {
		out[i] = t.Kind
	}
	return out
}

// TestTokenizerSplitsSequences pins that escape sequences, pastes and
// multibyte characters are never mistaken for typed text, and that a split
// sequence waits for its remainder.
func TestTokenizerSplitsSequences(t *testing.T) {
	var z Tokenizer
	toks := z.Feed([]byte("a\x1b[Ab\x7f\x1b[200~pasted\x1b[201~é\x1b[<35;10;5M\x1b[127u"))
	want := []TokenKind{TokText, TokOther, TokText, TokBackspace, TokPaste, TokPaste, TokText, TokOther, TokBackspace}
	got := kinds(toks)
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("token %d: got %v want %v (%q)", i, got[i], want[i], toks[i].Bytes)
		}
	}
	if toks[6].Rune != 'é' {
		t.Fatalf("multibyte rune %q", toks[6].Rune)
	}
	// A CSI split across reads is held, then completed.
	if toks := z.Feed([]byte("\x1b[1;")); len(toks) != 0 || !z.Pending() {
		t.Fatalf("partial CSI should wait: %v", toks)
	}
	if toks := z.Feed([]byte("5R")); len(toks) != 1 || toks[0].Kind != TokOther || string(toks[0].Bytes) != "\x1b[1;5R" {
		t.Fatalf("completed CSI: %+v", toks)
	}
	// A lone Escape is released by Flush.
	z.Feed([]byte("\x1b"))
	if toks := z.Flush(); len(toks) != 1 || string(toks[0].Bytes) != "\x1b" {
		t.Fatalf("flush: %+v", toks)
	}
}

// TestTokenizerForwardsEveryByte checks nothing is lost or reordered.
func TestTokenizerForwardsEveryByte(t *testing.T) {
	var z Tokenizer
	in := "hello \x1b[D\x1bOP\x1b]11;rgb:1/2/3\x1b\\x\x1bq"
	var out []byte
	for _, tok := range z.Feed([]byte(in)) {
		out = append(out, tok.Bytes...)
	}
	for _, tok := range z.Flush() {
		out = append(out, tok.Bytes...)
	}
	if string(out) != in {
		t.Fatalf("bytes changed:\n got %q\nwant %q", out, in)
	}
}

// TestPasteKeys checks the keys that open the clipboard window: Ctrl-V and
// Cmd-V in legacy and kitty encodings, but not a plain v or Alt-V.
func TestPasteKeys(t *testing.T) {
	for _, k := range []string{"\x16", "\x1b[118;5u", "\x1b[118;9u", "\x1b[118;5:1u"} {
		if !isPasteKey([]byte(k)) {
			t.Errorf("%q should be a paste key", k)
		}
	}
	for _, k := range []string{"v", "\x1b[118;3u", "\x1b[118u"} {
		if isPasteKey([]byte(k)) {
			t.Errorf("%q is not a paste key", k)
		}
	}
}

// TestPasteArrivingInPieces finds the end marker when a paste comes in many
// reads, including a marker split across two of them, and keeps bytes
// after it as ordinary keys.
func TestPasteArrivingInPieces(t *testing.T) {
	var z Tokenizer
	var toks []Token
	for _, piece := range []string{"\x1b[200~one ", "two\x1b[2", "01~x"} {
		toks = append(toks, z.Feed([]byte(piece))...)
	}
	// The start marker is its own token; the content token ends with the end marker.
	if len(toks) != 3 || toks[1].Kind != TokPaste || string(toks[1].Bytes) != "one two\x1b[201~" || toks[2].Rune != 'x' {
		t.Fatalf("got %+v", toks)
	}
}
