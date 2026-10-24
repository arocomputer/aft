package session

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/arocomputer/aft/internal/bridge/proto"
	"github.com/arocomputer/aft/internal/bridge/udp"
	"github.com/arocomputer/aft/internal/home"
)

// TestMain lets the test binary act as the server executable, so Start
// can spawn real detached sessions during tests.
func TestMain(m *testing.M) {
	if os.Getenv("AFT_TEST_AS_SERVER") == "1" {
		os.Exit(Main(nil, os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
	}
	os.Exit(m.Run())
}

func isolate(t *testing.T) {
	t.Helper()
	// Unix socket paths are length-limited (~104 bytes on macOS), and the OS
	// temp dir is deep there, so use a short home under /tmp.
	dir, err := os.MkdirTemp("/tmp", "aft-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	t.Setenv("AFT_HOME", dir)
	t.Setenv("AFT_TEST_AS_SERVER", "1")
	t.Setenv("SHELL", "/bin/sh")
	// Sessions run through the login shell, which reads the profile; a
	// developer's own dotfiles must not print into a test's screen.
	t.Setenv("HOME", dir)
}

type conn struct {
	c net.Conn
	r *bufio.Reader
}

func attach(t *testing.T, id string, cols, rows int) *conn {
	t.Helper()
	c, err := net.Dial("unix", SocketPath(id))
	if err != nil {
		t.Fatal(err)
	}
	if err := proto.Write(c, proto.Resize, proto.EncodeResize(cols, rows)); err != nil {
		t.Fatal(err)
	}
	return &conn{c: c, r: bufio.NewReader(c)}
}

func (c *conn) read(t *testing.T) proto.Frame {
	t.Helper()
	c.c.SetReadDeadline(time.Now().Add(5 * time.Second))
	f, err := proto.Read(c.r)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// collect reads output frames until the text appears or time runs out.
func (c *conn) collect(t *testing.T, want string) string {
	t.Helper()
	var all strings.Builder
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		c.c.SetReadDeadline(deadline)
		f, err := proto.Read(c.r)
		if err != nil {
			break
		}
		if f.Type == proto.Output {
			all.Write(f.Payload)
			if strings.Contains(all.String(), want) {
				return all.String()
			}
		}
	}
	t.Fatalf("did not see %q in %q", want, all.String())
	return ""
}

// TestSessionLifecycle runs a real detached session: attach, type, detach,
// reattach with a repaint, then end it.
func TestSessionLifecycle(t *testing.T) {
	isolate(t)
	self, _ := os.Executable()
	serverCmd := []string{self}
	o := Options{ID: "t-shell", Command: []string{"sh", "-c", `printf 'ready> '; read line; printf 'got:%s\n' "$line"; read again`}, Dir: t.TempDir(), Cols: 40, Rows: 10}
	if err := Start(serverCmd, o); err != nil {
		t.Fatal(err)
	}
	defer Control("t-shell", proto.End)

	c := attach(t, "t-shell", 40, 10)
	if f := c.read(t); f.Type != proto.Hello {
		t.Fatalf("first frame %v", f.Type)
	}
	c.collect(t, "ready>") // trailing space is trimmed on a repaint, so do not depend on it
	proto.Write(c.c, proto.Input, []byte("hello\r"))
	c.collect(t, "got:hello")

	list, err := List()
	if err != nil || len(list) != 1 || list[0].ID != "t-shell" {
		t.Fatalf("list %+v %v", list, err)
	}

	// Drop the connection: the session must survive.
	c.c.Close()
	time.Sleep(100 * time.Millisecond)
	if list, _ := List(); len(list) != 1 {
		t.Fatal("session died with its client")
	}

	// Reattach at a new size: the repaint must carry the earlier output.
	c2 := attach(t, "t-shell", 60, 12)
	var h proto.HelloInfo
	f := c2.read(t)
	json.Unmarshal(f.Payload, &h)
	if f.Type != proto.Hello || h.Cols != 60 || h.ID != "t-shell" {
		t.Fatalf("hello %+v", h)
	}
	paint := c2.collect(t, "got:hello")
	if !strings.Contains(paint, "ready> hello") {
		t.Fatalf("repaint missing prompt line: %q", paint)
	}

	// Ending the session delivers an Exit frame to the attached client.
	proto.Write(c2.c, proto.End, nil)
	deadline := time.Now().Add(5 * time.Second)
	for {
		c2.c.SetReadDeadline(deadline)
		f, err := proto.Read(c2.r)
		if err != nil {
			t.Fatalf("no exit frame: %v", err)
		}
		if f.Type == proto.Exit {
			break
		}
	}
	time.Sleep(100 * time.Millisecond)
	if list, _ := List(); len(list) != 0 {
		t.Fatalf("session still listed: %+v", list)
	}
}

// TestDetachedQueriesAreAnswered checks an application does not hang on a
// cursor-position report while nobody is attached.
func TestDetachedQueriesAreAnswered(t *testing.T) {
	isolate(t)
	self, _ := os.Executable()
	// The program asks for the cursor position and prints the reply it gets.
	script := `stty raw -echo; printf '\033[6n'; reply=$(head -c 6 | tr -d '\033'); stty sane; printf 'reply=%s\n' "$reply"; sleep 30`
	o := Options{ID: "t-query", Command: []string{"sh", "-c", script}, Dir: t.TempDir(), Cols: 40, Rows: 10}
	if err := Start([]string{self}, o); err != nil {
		t.Fatal(err)
	}
	defer Control("t-query", proto.End)
	time.Sleep(300 * time.Millisecond) // the query happens with no client
	c := attach(t, "t-query", 40, 10)
	c.read(t) // hello
	paint := c.collect(t, "reply=")
	if !strings.Contains(paint, "reply=[1;1") {
		t.Fatalf("query not answered while detached: %q", paint)
	}
}

// TestSecondClientTakesOver checks a new attach gets the session and the old
// client is told it was detached.
func TestSecondClientTakesOver(t *testing.T) {
	isolate(t)
	self, _ := os.Executable()
	o := Options{ID: "t-two", Command: []string{"sh", "-c", "printf 'x> '; sleep 30"}, Dir: t.TempDir(), Cols: 40, Rows: 10}
	if err := Start([]string{self}, o); err != nil {
		t.Fatal(err)
	}
	defer Control("t-two", proto.End)
	a := attach(t, "t-two", 40, 10)
	a.read(t)
	a.collect(t, "x>")
	b := attach(t, "t-two", 40, 10)
	b.read(t)
	b.collect(t, "x>")
	deadline := time.Now().Add(5 * time.Second)
	for {
		a.c.SetReadDeadline(deadline)
		f, err := proto.Read(a.r)
		if err != nil {
			t.Fatalf("old client not told: %v", err)
		}
		if f.Type == proto.Detached {
			break
		}
	}
}

// TestWrapperWrapsCommand checks a configured wrapper runs the agent inside it:
// the wrapper writes a marker, then execs the command, and the session shows
// both. This is the knob that lets users bring their own environment.
func TestWrapperWrapsCommand(t *testing.T) {
	isolate(t)
	self, _ := os.Executable()
	o := Options{
		ID:      "t-wrap",
		Command: []string{"sh", "-c", "echo COMMAND-RAN; sleep 30"},
		Wrapper: []string{"sh", "-c", `echo WRAPPER-RAN; exec "$@"`, "sh"},
		Dir:     t.TempDir(),
		Cols:    40, Rows: 10,
	}
	if err := Start([]string{self}, o); err != nil {
		t.Fatal(err)
	}
	defer Control("t-wrap", proto.End)
	c := attach(t, "t-wrap", 40, 10)
	c.read(t) // hello
	out := c.collect(t, "COMMAND-RAN")
	if !strings.Contains(out, "WRAPPER-RAN") {
		t.Fatalf("wrapper did not run before the command: %q", out)
	}
}

// TestNewIDSkipsNamesInUse gives a new session a word no live session on the
// host has.
func TestNewIDSkipsNamesInUse(t *testing.T) {
	t.Setenv("AFT_HOME", t.TempDir())
	home.Ensure()
	for _, n := range names[1:] {
		writeJSON(RecordPath(n), Record{ID: n, Server: os.Getpid(), PID: os.Getpid()})
	}
	if id := NewID(); id != names[0] {
		t.Fatalf("got %q, want the only free name %q", id, names[0])
	}
}

// TestCleanTitle drops the status glyph an agent puts before its title.
func TestCleanTitle(t *testing.T) {
	for in, want := range map[string]string{"✳ Fix login": "Fix login", "⠐ Fix login": "Fix login", "zsh": "zsh", "~/room/code": "~/room/code", "[1] build": "[1] build", "": ""} {
		if got := cleanTitle(in); got != want {
			t.Errorf("cleanTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestUDPReattach reaches a session over UDP, drops the connection without a
// goodbye, as a laptop changing networks does, and redials with the same
// offer: the session takes the new connection and repaints.
func TestUDPReattach(t *testing.T) {
	isolate(t)
	self, _ := os.Executable()
	o := Options{ID: "t-udp", Command: []string{"sh", "-c", `printf 'mark-udp\n'; sleep 30`}, Dir: t.TempDir(), Cols: 40, Rows: 10}
	if err := Start([]string{self}, o); err != nil {
		t.Fatal(err)
	}
	defer Control("t-udp", proto.End)
	offer, err := OfferUDP("t-udp")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		nc, err := udp.Dial("127.0.0.1", offer)
		if err != nil {
			t.Fatalf("dial %d: %v", i, err)
		}
		proto.Write(nc, proto.Resize, proto.EncodeResize(40, 10))
		c := &conn{c: nc, r: bufio.NewReader(nc)}
		if f := c.read(t); f.Type != proto.Hello {
			t.Fatalf("dial %d: got frame %x, want hello", i, f.Type)
		}
		c.collect(t, "mark-udp")
		// No Detach frame: the next dial must take over regardless.
	}
}

// TestEndingUnseenLeavesRemains lets a session exit with no one attached: it
// leaves its exit code and last screen, and remains hands them over once.
func TestEndingUnseenLeavesRemains(t *testing.T) {
	isolate(t)
	self, _ := os.Executable()
	o := Options{ID: "t-gone", Command: []string{"sh", "-c", `printf 'last words\n'; sleep 0.3; exit 3`}, Dir: t.TempDir(), Cols: 40, Rows: 10}
	if err := Start([]string{self}, o); err != nil {
		t.Fatal(err)
	}
	var r []Record
	for deadline := time.Now().Add(5 * time.Second); len(r) == 0 && time.Now().Before(deadline); {
		time.Sleep(50 * time.Millisecond)
		r = Remains()
	}
	if len(r) != 1 || r[0].Ended == nil || r[0].Ended.Code != 3 {
		t.Fatalf("remains: %+v", r)
	}
	var out strings.Builder
	if code := Main([]string{self}, []string{"remains", "t-gone"}, nil, &out, io.Discard); code != 0 || !strings.Contains(out.String(), "last words") {
		t.Fatalf("remains printed %q (exit %d)", out.String(), code)
	}
	if len(Remains()) != 0 {
		t.Fatal("remains were not cleared after being shown")
	}
}

// TestNewIDSkipsEndedNames keeps the name of a session that ended unseen, so
// a new one never shares it with remains that kill would forget instead.
func TestNewIDSkipsEndedNames(t *testing.T) {
	t.Setenv("AFT_HOME", t.TempDir())
	home.Ensure()
	for _, n := range names[1:] {
		writeJSON(RecordPath(n), Record{ID: n, Server: os.Getpid(), PID: os.Getpid()})
	}
	leaveRemains(Record{ID: names[0]}, Ending{At: time.Now().UTC().Format(time.RFC3339)})
	if id := NewID(); id == names[0] {
		t.Fatalf("reused %q, which an ended session still holds", id)
	}
}

// TestGoneSessionSaysSo reaches for a session that is not running: attach
// answers in the client's protocol and udp answers gone, so a client stops
// instead of reconnecting, and says why.
func TestGoneSessionSaysSo(t *testing.T) {
	isolate(t)
	home.Ensure()
	leaveRemains(Record{ID: "fox"}, Ending{At: time.Now().UTC().Format(time.RFC3339), Code: 1})
	var out bytes.Buffer
	Main(nil, []string{"attach", "fox"}, strings.NewReader(""), &out, io.Discard)
	f, err := proto.Read(bufio.NewReader(&out))
	if err != nil || f.Type != proto.Error || !strings.Contains(string(f.Payload), "aft attach fox") {
		t.Fatalf("attach answered %x %q %v", f.Type, f.Payload, err)
	}
	out.Reset()
	Main(nil, []string{"udp", "fox"}, nil, &out, io.Discard)
	var got struct{ Gone, Remains bool }
	if json.Unmarshal(out.Bytes(), &got) != nil || !got.Gone || !got.Remains {
		t.Fatalf("udp answered %q", out.String())
	}
}

// TestStaleOrMissingSocketIsNotRunning treats a missing socket and one left
// behind by a session that died (its connection is refused) as not running,
// which the client turns into "gone"; other dial failures are not.
func TestStaleOrMissingSocketIsNotRunning(t *testing.T) {
	isolate(t)
	home.Ensure()
	var gone notRunning
	if _, err := dial("nobody"); !errors.As(err, &gone) {
		t.Fatalf("missing socket: %v, want not running", err)
	}
	// A session that died leaves its socket file behind, which refuses.
	ln, err := net.Listen("unix", SocketPath("stale"))
	if err != nil {
		t.Fatal(err)
	}
	ln.(*net.UnixListener).SetUnlinkOnClose(false)
	ln.Close()
	if _, err := dial("stale"); !errors.As(err, &gone) {
		t.Fatalf("stale socket: %v, want not running", err)
	}
}

// TestNewIDClaimsTheName makes two names picked back to back differ, because
// each is claimed by its record file the moment it is picked.
func TestNewIDClaimsTheName(t *testing.T) {
	t.Setenv("AFT_HOME", t.TempDir())
	a, b := NewID(), NewID()
	if a == b {
		t.Fatalf("both sessions were named %q", a)
	}
	for _, id := range []string{a, b} {
		if fi, err := os.Stat(RecordPath(id)); err != nil || fi.Size() != 0 {
			t.Fatalf("%s is not claimed by an empty record: %v", id, err)
		}
	}
}

// TestRenameFollowsTheSession renames a running session: the new name lists
// and answers, the old one is gone, the attached terminal is told, and a name
// in use is refused.
func TestRenameFollowsTheSession(t *testing.T) {
	isolate(t)
	self, _ := os.Executable()
	o := Options{ID: "t-old", Command: []string{"sh", "-c", "printf 'r> '; sleep 30"}, Dir: t.TempDir(), Cols: 40, Rows: 10}
	if err := Start([]string{self}, o); err != nil {
		t.Fatal(err)
	}
	other := Options{ID: "t-taken", Command: []string{"sleep", "30"}, Dir: t.TempDir(), Cols: 40, Rows: 10}
	if err := Start([]string{self}, other); err != nil {
		t.Fatal(err)
	}
	defer Control("t-taken", proto.End)
	a := attach(t, "t-old", 40, 10)
	a.read(t)
	a.collect(t, "r>")
	if err := Rename("t-old", "t-taken"); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("a name in use was given away: %v", err)
	}
	if err := Rename("t-old", "t-new"); err != nil {
		t.Fatal(err)
	}
	defer Control("t-new", proto.End)
	if f := a.read(t); f.Type != proto.Renamed || string(f.Payload) != "t-new" {
		t.Fatalf("attached client got %x %q, want Renamed t-new", f.Type, f.Payload)
	}
	if r, err := Load("t-new"); err != nil || r.ID != "t-new" {
		t.Fatalf("new record: %+v %v", r, err)
	}
	if _, err := Load("t-old"); err == nil {
		t.Fatal("old record still there")
	}
	if _, err := OfferUDP("t-new"); err != nil {
		t.Fatalf("new name does not answer: %v", err)
	}
	if err := Control("t-old", proto.Ping); err == nil {
		t.Fatal("old name still answers")
	}
}

// TestReuseComesBackToTheSameProgram makes udp --reuse answer with the session
// already running the same program in the same directory, and start a new one
// for another program or directory.
func TestReuseComesBackToTheSameProgram(t *testing.T) {
	isolate(t)
	self, _ := os.Executable()
	dir := t.TempDir()
	o := Options{ID: "t-first", Command: []string{"sleep", "30"}, Dir: dir, Cols: 40, Rows: 10}
	if err := Start([]string{self}, o); err != nil {
		t.Fatal(err)
	}
	defer Control("t-first", proto.End)
	ask := func(args ...string) (id string, reused bool) {
		var out strings.Builder
		if code := Main([]string{self}, append([]string{"udp", "--reuse", "--cols", "40", "--rows", "10"}, args...), nil, &out, io.Discard); code != 0 {
			t.Fatalf("udp exited %d", code)
		}
		var got struct {
			ID     string `json:"id"`
			Reused bool   `json:"reused"`
		}
		json.Unmarshal([]byte(out.String()), &got)
		return got.ID, got.Reused
	}
	if id, reused := ask("--dir", dir, "sleep", "30"); id != "t-first" || !reused {
		t.Fatalf("same program here: got %s reused=%v", id, reused)
	}
	id, reused := ask("--dir", dir, "sh", "-c", "sleep 30")
	if id == "t-first" || id == "" || reused {
		t.Fatalf("another program here: got %s reused=%v", id, reused)
	}
	Control(id, proto.End)
	id, reused = ask("--dir", t.TempDir(), "sleep", "30")
	if id == "t-first" || id == "" || reused {
		t.Fatalf("same program elsewhere: got %s reused=%v", id, reused)
	}
	Control(id, proto.End)
}

// TestTakeoverNamesTheMachine tells a replaced client which machine took the
// session, when the new client said where it is.
func TestTakeoverNamesTheMachine(t *testing.T) {
	isolate(t)
	self, _ := os.Executable()
	o := Options{ID: "t-from", Command: []string{"sh", "-c", "printf 'f> '; sleep 30"}, Dir: t.TempDir(), Cols: 40, Rows: 10}
	if err := Start([]string{self}, o); err != nil {
		t.Fatal(err)
	}
	defer Control("t-from", proto.End)
	a := attach(t, "t-from", 40, 10)
	a.read(t)
	a.collect(t, "f>")
	b := attach(t, "t-from", 40, 10)
	proto.Write(b.c, proto.From, []byte("laptop"))
	b.read(t)
	for {
		f := a.read(t)
		if f.Type == proto.Detached {
			if string(f.Payload) != "laptop" {
				t.Fatalf("told %q, want laptop", f.Payload)
			}
			break
		}
	}
}

// TestMuteSessionIsExplained turns a request a session never answers, as one
// from an older server does with a frame it does not know, into a sentence
// rather than an I/O timeout.
func TestMuteSessionIsExplained(t *testing.T) {
	isolate(t)
	home.Ensure()
	ln, err := net.Listen("unix", SocketPath("t-mute"))
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			defer c.Close() // hold it open, say nothing
		}
	}()
	err = Rename("t-mute", "t-loud")
	if err == nil || !strings.Contains(err.Error(), "did not answer") {
		t.Fatalf("got %v", err)
	}
}
