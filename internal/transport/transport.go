// Package transport reaches a server's helper. Over SSH it uses the user's own
// OpenSSH client with one shared master connection per host, so keys, agents,
// jump hosts and host trust all come from the user's SSH configuration. The
// reserved host "local" runs the helper on this machine, which is how tests
// and single-machine trials work.
package transport

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/fschrhunt/aft/internal/home"
)

// Host is a server to reach. SSH is the destination given to ssh, usually an
// alias from ~/.ssh/config. Name "local" means this machine.
type Host struct {
	Name string
	SSH  string
}

// Local is the host that runs sessions on this machine.
var Local = Host{Name: "local"}

// IsLocal reports whether the host is this machine.
func (h Host) IsLocal() bool { return h.Name == "local" || h.SSH == "" }

// RemoteHelper is the helper path on a server, expanded by the remote shell.
const RemoteHelper = "~/.aft/bin/aft-helper"

func controlPath() string { return filepath.Join(home.Run(), "cm-%C") }

// sshOptions are shared by every ssh invocation.
func sshOptions(batch bool) []string {
	opts := []string{
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=" + controlPath(),
		"-o", "ControlPersist=600",
		"-o", "ServerAliveInterval=5",
		"-o", "ServerAliveCountMax=3",
	}
	if batch {
		opts = append(opts, "-o", "BatchMode=yes")
	}
	return opts
}

// EnsureMaster makes sure a master connection to the host exists. The first
// connection may prompt for a passphrase, so it inherits the terminal; callers
// run it before entering raw mode.
func EnsureMaster(h Host) error {
	if h.IsLocal() {
		return nil
	}
	if err := home.Ensure(); err != nil {
		return err
	}
	if MasterAlive(h) {
		return nil
	}
	cmd := exec.Command("ssh", append(sshOptions(false), "-fN", "--", h.SSH)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("connect to %s: %w", h.SSH, err)
	}
	return nil
}

// MasterAlive reports whether the shared connection is up.
func MasterAlive(h Host) bool {
	if h.IsLocal() {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ssh", "-o", "ControlPath="+controlPath(), "-O", "check", "--", h.SSH)
	return cmd.Run() == nil
}

// EnsureMasterBatch reconnects without a terminal, for use while attached.
func EnsureMasterBatch(h Host) error {
	if h.IsLocal() || MasterAlive(h) {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ssh", append(sshOptions(true), "-fN", "--", h.SSH)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("reconnect to %s: %s", h.SSH, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// CloseMaster ends the shared connection.
func CloseMaster(h Host) {
	if h.IsLocal() {
		return
	}
	exec.Command("ssh", "-o", "ControlPath="+controlPath(), "-O", "exit", "--", h.SSH).Run()
}

// Proc is a running helper command with its pipes.
type Proc struct {
	cmd    *exec.Cmd
	Stdin  io.WriteCloser
	Stdout io.ReadCloser
	stderr bytes.Buffer
}

// Helper starts a helper subcommand on the host with piped stdin and stdout.
func Helper(h Host, args ...string) (*Proc, error) {
	cmd, err := helperCommand(h, args)
	if err != nil {
		return nil, err
	}
	p := &Proc{cmd: cmd}
	cmd.Stderr = &p.stderr
	p.Stdin, err = cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	p.Stdout, err = cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return p, nil
}

// Wait reaps the process and returns its stderr with any error.
func (p *Proc) Wait() (string, error) {
	err := p.cmd.Wait()
	return strings.TrimSpace(p.stderr.String()), err
}

// Kill stops the process.
func (p *Proc) Kill() {
	if p.cmd.Process != nil {
		p.cmd.Process.Kill()
	}
}

// Stderr returns what the helper has written to stderr so far.
func (p *Proc) Stderr() string { return strings.TrimSpace(p.stderr.String()) }

// Output runs a helper subcommand to completion and returns its stdout.
func Output(h Host, stdin io.Reader, args ...string) ([]byte, error) {
	cmd, err := helperCommand(h, args)
	if err != nil {
		return nil, err
	}
	cmd.Stdin = stdin
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return out, fmt.Errorf("%s", msg)
	}
	return out, nil
}

// helperCommand builds the command that runs a helper subcommand on the host.
func helperCommand(h Host, args []string) (*exec.Cmd, error) {
	if h.IsLocal() {
		self, err := os.Executable()
		if err != nil {
			return nil, err
		}
		return exec.Command(self, append([]string{"helper"}, args...)...), nil
	}
	remote := RemoteHelper + " " + shellJoin(args)
	return exec.Command("ssh", append(sshOptions(true), "-T", "--", h.SSH, remote)...), nil
}

// Shell runs a command on the host with this process's terminal attached.
func Shell(h Host, script string) error {
	var cmd *exec.Cmd
	if h.IsLocal() {
		cmd = exec.Command("sh", "-c", script)
	} else {
		cmd = exec.Command("ssh", append(sshOptions(false), "-t", "--", h.SSH, script)...)
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// RunRaw runs a remote shell script with the given stdin and returns stdout.
func RunRaw(h Host, stdin io.Reader, script string) ([]byte, error) {
	var cmd *exec.Cmd
	if h.IsLocal() {
		cmd = exec.Command("sh", "-c", script)
	} else {
		cmd = exec.Command("ssh", append(sshOptions(true), "-T", "--", h.SSH, script)...)
	}
	cmd.Stdin = stdin
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return out, fmt.Errorf("%s", msg)
	}
	return out, nil
}

// Forward adds or cancels a loopback port forward on the master connection.
func Forward(h Host, port int, add bool) error {
	if h.IsLocal() {
		return nil
	}
	op := "forward"
	if !add {
		op = "cancel"
	}
	spec := fmt.Sprintf("127.0.0.1:%d:localhost:%d", port, port)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ssh", "-o", "ControlPath="+controlPath(), "-O", op, "-L", spec, "--", h.SSH)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s port %d: %s", op, port, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// Quote makes one POSIX shell word.
func Quote(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\n'\"\\$`!*?[]{}()<>|&;~#") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func shellJoin(args []string) string {
	q := make([]string, len(args))
	for i, a := range args {
		q[i] = Quote(a)
	}
	return strings.Join(q, " ")
}
