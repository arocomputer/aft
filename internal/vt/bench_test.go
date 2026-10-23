package vt

import (
	"os"
	"testing"
)

// BenchmarkFeed measures emulator parse throughput on recorded agent output,
// the work both the client shadow and the session do per byte of server output.
func BenchmarkFeed(b *testing.B) {
	data, err := os.ReadFile("testdata/opencode.bin")
	if err != nil {
		b.Skip("no recording")
	}
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		New(100, 30).Write(data)
	}
}

// BenchmarkRender measures repainting a screen to escape sequences, done on
// every reattach.
func BenchmarkRender(b *testing.B) {
	data, _ := os.ReadFile("testdata/opencode.bin")
	t := New(100, 30)
	t.Write(data)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		t.Render(RenderOptions{Scrollback: true})
	}
}

// BenchmarkSnapshot measures building a copy-mode snapshot of a full scrollback.
func BenchmarkSnapshot(b *testing.B) {
	t := New(100, 40)
	for i := 0; i < 5000; i++ {
		t.Write([]byte("a line of output with several words to fill the row\r\n"))
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		t.Snapshot()
	}
}
