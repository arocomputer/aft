package terminal

import (
	"io"
	"time"
)

// laggyWriter and laggyReader add a fixed delay in each direction so the
// selftest can simulate network latency and let a person feel how prediction
// hides it. They are used only when Options.LatencyMS is set.
type laggyWriter struct {
	w io.Writer
	d time.Duration
}

func (l laggyWriter) Write(b []byte) (int, error) {
	time.Sleep(l.d)
	return l.w.Write(b)
}

// laggyReader returns a reader that delivers r's data delayed by d, preserving
// order. It runs a goroutine that copies through an in-process pipe.
func laggyReader(r io.Reader, d time.Duration) io.Reader {
	pr, pw := io.Pipe()
	go func() {
		buf := make([]byte, 32<<10)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				time.Sleep(d)
				if _, werr := pw.Write(buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				pw.CloseWithError(err)
				return
			}
		}
	}()
	return pr
}
