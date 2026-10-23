// Package client is the Aft terminal client. It runs on the user's machine,
// attaches to a session on a server, passes the application's output through
// to the real terminal, predicts typed characters so typing feels local, and
// reconnects when the connection drops.
package client

import (
	"bufio"
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/arocomputer/aft/internal/bridge"
	"github.com/arocomputer/aft/internal/home"
	"github.com/arocomputer/aft/internal/ports"
	"github.com/arocomputer/aft/internal/proto"
	"github.com/arocomputer/aft/internal/remote"
	"github.com/arocomputer/aft/internal/transport"
	"github.com/arocomputer/aft/internal/vt"
)

// Options describe what to run or attach to.
type Options struct {
	Host      transport.Host
	SessionID string   // attach to this session; empty starts a new one
	Command   []string // for a new session
	Dir       string   // working directory on the server
	Env       []string // KEY=VALUE pairs to pass through
	Wrapper   []string // optional command to run the agent inside
	Predict   bool
	Ports     ports.Range
	DetachKey byte // 0 disables
	Prefix    byte // 0 disables; opens copy mode and detach
	LatencyMS int  // simulated one-way delay each direction, for the selftest
	Quiet     bool
}

// Run attaches the terminal to a session and returns the process exit code.
// saneSize defaults a terminal size to 80x24 when the terminal reports an error
// or a zero dimension. A 0x0 size would otherwise be sent as a resize the
// session reads as a control connection, so the session would never attach.
func saneSize(cols, rows int, err error) (int, int) {
	if err != nil || cols <= 0 || rows <= 0 {
		return 80, 24
	}
	return cols, rows
}

func Run(o Options) int {
	// Advanced/testing env overrides: AFT_LATENCY_MS simulates a slow link each
	// direction, and AFT_PREDICT=0 turns prediction off, so you can feel the
	// difference without a remote server.
	if o.LatencyMS == 0 {
		if n, err := strconv.Atoi(os.Getenv("AFT_LATENCY_MS")); err == nil && n > 0 {
			o.LatencyMS = n
		}
	}
	if v := os.Getenv("AFT_PREDICT"); v == "0" || v == "false" {
		o.Predict = false
	}
	in, out := os.Stdin, os.Stdout
	if !term.IsTerminal(int(in.Fd())) || !term.IsTerminal(int(out.Fd())) {
		fmt.Fprintln(os.Stderr, "aft: a terminal is required")
		return 2
	}
	if err := transport.EnsureMaster(o.Host); err != nil {
		fmt.Fprintln(os.Stderr, "aft:", err)
		return 1
	}
	if _, err := remote.Ensure(o.Host); err != nil {
		fmt.Fprintln(os.Stderr, "aft:", err)
		return 1
	}
	cols, rows := saneSize(term.GetSize(int(out.Fd())))

	c := &session{o: o, in: in, out: out, cols: cols, rows: rows, id: o.SessionID}
	c.outbound = make(chan []byte, 4096)
	c.sendDone = make(chan struct{})
	defer close(c.sendDone)
	go c.writer()
	c.mirror = ports.New(o.Host, o.Ports)
	c.pred = NewPredictor(cols, rows, out, o.Predict)
	// A copy made by the agent on the server (OSC 52) sets the laptop clipboard,
	// even when the terminal itself blocks OSC 52.
	c.pred.OnClipboard(func(data []byte) { go bridge.SetClipboard(data) })
	defer c.mirror.Close()
	defer c.clearMirrors()

	proc, err := c.open(true)
	if err != nil {
		fmt.Fprintln(os.Stderr, "aft:", err)
		return 1
	}
	state, err := term.MakeRaw(int(in.Fd()))
	if err != nil {
		fmt.Fprintln(os.Stderr, "aft:", err)
		return 1
	}
	restore := func() { term.Restore(int(in.Fd()), state) }
	defer restore()
	// If the terminal window closes (SIGHUP) or we are killed (SIGTERM), restore
	// the terminal before exiting so it is never left in raw mode. The session
	// keeps running on the server and can be reattached.
	death := make(chan os.Signal, 1)
	signal.Notify(death, syscall.SIGHUP, syscall.SIGTERM)
	go func() {
		<-death
		restore()
		os.Exit(1)
	}()
	code := c.loop(proc)
	restore()
	if c.message != "" {
		fmt.Fprintln(os.Stderr, c.message)
	}
	return code
}

type session struct {
	o          Options
	in, out    *os.File
	cols, rows int
	id         string
	pred       *Predictor
	mirror     *ports.Mirror
	message    string

	wmu sync.Mutex
	wr  io.Writer
	rd  io.Reader

	z     Tokenizer        // input tokenizer, kept across the loop
	flush <-chan time.Time // fires to release a lone Escape held by the tokenizer

	outbound chan []byte   // frames queued for the writer goroutine
	sendDone chan struct{} // closed on exit to stop the writer

	copy          *copyMode // non-nil while the scrollback pager is open
	prefixPending bool      // the prefix key was pressed; the next key is a command
	awaitSnap     bool      // a snapshot was requested to open copy mode
}

// open starts or attaches to the session and performs the handshake.
func (c *session) open(first bool) (*transport.Proc, error) {
	var args []string
	if c.id == "" {
		args = []string{"new", "--dir", c.o.Dir, "--cols", strconv.Itoa(c.cols), "--rows", strconv.Itoa(c.rows)}
		for _, kv := range c.o.Env {
			args = append(args, "--env", kv)
		}
		for _, w := range c.o.Wrapper {
			args = append(args, "--wrap", w)
		}
		args = append(args, "--")
		args = append(args, c.o.Command...)
	} else {
		args = []string{"attach", c.id}
	}
	proc, err := transport.Server(c.o.Host, args...)
	if err != nil {
		return nil, err
	}
	c.wmu.Lock()
	c.wr = proc.Stdin
	c.rd = proc.Stdout
	if d := time.Duration(c.o.LatencyMS) * time.Millisecond / 2; d > 0 {
		c.wr = laggyWriter{proc.Stdin, d}
		c.rd = laggyReader(proc.Stdout, d)
	}
	c.wmu.Unlock()
	if err := c.send(proto.Resize, proto.EncodeResize(c.cols, c.rows)); err != nil {
		proc.Kill()
		return nil, err
	}
	return proc, nil
}

// send queues a frame for the writer goroutine. It never writes to the network
// itself, so a slow or congested link cannot delay the input loop or the local
// prediction. Frames keep their order through the single queue and writer.
func (c *session) send(t proto.Type, payload []byte) error {
	var buf bytes.Buffer
	if err := proto.Write(&buf, t, payload); err != nil {
		return err
	}
	select {
	case c.outbound <- buf.Bytes():
	case <-c.sendDone:
	}
	return nil
}

// writer drains the outbound queue to the current connection. It is the only
// goroutine that writes to the network, so a blocking write (backpressure,
// the simulated latency) stalls only the queue, never typing.
func (c *session) writer() {
	for {
		select {
		case b := <-c.outbound:
			c.wmu.Lock()
			w := c.wr
			c.wmu.Unlock()
			if w != nil {
				w.Write(b)
			}
		case <-c.sendDone:
			return
		}
	}
}

func (c *session) title(s string) {
	c.out.Write([]byte("\x1b]2;" + s + "\x1b\\"))
}

type frameOrErr struct {
	f   proto.Frame
	err error
}

// loop runs until the application exits, the client detaches, or the user
// gives up on a reconnect.
func (c *session) loop(proc *transport.Proc) int {
	keys := make(chan []byte, 64)
	go func() {
		buf := make([]byte, 32<<10)
		for {
			n, err := c.in.Read(buf)
			if n > 0 {
				keys <- append([]byte(nil), buf[:n]...)
			}
			if err != nil {
				close(keys)
				return
			}
		}
	}()
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	defer signal.Stop(winch)
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()

	connected := true
	frames := c.readFrames(proc)
	for {
		// Keystrokes take priority: drain every queued key before touching
		// output, so a burst of agent output never stalls your typing.
		if connected {
		drain:
			for {
				select {
				case b, ok := <-keys:
					if leave, code := c.handleBatch(b, ok, connected); leave {
						return code
					}
				default:
					break drain
				}
			}
		}
		select {
		case b, ok := <-keys:
			if leave, code := c.handleBatch(b, ok, connected); leave {
				return code
			}
		case <-c.flush:
			c.flush = nil
			if connected && !c.handleKeys(c.z.Flush()) {
				return 0
			}
		case <-winch:
			if cols, rows, err := term.GetSize(int(c.out.Fd())); err == nil {
				c.cols, c.rows = cols, rows
				c.pred.Resize(cols, rows)
				if c.copy != nil {
					c.copy.resize(cols, rows)
					c.out.Write(c.copy.view())
				}
				if connected {
					c.send(proto.Resize, proto.EncodeResize(cols, rows))
				}
			}
		case <-tick.C:
			c.pred.Tick()
		case fe := <-frames:
			if fe.err != nil {
				// The connection dropped without the application ending.
				connected = false
				// Leave copy mode: its snapshot is stale and the reattach repaints.
				c.copy = nil
				c.awaitSnap = false
				msg, _ := proc.Wait()
				if c.id == "" {
					c.message = "aft: could not start the session: " + msg
					return 1
				}
				c.title("aft · reconnecting")
				var typed []byte
				var err error
				proc, typed, err = c.reconnect(keys)
				if err != nil {
					c.message = "aft: " + err.Error()
					return 1
				}
				connected = true
				frames = c.readFrames(proc)
				// Deliver what was typed while disconnected, in order.
				if len(typed) > 0 && !c.handleKeys(c.z.Feed(typed)) {
					return 0
				}
				continue
			}
			switch code, done := c.handleFrame(fe.f); {
			case done:
				proc.Wait()
				return code
			}
		}
	}
}

func (c *session) readFrames(proc *transport.Proc) <-chan frameOrErr {
	ch := make(chan frameOrErr, 256)
	rd := c.rd
	go func() {
		r := bufio.NewReaderSize(rd, 256<<10)
		for {
			f, err := proto.Read(r)
			if err != nil {
				ch <- frameOrErr{err: err}
				return
			}
			ch <- frameOrErr{f: f}
		}
	}()
	return ch
}

// handleKeys routes input: to copy mode when open, to a prefix command when the
// prefix was just pressed, otherwise to the application with prediction. It
// returns false when the client should leave (the user detached).
func (c *session) handleKeys(toks []Token) bool {
	for _, t := range toks {
		switch {
		case c.copy != nil:
			exit, copied := c.copy.key(t)
			if copied != nil {
				go bridge.SetClipboard(copied)
			}
			if exit {
				c.exitCopy()
			} else {
				c.out.Write(c.copy.view())
			}
		case c.prefixPending:
			c.prefixPending = false
			if !c.prefixCommand(t) {
				return false // prefix-d (detach) asks the loop to leave
			}
		case c.o.Prefix != 0 && len(t.Bytes) == 1 && t.Bytes[0] == c.o.Prefix:
			c.prefixPending = true
		case c.o.DetachKey != 0 && len(t.Bytes) == 1 && t.Bytes[0] == c.o.DetachKey:
			c.send(proto.Detach, nil)
			c.message = c.detachMessage()
			return false
		default:
			// Draw the local prediction first, so your echo is never delayed by
			// a slow or congested link, then forward the keystroke.
			c.pred.Key(t)
			if err := c.send(proto.Input, t.Bytes); err != nil {
				return true
			}
		}
	}
	return true
}

// handleBatch tokenizes one read from the terminal and routes it. It returns
// leave=true with an exit code when the client should stop (stdin closed, or a
// detach). It is called both from the priority drain and the main select.
func (c *session) handleBatch(b []byte, ok, connected bool) (leave bool, code int) {
	if !ok {
		c.send(proto.Detach, nil)
		return true, 0
	}
	if !connected {
		// While disconnected, only Ctrl-C leaves; other keys are dropped.
		if len(b) == 1 && b[0] == 0x03 {
			c.message = fmt.Sprintf("aft: left session %s while disconnected; it keeps running. Reattach with: aft attach %s", c.id, c.id)
			return true, 1
		}
		return false, 0
	}
	toks := c.z.Feed(b)
	if c.z.Pending() {
		c.flush = time.After(40 * time.Millisecond)
	} else {
		c.flush = nil
	}
	if !c.handleKeys(toks) {
		return true, 0
	}
	return false, 0
}

// prefixCommand runs the key after the prefix. It returns false only for detach,
// which asks the loop to leave.
func (c *session) prefixCommand(t Token) bool {
	switch {
	case len(t.Bytes) == 1 && t.Bytes[0] == c.o.Prefix:
		// The prefix twice sends the literal key to the application.
		c.pred.Key(t)
		c.send(proto.Input, t.Bytes)
	case string(t.Bytes) == "[":
		c.enterCopy()
	case string(t.Bytes) == "d":
		c.send(proto.Detach, nil)
		c.message = c.detachMessage()
		return false
	}
	return true
}

func (c *session) detachMessage() string {
	return fmt.Sprintf("aft: detached from %s; it keeps running. Reattach with: aft attach %s", c.id, c.id)
}

// enterCopy requests a snapshot; copy mode opens when it arrives.
func (c *session) enterCopy() {
	if c.awaitSnap || c.copy != nil {
		return
	}
	c.awaitSnap = true
	c.send(proto.ReqSnapshot, nil)
}

// exitCopy closes the pager and restores the live screen from the shadow.
func (c *session) exitCopy() {
	c.copy = nil
	c.pred.Repaint()
}

// handleFrame applies one frame. done is true when the client should exit.
func (c *session) handleFrame(f proto.Frame) (code int, done bool) {
	switch f.Type {
	case proto.Output:
		if c.copy != nil {
			// The pager owns the screen; keep the shadow current for the repaint
			// on exit, but do not draw the live output over the pager.
			c.pred.Absorb(f.Payload)
			return 0, false
		}
		if err := c.pred.Output(f.Payload); err != nil {
			return 1, true
		}
	case proto.Snapshot:
		if c.awaitSnap {
			c.awaitSnap = false
			var sd vt.SnapshotData
			if gob.NewDecoder(bytes.NewReader(f.Payload)).Decode(&sd) == nil {
				c.copy = newCopyMode(sd, c.cols, c.rows)
				c.out.Write(c.copy.view())
			}
		}
	case proto.Hello:
		var h proto.HelloInfo
		if json.Unmarshal(f.Payload, &h) == nil {
			c.id = h.ID
		}
	case proto.Exit:
		var e proto.ExitInfo
		json.Unmarshal(f.Payload, &e)
		if e.Signal != "" && !c.o.Quiet {
			c.message = "aft: the session ended by signal " + e.Signal
		}
		return e.Code, true
	case proto.Detached:
		c.message = fmt.Sprintf("aft: detached from %s; it keeps running. Reattach with: aft attach %s", c.id, c.id)
		return 0, true
	case proto.Error:
		c.message = "aft: " + string(f.Payload)
		return 1, true
	case proto.BridgeReq:
		var req proto.BridgeRequest
		if json.Unmarshal(f.Payload, &req) == nil {
			go func() {
				resp := bridge.Handle(req)
				b, _ := json.Marshal(resp)
				c.send(proto.BridgeReply, b)
			}()
		}
	case proto.Listeners:
		go func() { c.mirror.Update(proto.DecodePorts(f.Payload)); c.writeMirrors() }()
	case proto.Pong:
	}
	return 0, false
}

// reconnect reattaches with backoff. Keys typed meanwhile are kept and
// returned for delivery, except Ctrl-C, which leaves the session running and
// quits the client.
func (c *session) reconnect(keys <-chan []byte) (*transport.Proc, []byte, error) {
	delay := 500 * time.Millisecond
	var typed []byte
	for attempt := 1; ; attempt++ {
		wait := time.After(delay)
	collect:
		for {
			select {
			case b, ok := <-keys:
				if !ok || (len(b) == 1 && b[0] == 0x03) {
					return nil, nil, fmt.Errorf("left session %s while disconnected; it keeps running. Reattach with: aft attach %s", c.id, c.id)
				}
				typed = append(typed, b...)
			case <-wait:
				break collect
			}
		}
		if err := transport.EnsureMasterBatch(c.o.Host); err == nil {
			if proc, err := c.open(false); err == nil {
				c.pred.Reset()
				c.title("")
				return proc, typed, nil
			}
		}
		if delay < 5*time.Second {
			delay *= 2
		}
		if attempt%4 == 0 {
			c.title(fmt.Sprintf("aft · reconnecting (%d)", attempt))
		}
	}
}

// writeMirrors records the currently mirrored ports so `aft ports` can report
// them. It is keyed by session id and removed when the client exits.
func (c *session) writeMirrors() {
	if c.id == "" {
		return
	}
	active := c.mirror.Active()
	path := filepath.Join(home.Sockets(), "mirrors-"+c.id+".json")
	if len(active) == 0 {
		os.Remove(path)
		return
	}
	if b, err := json.Marshal(active); err == nil {
		os.WriteFile(path, b, 0o600)
	}
}

// clearMirrors removes this session's mirror record.
func (c *session) clearMirrors() {
	if c.id != "" {
		os.Remove(filepath.Join(home.Sockets(), "mirrors-"+c.id+".json"))
	}
}
