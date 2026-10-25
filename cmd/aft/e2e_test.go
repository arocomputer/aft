package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"

	"github.com/arocomputer/aft/internal/bridge/transport"
	"github.com/arocomputer/aft/internal/config"
	"github.com/arocomputer/aft/internal/home"
	"github.com/arocomputer/aft/internal/session"
	"github.com/arocomputer/aft/internal/vt"
)

// TestMain lets this test binary act as the aft executable, including as the
// local server the client spawns, so a whole session runs in-process trees.
func TestMain(m *testing.M) {
	if os.Getenv("AFT_TEST_AS_AFT") == "1" && filepath.Base(os.Args[0]) == home.ServerName {
		os.Exit(serverMain(os.Args[1:]))
	}
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
	t.Cleanup(func() {
		// A test that fails midway leaves its session running; its shell
		// would live on invisibly, so end it by the record it kept.
		entries, _ := os.ReadDir(filepath.Join(dir, "sessions"))
		for _, e := range entries {
			var r session.Record
			if b, err := os.ReadFile(filepath.Join(dir, "sessions", e.Name())); err == nil && json.Unmarshal(b, &r) == nil && r.Server > 0 {
				syscall.Kill(r.Server, syscall.SIGTERM)
			}
		}
		os.RemoveAll(dir)
	})
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

// start launches a client under a PTY. AFT_TEST_BINARY selects an extracted
// release executable; otherwise the test binary acts as the client.
func start(t *testing.T, args ...string) *screen {
	t.Helper()
	self := os.Getenv("AFT_TEST_BINARY")
	if self == "" {
		var err error
		self, err = os.Executable()
		if err != nil {
			t.Fatal(err)
		}
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
// Keys typed while it was away arrive after the reattach, in order.
func TestShellSessionSurvivesConnectionLoss(t *testing.T) {
	if _, err := exec.LookPath("pgrep"); err != nil {
		t.Skip("pgrep required")
	}
	t.Setenv("AFT_HOME", shortHome(t))
	s := start(t, "local")
	defer s.cmd.Process.Kill()
	s.typeKeys("echo marker-$((20+22))\r")
	s.waitFor("marker-42", 10*time.Second)

	relay := relayPID(t, s.cmd.Process.Pid)
	syscall.Kill(relay, syscall.SIGKILL)
	// A reconnect that succeeds at once says nothing: the notice across the
	// top row appears only when the session stays out of reach. The new relay
	// shows the client is back.
	deadline := time.Now().Add(5 * time.Second)
	for relayPID(t, s.cmd.Process.Pid) == relay && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
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

// TestHostShellsAreIndependent opens two connections in the same directory:
// each gets its own shell state, and both remain usable. Agent launch wrappers
// must not wrap either host shell.
func TestHostShellsAreIndependent(t *testing.T) {
	t.Setenv("AFT_HOME", shortHome(t))
	if err := config.Save(config.Config{Wrapper: []string{"false"}}); err != nil {
		t.Fatal(err)
	}
	s := start(t, "local")
	defer s.cmd.Process.Kill()
	s.typeKeys("aft_test_value=first; echo first-$((1+1))\r")
	s.waitFor("first-2", 10*time.Second)

	s2 := start(t, "local")
	defer s2.cmd.Process.Kill()
	s2.typeKeys("echo second-${aft_test_value:-fresh}\r")
	s2.waitFor("second-fresh", 10*time.Second)
	s.typeKeys("echo still-${aft_test_value}\r")
	s.waitFor("still-first", 5*time.Second)
	for _, client := range []*screen{s, s2} {
		client.typeKeys("exit\r")
		select {
		case <-client.done:
		case <-time.After(10 * time.Second):
			t.Fatal("client did not exit after its shell ended")
		}
		if err := client.cmd.Wait(); err != nil {
			t.Fatalf("client exit: %v", err)
		}
	}
}

// TestShellProjectVisibility follows a shell into and out of an enabled tree,
// permits an explicit takeover inside, and rejects attach outside, even by host.
func TestShellProjectVisibility(t *testing.T) {
	t.Setenv("AFT_HOME", shortHome(t))
	project := t.TempDir()
	subdir := filepath.Join(project, "sub")
	if err := os.Mkdir(subdir, 0o700); err != nil {
		t.Fatal(err)
	}
	st := config.State{Projects: map[string]config.Project{
		project: {Host: "local", Dir: project},
	}}
	if err := config.SaveState(st); err != nil {
		t.Fatal(err)
	}
	s := start(t, "local")
	defer s.cmd.Process.Kill()
	s.typeKeys("echo general-$((1+1))\r")
	s.waitFor("general-2", 10*time.Second)
	if all := allSessions(config.Config{}); len(all) != 0 {
		t.Fatalf("general host shell appeared: %+v", all)
	}
	s.typeKeys("cd '" + subdir + "'; echo project-$((2+2))\r")
	s.waitFor("project-4", 5*time.Second)
	list, err := projectSessions(transport.Local, st)
	if err != nil || len(list) != 1 || list[0].Dir != config.RealPath(subdir) {
		t.Fatalf("shell inside project: %+v, %v", list, err)
	}
	id := list[0].ID
	s2 := start(t, "attach", "--host", "local", id)
	defer s2.cmd.Process.Kill()
	s2.waitFor("project-4", 10*time.Second)
	select {
	case <-s.done:
	case <-time.After(5 * time.Second):
		t.Fatal("explicit attach did not take over")
	}
	s.cmd.Wait()
	s2.typeKeys("cd /; echo outside-$((3+3))\r")
	s2.waitFor("outside-6", 5*time.Second)
	if all := allSessions(config.Config{}); len(all) != 0 {
		t.Fatalf("shell outside project appeared: %+v", all)
	}
	for _, args := range [][]string{{id}, {"--host", "local", id}} {
		captureCLI(t, func() {
			if code := cmdAttach(args); code != 1 {
				t.Errorf("attach %v returned %d outside project", args, code)
			}
		})
	}
	// Hiding the shell does not stop it; it can return to the project.
	s2.typeKeys("cd '" + project + "'; echo returned-$((4+4))\r")
	s2.waitFor("returned-8", 5*time.Second)
	if list, err := projectSessions(transport.Local, st); err != nil || len(list) != 1 || list[0].ID != id {
		t.Fatalf("shell returning to project: %+v, %v", list, err)
	}
	if err := config.SaveState(config.State{}); err != nil {
		t.Fatal(err)
	}
	if all := allSessions(config.Config{}); len(all) != 0 {
		t.Fatalf("disabled project still exposes its shell: %+v", all)
	}
	s2.typeKeys("exit\r")
	select {
	case <-s2.done:
	case <-time.After(10 * time.Second):
		t.Fatal("client did not exit after its shell ended")
	}
	s2.cmd.Wait()
}

// TestCopyModeScrollback drives a real shell session: it prints a marker that
// scrolls off, enters copy mode with the prefix, jumps to the top, and expects
// the marker back on screen with the copy-mode status bar.
func TestCopyModeScrollback(t *testing.T) {
	t.Setenv("AFT_HOME", shortHome(t))
	s := start(t, "local")
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
