// Package bridge performs, on the user's machine, the requests a remote
// application makes through the shims on the server: opening a URL in the
// local browser and reading the local clipboard.
package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/fschrhunt/aft/internal/proto"
)

// Handle performs one request and returns the response to send back.
func Handle(req proto.BridgeRequest) proto.BridgeResponse {
	resp := proto.BridgeResponse{ID: req.ID}
	var args map[string]string
	if len(req.Args) > 0 {
		json.Unmarshal(req.Args, &args)
	}
	switch req.Kind {
	case "open-url":
		url := args["url"]
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			return fail(resp, "only http and https URLs are opened")
		}
		if err := OpenURL(url); err != nil {
			return fail(resp, err.Error())
		}
		resp.OK = true
	case "clipboard-get":
		mime := args["mime"]
		if mime == "" {
			mime = "text/plain"
		}
		data, err := Clipboard(mime)
		if err != nil {
			return fail(resp, err.Error())
		}
		resp.OK, resp.Data, resp.Mime = true, data, mime
	case "clipboard-types":
		resp.OK = true
		resp.Data = []byte(strings.Join(ClipboardTypes(), "\n") + "\n")
	default:
		return fail(resp, "unknown request "+req.Kind)
	}
	return resp
}

func fail(r proto.BridgeResponse, msg string) proto.BridgeResponse {
	r.OK, r.Error = false, msg
	return r
}

func run(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = nil
	return cmd.Output()
}

// OpenURL opens a URL in the default browser.
func OpenURL(url string) error {
	var err error
	if runtime.GOOS == "darwin" {
		_, err = run("open", url)
	} else {
		_, err = run("xdg-open", url)
	}
	if err != nil {
		return fmt.Errorf("could not open the browser: %w", err)
	}
	return nil
}

// Clipboard returns clipboard contents of the given MIME type.
func Clipboard(mime string) ([]byte, error) {
	image := strings.HasPrefix(mime, "image/")
	if runtime.GOOS == "darwin" {
		if !image {
			return run("pbpaste")
		}
		return macImage()
	}
	if _, err := exec.LookPath("wl-paste"); err == nil && os.Getenv("WAYLAND_DISPLAY") != "" {
		return run("wl-paste", "--no-newline", "-t", mime)
	}
	if _, err := exec.LookPath("xclip"); err == nil {
		return run("xclip", "-selection", "clipboard", "-t", mime, "-o")
	}
	if _, err := exec.LookPath("xsel"); err == nil && !image {
		return run("xsel", "--clipboard", "--output")
	}
	return nil, fmt.Errorf("no clipboard tool found (install wl-clipboard or xclip)")
}

// ClipboardTypes lists the MIME types available, best effort.
func ClipboardTypes() []string {
	types := []string{"text/plain", "UTF8_STRING", "STRING"}
	if runtime.GOOS == "darwin" {
		if out, err := run("osascript", "-e", "clipboard info"); err == nil && strings.Contains(string(out), "PNGf") || strings.Contains(string(out), "TIFF") {
			types = append(types, "image/png")
		}
		return types
	}
	if _, err := exec.LookPath("wl-paste"); err == nil && os.Getenv("WAYLAND_DISPLAY") != "" {
		if out, err := run("wl-paste", "-l"); err == nil && strings.Contains(string(out), "image/png") {
			types = append(types, "image/png")
		}
		return types
	}
	if _, err := exec.LookPath("xclip"); err == nil {
		if out, err := run("xclip", "-selection", "clipboard", "-t", "TARGETS", "-o"); err == nil && strings.Contains(string(out), "image/png") {
			types = append(types, "image/png")
		}
	}
	return types
}

// macImage reads a PNG from the macOS clipboard through AppleScript, since
// pbpaste only returns text.
func macImage() ([]byte, error) {
	f, err := os.CreateTemp("", "aft-clip-*.png")
	if err != nil {
		return nil, err
	}
	path := f.Name()
	f.Close()
	defer os.Remove(path)
	script := fmt.Sprintf(`set f to open for access POSIX file %q with write permission
try
  set eof f to 0
  write (the clipboard as «class PNGf») to f
end try
close access f`, path)
	if _, err := run("osascript", "-e", script); err != nil {
		return nil, fmt.Errorf("no image on the clipboard")
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return nil, fmt.Errorf("no image on the clipboard")
	}
	return data, nil
}
