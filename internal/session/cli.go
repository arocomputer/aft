// The server CLI: the commands the terminal runs over SSH to start, attach to
// and manage sessions, plus clipboard-put for pasted files. It is built into
// both the full aft binary (for local sessions) and the small server binary
// that Aft installs on servers.

package session

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/arocomputer/aft/internal/bridge"
	"github.com/arocomputer/aft/internal/bridge/proto"
	"github.com/arocomputer/aft/internal/home"
)

// Info is what `server info` prints for doctor and version checks. SHA is the
// live hash of the running server binary, so aft can tell whether the installed
// server matches the one it carries without keeping a separate version file.
type Info struct {
	OS      string            `json:"os"`
	Arch    string            `json:"arch"`
	Version string            `json:"version"`
	SHA     string            `json:"sha"`
	Home    string            `json:"home"`
	Shell   string            `json:"shell"`
	Agents  map[string]string `json:"agents"`
	Shims   bool              `json:"shims"`
}

// Version is set by the build.
var Version = "dev"

// Main runs a server subcommand with stdio attached to the caller. prefix is
// how to invoke server subcommands again from a child process: the server
// binary by itself, or the aft binary followed by "server".
func Main(prefix []string, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: server new|attach|serve|ls|end|kick|setup|info|listeners|clipboard-put")
		return 2
	}
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if len(prefix) == 0 {
		prefix = []string{self}
	}
	switch args[0] {
	case "serve", "new":
		o, err := parseSessionArgs(args[0], args[1:])
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		if args[0] == "serve" {
			if err := Serve(o); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
			return 0
		}
		if err := Start(prefix, o); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return relay(o.ID, stdin, stdout, stderr)
	case "attach":
		if len(args) != 2 || !ValidID(args[1]) {
			fmt.Fprintln(stderr, "usage: server attach ID")
			return 2
		}
		return relay(args[1], stdin, stdout, stderr)
	case "ls":
		list, err := List()
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if list == nil {
			list = []Record{}
		}
		json.NewEncoder(stdout).Encode(list)
		return 0
	case "end", "kick":
		if len(args) != 2 {
			fmt.Fprintf(stderr, "usage: server %s ID|--all\n", args[0])
			return 2
		}
		var ids []string
		if args[1] == "--all" {
			list, _ := List()
			for _, r := range list {
				ids = append(ids, r.ID)
			}
		} else {
			ids = []string{args[1]}
		}
		code := 0
		for _, id := range ids {
			if !ValidID(id) {
				fmt.Fprintf(stderr, "invalid session id %q\n", id)
				code = 2
				continue
			}
			t := kindFor(args[0])
			if err := Control(id, t); err != nil {
				fmt.Fprintln(stderr, err)
				code = 1
			}
		}
		return code
	case "setup":
		if err := Setup(self); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintln(stdout, "ok")
		return 0
	case "info":
		json.NewEncoder(stdout).Encode(GatherInfo())
		return 0
	case "clipboard-put":
		if len(args) != 2 {
			fmt.Fprintln(stderr, "usage: server clipboard-put NAME < file")
			return 2
		}
		path, err := bridge.Store(args[1], stdin)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintln(stdout, path)
		return 0
	case "sharepath":
		if len(args) != 2 {
			fmt.Fprintln(stderr, "usage: server sharepath SHARE")
			return 2
		}
		path, err := SharePath(args[1])
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintln(stdout, path)
		return 0
	case "listeners":
		ports, err := Listeners()
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		json.NewEncoder(stdout).Encode(ports)
		return 0
	case "uninstall":
		// Ends every session and removes Aft's directory on this machine.
		list, _ := List()
		for _, r := range list {
			Control(r.ID, kindFor("end"))
		}
		for _, d := range []string{home.Dir(), home.Data(), home.Temp()} {
			if err := os.RemoveAll(d); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
			fmt.Fprintln(stdout, "removed", d)
		}
		return 0
	}
	fmt.Fprintf(stderr, "unknown server command %q\n", args[0])
	return 2
}

func parseSessionArgs(name string, args []string) (Options, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var o Options
	fs.StringVar(&o.ID, "id", "", "session id")
	fs.StringVar(&o.Dir, "dir", "", "working directory")
	fs.IntVar(&o.Cols, "cols", 80, "columns")
	fs.IntVar(&o.Rows, "rows", 24, "rows")
	var env, wrap multi
	fs.Var(&env, "env", "KEY=VALUE to pass to the application")
	fs.Var(&wrap, "wrap", "wrapper word to run the agent inside")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	o.Env = env
	o.Wrapper = wrap
	o.Command = fs.Args()
	if len(o.Command) == 0 {
		return o, fmt.Errorf("%s requires -- COMMAND", name)
	}
	if o.ID == "" {
		o.ID = NewID(o.Command[0])
	}
	if !ValidID(o.ID) {
		return o, fmt.Errorf("invalid session id %q", o.ID)
	}
	if o.Dir == "" {
		o.Dir, _ = os.UserHomeDir()
	}
	for _, kv := range o.Env {
		k, _, ok := strings.Cut(kv, "=")
		if !ok || k == "" || strings.ContainsAny(k, " \t\n") {
			return o, fmt.Errorf("invalid --env %q", kv)
		}
	}
	return o, nil
}

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(s string) error { *m = append(*m, s); return nil }

func relay(id string, stdin io.Reader, stdout, stderr io.Writer) int {
	if err := Attach(id, struct {
		io.Reader
		io.Writer
	}{stdin, stdout}); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

// Setup prepares a host for sessions: the directory layout and the bridge
// shims. self is the installed server binary, which the shims point at.
func Setup(self string) error {
	home.Migrate()
	if err := home.Ensure(); err != nil {
		return err
	}
	return home.InstallShims(self)
}

// GatherInfo inspects this machine for doctor and the version check.
func GatherInfo() Info {
	info := Info{OS: runtime.GOOS, Arch: runtime.GOARCH, Version: Version, Home: home.Dir(), Shell: os.Getenv("SHELL"), Agents: map[string]string{}}
	if exe, err := os.Executable(); err == nil {
		info.SHA, _ = FileSHA256(exe)
	}
	if _, err := os.Lstat(filepath.Join(home.Shims(), "open")); err == nil {
		info.Shims = true
	}
	for _, a := range []string{"claude", "codex", "opencode", "pi", "gemini", "amp", "aider", "goose", "git", "node"} {
		if p, err := exec.LookPath(a); err == nil {
			info.Agents[a] = p
		}
	}
	return info
}

// FileSHA256 returns the hex digest of a file.
func FileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// kindFor maps the end and kick subcommands to their control frames.
func kindFor(cmd string) proto.Type {
	if cmd == "kick" {
		return proto.Kick
	}
	return proto.End
}
