package install

import (
	"runtime"
	"testing"
)

// TestBlobDecompresses checks the embedded, gzipped server for this platform
// decompresses to a non-empty binary with the right executable magic. It guards
// the embed-and-compress install path: a missing blob or a bad gzip breaks every
// remote install, and the unit tests otherwise never exercise decompression.
func TestBlobDecompresses(t *testing.T) {
	b, err := Blob(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatalf("Blob(%s/%s): %v (run ./x servers)", runtime.GOOS, runtime.GOARCH, err)
	}
	if len(b) < 1<<20 {
		t.Fatalf("server for %s/%s is only %d bytes", runtime.GOOS, runtime.GOARCH, len(b))
	}
	switch runtime.GOOS {
	case "linux":
		if string(b[:4]) != "\x7fELF" {
			t.Fatalf("linux server is not an ELF binary: %x", b[:4])
		}
	case "darwin":
		// Mach-O: 0xfeedfacf (64-bit) or 0xcafebabe (universal), little-endian.
		m := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
		if m != 0xcffaedfe && m != 0xfeedfacf && m != 0xcafebabe && m != 0xbebafeca {
			t.Fatalf("darwin server has unexpected Mach-O magic: %x", b[:4])
		}
	}
}

// TestBlobUnknownPlatform reports a clear error rather than an empty binary.
func TestBlobUnknownPlatform(t *testing.T) {
	if _, err := Blob("plan9", "sparc"); err == nil {
		t.Fatal("expected an error for an unsupported platform")
	}
}
