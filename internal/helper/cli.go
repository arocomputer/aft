// Package helper is the server side of Aft: the commands the client runs over
// SSH to start, attach to and manage sessions, plus the bridge shims that give
// an application on the server access to the user's machine. It is built into
// both the full aft binary (for local sessions) and the small aft-helper binary
// that Aft installs on servers.
package helper

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

	"github.com/fschrhunt/aft/internal/holder"
	"github.com/fschrhunt/aft/internal/home"
	"github.com/fschrhunt/aft/internal/proto"
)

// Info is what `aft-helper info` prints for doctor and version checks.
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

// Main runs a helper subcommand with stdio attached to the caller. prefix is
// how to invoke helper subcommands again from a child process: the aft-helper
// binary by itself, or the aft binary followed by "helper".
func Main(prefix []string, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: aft-helper new|attach|serve|ls|end|kick|setup|info|listeners")
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
			if err := holder.Serve(o); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
			return 0
		}
		if err := holder.Start(prefix, o); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return relay(o.ID, stdin, stdout, stderr)
	case "attach":
		if len(args) != 2 || !holder.ValidID(args[1]) {
			fmt.Fprintln(stderr, "usage: aft-helper attach ID")
			return 2
		}
		return relay(args[1], stdin, stdout, stderr)
	case "ls":
		list, err := holder.List()
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if list == nil {
			list = []holder.Record{}
		}
		json.NewEncoder(stdout).Encode(list)
		return 0
	case "end", "kick":
		if len(args) != 2 {
			fmt.Fprintf(stderr, "usage: aft-helper %s ID|--all\n", args[0])
			return 2
		}
		var ids []string
		if args[1] == "--all" {
			list, _ := holder.List()
			for _, r := range list {
				ids = append(ids, r.ID)
			}
		} else {
			ids = []string{args[1]}
		}
		code := 0
		for _, id := range ids {
			if !holder.ValidID(id) {
				fmt.Fprintf(stderr, "invalid session id %q\n", id)
				code = 2
				continue
			}
			t := kindFor(args[0])
			if err := holder.Control(id, t); err != nil {
				fmt.Fprintln(stderr, err)
				code = 1
			}
		}
		return code
	case "setup":
		fs := flag.NewFlagSet("setup", flag.ContinueOnError)
		sha := fs.String("sha", "", "sha256 of this helper binary, recorded for version checks")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if err := Setup(self, *sha); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintln(stdout, "ok")
		return 0
	case "info":
		json.NewEncoder(stdout).Encode(GatherInfo())
		return 0
	case "sharepath":
		if len(args) != 2 {
			fmt.Fprintln(stderr, "usage: aft-helper sharepath SHARE")
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
		ports, err := holder.Listeners()
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		json.NewEncoder(stdout).Encode(ports)
		return 0
	case "uninstall":
		// Ends every session and removes Aft's directory on this machine.
		list, _ := holder.List()
		for _, r := range list {
			holder.Control(r.ID, kindFor("end"))
		}
		if err := os.RemoveAll(home.Dir()); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintln(stdout, "removed", home.Dir())
		return 0
	}
	fmt.Fprintf(stderr, "unknown helper command %q\n", args[0])
	return 2
}

func parseSessionArgs(name string, args []string) (holder.Options, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var o holder.Options
	fs.StringVar(&o.ID, "id", "", "session id")
	fs.StringVar(&o.Dir, "dir", "", "working directory")
	fs.IntVar(&o.Cols, "cols", 80, "columns")
	fs.IntVar(&o.Rows, "rows", 24, "rows")
	var env multi
	fs.Var(&env, "env", "KEY=VALUE to pass to the application")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	o.Env = env
	o.Command = fs.Args()
	if len(o.Command) == 0 {
		return o, fmt.Errorf("%s requires -- COMMAND", name)
	}
	if o.ID == "" {
		o.ID = holder.NewID(o.Command[0])
	}
	if !holder.ValidID(o.ID) {
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
	if err := holder.Attach(id, struct {
		io.Reader
		io.Writer
	}{stdin, stdout}); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

// Setup prepares a server for sessions: the directory layout, the bridge
// shims, and the recorded hash of the installed helper.
func Setup(self, sha string) error {
	if err := home.Ensure(); err != nil {
		return err
	}
	if err := home.InstallBridge(self); err != nil {
		return err
	}
	if sha != "" {
		if err := os.WriteFile(filepath.Join(home.Bin(), "aft-helper.sha256"), []byte(sha+"\n"), 0o600); err != nil {
			return err
		}
	}
	return nil
}

// GatherInfo inspects this machine for doctor and the version check.
func GatherInfo() Info {
	info := Info{OS: runtime.GOOS, Arch: runtime.GOARCH, Version: Version, Home: home.Dir(), Shell: os.Getenv("SHELL"), Agents: map[string]string{}}
	if b, err := os.ReadFile(filepath.Join(home.Bin(), "aft-helper.sha256")); err == nil {
		info.SHA = strings.TrimSpace(string(b))
	}
	if _, err := os.Lstat(filepath.Join(home.Bridge(), "open")); err == nil {
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
