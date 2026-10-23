package helper

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/arocomputer/aft/internal/home"
	"github.com/arocomputer/aft/internal/proto"
)

// IsShim reports whether argv0 names a bridge shim.
func IsShim(argv0 string) bool {
	base := filepath.Base(argv0)
	for _, n := range home.BridgeShims {
		if n == base {
			return true
		}
	}
	return false
}

// RunShim implements a bridge shim: open and xdg-open send URLs to the user's
// machine, and the clipboard readers fetch its clipboard. Anything a shim does
// not handle is passed to the real command, found on PATH after the shim
// directory.
func RunShim(argv0 string, args []string) int {
	name := filepath.Base(argv0)
	switch name {
	case "open", "xdg-open":
		if len(args) == 1 && isURL(args[0]) {
			if _, err := bridge("open-url", map[string]string{"url": args[0]}); err != nil {
				fmt.Fprintln(os.Stderr, name+":", err)
				return 1
			}
			return 0
		}
		return passthrough(name, args)
	case "pbpaste":
		return clipboardTo(os.Stdout, "text/plain")
	case "wl-paste":
		mime := "text/plain"
		for i, a := range args {
			if (a == "-t" || a == "--type") && i+1 < len(args) {
				mime = args[i+1]
			}
			if a == "-l" || a == "--list-types" {
				return listTypes()
			}
		}
		return clipboardTo(os.Stdout, mime)
	case "xclip":
		mime, out := "text/plain", false
		for i, a := range args {
			switch a {
			case "-o", "-out":
				out = true
			case "-t", "-target":
				if i+1 < len(args) {
					mime = args[i+1]
				}
			}
		}
		if !out {
			return passthrough(name, args) // writing the clipboard is passed through
		}
		if mime == "TARGETS" {
			return listTypes()
		}
		return clipboardTo(os.Stdout, mime)
	case "xsel":
		for _, a := range args {
			if a == "-i" || a == "--input" {
				return passthrough(name, args)
			}
		}
		return clipboardTo(os.Stdout, "text/plain")
	}
	return passthrough(name, args)
}

func isURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

func listTypes() int {
	resp, err := bridge("clipboard-types", nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	os.Stdout.Write(resp.Data)
	return 0
}

func clipboardTo(w io.Writer, mime string) int {
	resp, err := bridge("clipboard-get", map[string]string{"mime": mime})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	w.Write(resp.Data)
	return 0
}

// bridge sends one request to the session's bridge socket and waits.
func bridge(kind string, args any) (proto.BridgeResponse, error) {
	sock := os.Getenv("AFT_BRIDGE")
	if sock == "" {
		return proto.BridgeResponse{}, fmt.Errorf("not inside an Aft session")
	}
	conn, err := net.DialTimeout("unix", sock, 2*time.Second)
	if err != nil {
		return proto.BridgeResponse{}, fmt.Errorf("the Aft session is gone")
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(60 * time.Second))
	raw, _ := json.Marshal(args)
	if err := json.NewEncoder(conn).Encode(proto.BridgeRequest{Kind: kind, Args: raw}); err != nil {
		return proto.BridgeResponse{}, err
	}
	var resp proto.BridgeResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return proto.BridgeResponse{}, err
	}
	if !resp.OK {
		return resp, fmt.Errorf("%s", resp.Error)
	}
	return resp, nil
}

// passthrough runs the real command of the same name, skipping the shim dir.
func passthrough(name string, args []string) int {
	bridge := home.Bridge()
	for _, dir := range strings.Split(os.Getenv("PATH"), string(os.PathListSeparator)) {
		if dir == "" || sameDir(dir, bridge) {
			continue
		}
		p := filepath.Join(dir, name)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0 {
			if err := syscall.Exec(p, append([]string{name}, args...), os.Environ()); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
		}
	}
	fmt.Fprintf(os.Stderr, "%s: not available on this machine\n", name)
	return 127
}

func sameDir(a, b string) bool {
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	if err1 != nil || err2 != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return ra == rb
}
