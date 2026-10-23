// Package client is the Aft terminal client. It runs on the user's machine,
// attaches to a session on a server, passes the application's output through
// to the real terminal, predicts typed characters so typing feels local, and
// reconnects when the connection drops.
package client

import (
	"bufio"
	"encoding/json"
	"errors"
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

	"github.com/fschrhunt/aft/internal/bridge"
	"github.com/fschrhunt/aft/internal/home"
	"github.com/fschrhunt/aft/internal/ports"
	"github.com/fschrhunt/aft/internal/proto"
	"github.com/fschrhunt/aft/internal/remote"
	"github.com/fschrhunt/aft/internal/transport"
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
	Quiet     bool
}

// Run attaches the terminal to a session and returns the process exit code.
func Run(o Options) int {
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
	cols, rows, err := term.GetSize(int(out.Fd()))
	if err != nil {
		cols, rows = 80, 24
	}

	c := &session{o: o, in: in, out: out, cols: cols, rows: rows, id: o.SessionID}
	c.mirror = ports.New(o.Host, o.Ports)
	c.pred = NewPredictor(cols, rows, out, o.Predict)
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

	wmu  sync.Mutex
	wr   io.Writer
	zero bool
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
	proc, err := transport.Helper(c.o.Host, args...)
	if err != nil {
		return nil, err
	}
	c.wmu.Lock()
	c.wr = proc.Stdin
	c.wmu.Unlock()
	if err := c.send(proto.Resize, proto.EncodeResize(c.cols, c.rows)); err != nil {
		proc.Kill()
		return nil, err
	}
	return proc, nil
}

func (c *session) send(t proto.Type, payload []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if c.wr == nil {
		return errors.New("not connected")
	}
	return proto.Write(c.wr, t, payload)
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

	var z Tokenizer
	var flush <-chan time.Time
	connected := true
	frames := c.readFrames(proc)
	for {
		select {
		case b, ok := <-keys:
			if !ok {
				c.send(proto.Detach, nil)
				return 0
			}
			if !connected {
				if len(b) == 1 && b[0] == 0x03 {
					c.message = fmt.Sprintf("aft: left session %s while disconnected; it keeps running. Reattach with: aft attach %s", c.id, c.id)
					return 1
				}
				continue
			}
			toks := z.Feed(b)
			if z.Pending() {
				flush = time.After(40 * time.Millisecond)
			} else {
				flush = nil
			}
			if !c.handleKeys(toks) {
				return 0
			}
		case <-flush:
			flush = nil
			if connected && !c.handleKeys(z.Flush()) {
				return 0
			}
		case <-winch:
			if cols, rows, err := term.GetSize(int(c.out.Fd())); err == nil {
				c.cols, c.rows = cols, rows
				c.pred.Resize(cols, rows)
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
				if len(typed) > 0 && !c.handleKeys(z.Feed(typed)) {
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
	go func() {
		r := bufio.NewReaderSize(proc.Stdout, 256<<10)
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

// handleKeys forwards tokens and lets the predictor draw. It returns false if
// the user pressed the detach key.
func (c *session) handleKeys(toks []Token) bool {
	for _, t := range toks {
		if c.o.DetachKey != 0 && t.Kind == TokOther && len(t.Bytes) == 1 && t.Bytes[0] == c.o.DetachKey {
			c.send(proto.Detach, nil)
			c.message = fmt.Sprintf("aft: detached from %s; it keeps running. Reattach with: aft attach %s", c.id, c.id)
			return false
		}
		if err := c.send(proto.Input, t.Bytes); err != nil {
			return true
		}
		c.pred.Key(t)
	}
	return true
}

// handleFrame applies one frame. done is true when the client should exit.
func (c *session) handleFrame(f proto.Frame) (code int, done bool) {
	switch f.Type {
	case proto.Output:
		if err := c.pred.Output(f.Payload); err != nil {
			return 1, true
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
	path := filepath.Join(home.Run(), "mirrors-"+c.id+".json")
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
		os.Remove(filepath.Join(home.Run(), "mirrors-"+c.id+".json"))
	}
}
