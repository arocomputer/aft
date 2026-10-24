package main

import (
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"

	"github.com/arocomputer/aft/internal/vt"
)

// TestMain lets this test binary act as the aft executable, including as the
// local server the client spawns, so a whole session runs in-process trees.
func TestMain(m *testing.M) {
	if os.Getenv("AFT_TEST_AS_AFT") == "1" && len(os.Args) > 1 {
		os.Exit(run(os.Args[1], os.Args[2:]))
	}
	os.Exit(m.Run())
}

// shortHome returns a short temp dir for AFT_HOME, since Unix socket paths are
// length-limited and the macOS temp dir is too deep.
func shortHome(t *testing.T) string {
	dir, err := os.MkdirTemp("/tmp", "aft-e2e-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// screen drives the aft client under a pseudo-terminal and emulates what the
// user would see.
type screen struct {
	t    *testing.T
	ptmx *os.File
	cmd  *exec.Cmd
	mu   sync.Mutex
	term *vt.Terminal
	raw  []byte
	done chan struct{}
}

func start(t *testing.T, args ...string) *screen {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(self, args...)
	cmd.Env = append(os.Environ(), "AFT_TEST_AS_AFT=1", "TERM=xterm-256color", "SHELL=/bin/sh", "PS1=$ ")
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	s := &screen{t: t, ptmx: ptmx, cmd: cmd, term: vt.New(80, 24), done: make(chan struct{})}
	go func() {
		buf := make([]byte, 32<<10)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				s.mu.Lock()
				s.raw = append(s.raw, buf[:n]...)
				s.term.Write(buf[:n])
				s.mu.Unlock()
			}
			if err != nil {
				close(s.done)
				return
			}
		}
	}()
	return s
}

func (s *screen) typeKeys(keys string) {
	if _, err := s.ptmx.Write([]byte(keys)); err != nil {
		s.t.Fatal(err)
	}
}

// waitFor blocks until text is on screen (any row) or fails the test.
func (s *screen) waitFor(text string, within time.Duration) {
	s.t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		found := false
		for r := 0; r < 24; r++ {
			if strings.Contains(s.term.Line(r), text) {
				found = true
				break
			}
		}
		s.mu.Unlock()
		if found {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var rows []string
	for r := 0; r < 24; r++ {
		rows = append(rows, s.term.Line(r))
	}
	s.t.Fatalf("never saw %q; screen:\n%s\nraw tail: %q", text, strings.Join(rows, "\n"), tail(s.raw, 300))
}

func (s *screen) rawContains(text string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Contains(string(s.raw), text)
}

func tail(b []byte, n int) []byte {
	if len(b) > n {
		return b[len(b)-n:]
	}
	return b
}

// relayPID finds the server relay the client spawned, so a test can kill it
// the way a dropped SSH connection would.
func relayPID(t *testing.T, parent int) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		out, _ := exec.Command("pgrep", "-P", strings.TrimSpace(itoa(parent)), "-f", "server (new|attach)").Output()
		if f := strings.Fields(string(out)); len(f) > 0 {
			var pid int
			for _, c := range f[0] {
				pid = pid*10 + int(c-'0')
			}
			return pid
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("relay process not found")
	return 0
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

// TestShellSessionSurvivesConnectionLoss runs a real shell session through the
// client on the local host, kills the relay as if the network dropped, and
// expects the client to reconnect with the screen intact and the shell alive.
func TestShellSessionSurvivesConnectionLoss(t *testing.T) {
	if _, err := exec.LookPath("pgrep"); err != nil {
		t.Skip("pgrep required")
	}
	t.Setenv("AFT_HOME", shortHome(t))
	s := start(t, "ssh", "local")
	defer s.cmd.Process.Kill()
	s.typeKeys("echo marker-$((20+22))\r")
	s.waitFor("marker-42", 10*time.Second)

	relay := relayPID(t, s.cmd.Process.Pid)
	syscall.Kill(relay, syscall.SIGKILL)
	// The client should announce the reconnect in the title and then repaint.
	deadline := time.Now().Add(5 * time.Second)
	for !s.rawContains("reconnecting") && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if !s.rawContains("reconnecting") {
		t.Fatal("no reconnecting title after the relay died")
	}
	s.typeKeys("echo after-$((40+2))\r")
	s.waitFor("after-42", 10*time.Second)
	s.waitFor("marker-42", time.Second) // earlier output survived the reattach

	s.typeKeys("exit\r")
	select {
	case <-s.done:
	case <-time.After(10 * time.Second):
		t.Fatal("client did not exit after the shell ended")
	}
	if err := s.cmd.Wait(); err != nil {
		t.Fatalf("client exit: %v", err)
	}
}

// TestShellReattachTakesOver runs aft ssh twice in the same place: the second
// comes back to the first's shell, which kept running, and the first client is
// detached because another terminal took the session.
func TestShellReattachTakesOver(t *testing.T) {
	t.Setenv("AFT_HOME", shortHome(t))
	s := start(t, "ssh", "local")
	defer s.cmd.Process.Kill()
	s.typeKeys("echo first-$((1+1))\r")
	s.waitFor("first-2", 10*time.Second)

	s2 := start(t, "ssh", "local")
	defer s2.cmd.Process.Kill()
	s2.waitFor("first-2", 10*time.Second)
	select {
	case <-s.done:
	case <-time.After(5 * time.Second):
		t.Fatal("first client stayed attached after the second took over")
	}
	s.cmd.Wait()
	s2.typeKeys("exit\r")
	select {
	case <-s2.done:
	case <-time.After(10 * time.Second):
		t.Fatal("client did not exit after the shell ended")
	}
}

// TestCopyModeScrollback drives a real shell session: it prints a marker that
// scrolls off, enters copy mode with the prefix, jumps to the top, and expects
// the marker back on screen with the copy-mode status bar.
func TestCopyModeScrollback(t *testing.T) {
	t.Setenv("AFT_HOME", shortHome(t))
	s := start(t, "ssh", "local")
	defer s.cmd.Process.Kill()
	s.typeKeys("printf 'MARKER-TOP\\n'; for i in $(seq 1 60); do echo line $i; done\r")
	s.waitFor("line 60", 10*time.Second)
	// The marker scrolled off the visible screen.
	// Enter copy mode: prefix (Ctrl-]) then "[".
	s.typeKeys("\x1d[")
	s.waitFor("COPY", 5*time.Second)
	// Jump to the top of the buffer; the marker comes back into view.
	s.typeKeys("g")
	s.waitFor("MARKER-TOP", 5*time.Second)
	// Leave copy mode and the shell.
	s.typeKeys("q")
	time.Sleep(200 * time.Millisecond)
	s.typeKeys("exit\r")
	select {
	case <-s.done:
	case <-time.After(10 * time.Second):
		t.Fatal("client did not exit after the shell ended")
	}
}
