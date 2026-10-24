package main

import (
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/creack/pty"
)

// BenchmarkTypingLatency measures how long a typed character takes to appear,
// end to end through the real client, over a simulated distant link (120 ms
// round trip). The "predicted" case should stay far below the link delay; the
// "raw" case tracks the round trip. ns/op is the echo latency. This is the
// headline client measurement: it is what makes typing feel local.
//
// It drives the test binary as a local session (no network), reads the pty at
// the byte level for precision, and reuses one session across iterations.
func BenchmarkTypingLatency(b *testing.B) {
	if os.Getenv("AFT_TEST_AS_AFT") == "1" {
		b.Skip("child process")
	}
	const oneWayMS = 60 // 120 ms round trip
	for _, predict := range []bool{true, false} {
		name := "predicted"
		if !predict {
			name = "raw"
		}
		b.Run(name, func(b *testing.B) {
			fd, done, kill := startLatencySession(b, oneWayMS, predict)
			defer kill()
			seen := byteStream(fd, done)
			drainBytes(seen, 1500*time.Millisecond) // let the prompt settle

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := fd.Write([]byte("x")); err != nil {
					b.Fatal(err)
				}
				if !waitByte(seen, 'x', 3*time.Second) {
					b.Fatal("echo never arrived")
				}
				b.StopTimer()
				if i%40 == 39 {
					fd.Write([]byte{0x15}) // Ctrl-U clears the line so it never wraps
					drainBytes(seen, 50*time.Millisecond)
				}
				b.StartTimer()
			}
		})
	}
}

// startLatencySession launches the test binary as an aft local session with the
// given simulated one-way delay and prediction setting, under a pseudo-terminal.
func startLatencySession(b *testing.B, oneWayMS int, predict bool) (fd *os.File, done <-chan struct{}, kill func()) {
	b.Helper()
	self, err := os.Executable()
	if err != nil {
		b.Fatal(err)
	}
	// A short home: Unix socket paths are length-limited, and the default temp
	// dir on macOS is long enough to overflow a session's socket path.
	home, err := os.MkdirTemp("/tmp", "aft-bench-")
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { os.RemoveAll(home) })
	cmd := exec.Command(self, "local")
	cmd.Env = append(os.Environ(),
		"AFT_TEST_AS_AFT=1",
		"AFT_HOME="+home,
		"TERM=xterm-256color",
		"SHELL=/bin/sh",
		"AFT_LATENCY_MS="+strconv.Itoa(oneWayMS),
	)
	if !predict {
		cmd.Env = append(cmd.Env, "AFT_PREDICT=0")
	}
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: 80, Rows: 24})
	if err != nil {
		b.Fatal(err)
	}
	d := make(chan struct{})
	go func() { cmd.Wait(); close(d) }()
	return ptmx, d, func() { cmd.Process.Kill(); ptmx.Close() }
}

// byteStream delivers pty output one byte at a time until done.
func byteStream(fd *os.File, done <-chan struct{}) <-chan byte {
	ch := make(chan byte, 8192)
	go func() {
		buf := make([]byte, 8192)
		for {
			n, err := fd.Read(buf)
			for i := 0; i < n; i++ {
				select {
				case ch <- buf[i]:
				default:
				}
			}
			if err != nil {
				return
			}
		}
	}()
	return ch
}

func drainBytes(seen <-chan byte, idle time.Duration) {
	t := time.NewTimer(idle)
	defer t.Stop()
	for {
		select {
		case <-seen:
			t.Reset(idle)
		case <-t.C:
			return
		}
	}
}

func waitByte(seen <-chan byte, b byte, timeout time.Duration) bool {
	deadline := time.After(timeout)
	for {
		select {
		case c := <-seen:
			if c == b {
				return true
			}
		case <-deadline:
			return false
		}
	}
}
