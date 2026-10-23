package client

import (
	"io"
	"testing"
)

// BenchmarkPredictKey measures the real per-keystroke cost of drawing a
// prediction in steady typing, where each key is promptly confirmed by the
// server echo (here simulated by clearing the pending list). This is the work
// aft adds between your key and the bytes to the terminal: microseconds. Felt
// typing latency is the terminal and display, not aft.
func BenchmarkPredictKey(b *testing.B) {
	p := NewPredictor(80, 24, io.Discard, true)
	p.Output([]byte("\x1b[1;1H$ "))
	tok := Token{Kind: TokText, Rune: 'a', Bytes: []byte("a")}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.Key(tok)
		// Simulate the echo confirming, so the cursor stays at the prompt and
		// the pending list stays bounded, as in real typing.
		p.mu.Lock()
		p.pending = p.pending[:0]
		p.caretValid = false
		p.mu.Unlock()
	}
}
