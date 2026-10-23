// Command latency measures how long a typed character takes to appear through
// the real aft client, with prediction on and off, across simulated link
// delays. It drives bin/aft as a local session under a pseudo-terminal, so it
// exercises the whole client, and prints a Markdown table.
//
// Prediction draws your keystroke locally, so its time should stay low and flat
// as the delay grows; without prediction the time tracks the round trip. Both
// are above your terminal and display latency, which aft cannot change; this
// isolates aft's own contribution.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"time"

	"github.com/creack/pty"
)

// config is one run: a simulated one-way delay and whether prediction is on.
type config struct {
	latencyMS int
	predict   bool
}

func main() {
	aft := "./bin/aft"
	if len(os.Args) > 1 {
		aft = os.Args[1]
	}
	if _, err := os.Stat(aft); err != nil {
		fmt.Fprintln(os.Stderr, "build first: ./x build (looked for", aft+")")
		os.Exit(1)
	}
	delays := []int{0, 50, 120, 250}

	fmt.Println("| simulated delay (round trip) | predicted echo | raw echo (no prediction) |")
	fmt.Println("| --- | --- | --- |")
	for _, d := range delays {
		pOn := measure(aft, config{latencyMS: d, predict: true})
		pOff := measure(aft, config{latencyMS: d, predict: false})
		fmt.Printf("| %d ms | %s | %s |\n", 2*d, dur(pOn), dur(pOff))
	}
}

func dur(d time.Duration) string {
	if d < 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.1f ms", float64(d.Microseconds())/1000)
}

// measure types several characters at a settled prompt and returns the median
// time until each appears in the terminal stream.
func measure(aft string, c config) time.Duration {
	home, err := os.MkdirTemp("", "aft-bench-")
	if err != nil {
		return -1
	}
	defer os.RemoveAll(home)
	cmd := exec.Command(aft, "shell", "--host", "local")
	cmd.Env = append(os.Environ(),
		"AFT_HOME="+home,
		"TERM=xterm-256color",
		"SHELL=/bin/sh",
		fmt.Sprintf("AFT_LATENCY_MS=%d", c.latencyMS),
	)
	if !c.predict {
		cmd.Env = append(cmd.Env, "AFT_PREDICT=0")
	}
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: 80, Rows: 24})
	if err != nil {
		return -1
	}
	defer func() { cmd.Process.Kill(); ptmx.Close() }()

	seen := make(chan byte, 4096)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			for i := 0; i < n; i++ {
				select {
				case seen <- buf[i]:
				default:
				}
			}
			if err != nil {
				return
			}
		}
	}()
	drain(seen, 1500*time.Millisecond) // let the prompt settle

	var samples []time.Duration
	for i := 0; i < 8; i++ {
		drain(seen, 150*time.Millisecond)
		start := time.Now()
		ptmx.Write([]byte("x"))
		if waitFor(seen, 'x', 3*time.Second) {
			samples = append(samples, time.Since(start))
		}
		// Space samples out so the previous echo does not overlap the next key.
		time.Sleep(350 * time.Millisecond)
	}
	ptmx.Write([]byte{0x15}) // Ctrl-U clears the line
	if len(samples) == 0 {
		return -1
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	return samples[len(samples)/2]
}

// drain discards output until it has been quiet for the given idle time.
func drain(seen <-chan byte, idle time.Duration) {
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

// waitFor reports whether b appears within the timeout.
func waitFor(seen <-chan byte, b byte, timeout time.Duration) bool {
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
