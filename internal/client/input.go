package client

import (
	"unicode/utf8"
)

// TokenKind classifies terminal input for the predictor.
type TokenKind uint8

const (
	// TokText is one printable character typed by the user.
	TokText TokenKind = iota
	// TokBackspace erases the character before the caret.
	TokBackspace
	// TokOther is anything else: control keys, escape sequences, terminal
	// replies and mouse reports. It is forwarded and never predicted.
	TokOther
	// TokPaste is bracketed-paste content including its markers.
	TokPaste
)

// Token is a unit of input. Bytes is exactly what the terminal sent and is
// forwarded unchanged; Rune is set for TokText.
type Token struct {
	Kind  TokenKind
	Bytes []byte
	Rune  rune
}

// Tokenizer splits raw terminal input into tokens. Escape sequences may arrive
// split across reads, so it keeps an incomplete tail until more bytes arrive or
// the caller gives up waiting with Flush.
type Tokenizer struct {
	buf     []byte
	pasting bool
}

var pasteStart = []byte("\x1b[200~")
var pasteEnd = []byte("\x1b[201~")

// Feed adds input and returns the complete tokens it contains.
func (z *Tokenizer) Feed(b []byte) []Token {
	z.buf = append(z.buf, b...)
	var out []Token
	for len(z.buf) > 0 {
		n, tok, complete := z.next()
		if !complete {
			break
		}
		tok.Bytes = append([]byte(nil), z.buf[:n]...)
		z.buf = z.buf[n:]
		out = append(out, tok)
	}
	return out
}

// Flush returns whatever is buffered as a single TokOther. Callers use it when
// no more input has arrived for a short time, so a lone Escape key is sent.
func (z *Tokenizer) Flush() []Token {
	if len(z.buf) == 0 {
		return nil
	}
	tok := Token{Kind: TokOther, Bytes: append([]byte(nil), z.buf...)}
	z.buf = z.buf[:0]
	return []Token{tok}
}

// Pending reports whether an incomplete sequence is buffered.
func (z *Tokenizer) Pending() bool { return len(z.buf) > 0 }

// next decodes one token from the front of the buffer. It returns the byte
// count, the token, and whether the token is complete.
func (z *Tokenizer) next() (int, Token, bool) {
	b := z.buf
	if z.pasting {
		if i := indexOf(b, pasteEnd); i >= 0 {
			z.pasting = false
			return i + len(pasteEnd), Token{Kind: TokPaste}, true
		}
		// Hold paste content until the end marker; a very large paste still
		// flows because the marker always arrives.
		if len(b) > 1<<20 {
			return len(b), Token{Kind: TokPaste}, true
		}
		return 0, Token{}, false
	}
	c := b[0]
	switch {
	case c == 0x7F || c == 0x08:
		return 1, Token{Kind: TokBackspace}, true
	case c == 0x1B:
		return z.escape(b)
	case c < 0x20:
		return 1, Token{Kind: TokOther}, true
	case c < 0x80:
		return 1, Token{Kind: TokText, Rune: rune(c)}, true
	}
	if !utf8.FullRune(b) {
		return 0, Token{}, false
	}
	r, size := utf8.DecodeRune(b)
	if r == utf8.RuneError {
		return size, Token{Kind: TokOther}, true
	}
	return size, Token{Kind: TokText, Rune: r}, true
}

// escape decodes a sequence starting with ESC.
func (z *Tokenizer) escape(b []byte) (int, Token, bool) {
	if len(b) < 2 {
		return 0, Token{}, false
	}
	switch b[1] {
	case '[':
		if hasPrefix(b, pasteStart) {
			z.pasting = true
			return len(pasteStart), Token{Kind: TokPaste}, true
		}
		// CSI: parameters and intermediates, then a final byte in 0x40..0x7E.
		for i := 2; i < len(b); i++ {
			if b[i] >= 0x40 && b[i] <= 0x7E {
				tok := Token{Kind: TokOther}
				if b[i] == 'u' && isKittyBackspace(b[2:i]) {
					tok.Kind = TokBackspace
				}
				return i + 1, tok, true
			}
			if b[i] < 0x20 || b[i] > 0x3F {
				// Not a valid CSI byte: treat ESC [ as its own token.
				return i, Token{Kind: TokOther}, true
			}
		}
		if len(b) > 64 {
			return len(b), Token{Kind: TokOther}, true
		}
		return 0, Token{}, false
	case 'O':
		if len(b) < 3 {
			return 0, Token{}, false
		}
		return 3, Token{Kind: TokOther}, true
	case ']', 'P', '^', '_', 'X':
		// String sequences end with BEL (OSC only) or ESC \.
		for i := 2; i < len(b); i++ {
			if b[i] == 0x07 && b[1] == ']' {
				return i + 1, Token{Kind: TokOther}, true
			}
			if b[i] == 0x1B && i+1 < len(b) && b[i+1] == '\\' {
				return i + 2, Token{Kind: TokOther}, true
			}
		}
		if len(b) > 1<<16 {
			return len(b), Token{Kind: TokOther}, true
		}
		return 0, Token{}, false
	default:
		// ESC + one byte: Alt-key or a two-byte sequence.
		if b[1] >= 0x80 {
			if !utf8.FullRune(b[1:]) {
				return 0, Token{}, false
			}
			_, size := utf8.DecodeRune(b[1:])
			return 1 + size, Token{Kind: TokOther}, true
		}
		return 2, Token{Kind: TokOther}, true
	}
}

// isKittyBackspace matches the kitty keyboard protocol encoding of Backspace,
// CSI 127 u, with or without a "1" (no modifier) field.
func isKittyBackspace(params []byte) bool {
	s := string(params)
	return s == "127" || s == "127;1" || s == "127;1:1"
}

func hasPrefix(b, p []byte) bool {
	return len(b) >= len(p) && string(b[:len(p)]) == string(p)
}

func indexOf(b, p []byte) int {
	for i := 0; i+len(p) <= len(b); i++ {
		if string(b[i:i+len(p)]) == string(p) {
			return i
		}
	}
	return -1
}
