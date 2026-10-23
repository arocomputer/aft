package proto

import (
	"bufio"
	"bytes"
	"testing"
)

// BenchmarkFrame measures encoding and decoding one wire frame, the per-message
// cost on both ends of the connection.
func BenchmarkFrame(b *testing.B) {
	payload := []byte("a typical chunk of terminal output from the server side")
	var buf bytes.Buffer
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := Write(&buf, Output, payload); err != nil {
			b.Fatal(err)
		}
		if _, err := Read(bufio.NewReader(&buf)); err != nil {
			b.Fatal(err)
		}
	}
}
