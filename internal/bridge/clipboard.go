package bridge

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// The local clipboard, through the platform's tools: pbcopy and pbpaste (and
// AppleScript for images) on macOS, wl-clipboard, xclip or xsel on Linux.

// SetClipboard writes data to the local clipboard.
func SetClipboard(data []byte) error {
	name, args := "", []string(nil)
	switch {
	case runtime.GOOS == "darwin":
		name = "pbcopy"
	case hasTool("wl-copy") && os.Getenv("WAYLAND_DISPLAY") != "":
		name = "wl-copy"
	case hasTool("xclip"):
		name, args = "xclip", []string{"-selection", "clipboard"}
	case hasTool("xsel"):
		name, args = "xsel", []string{"--clipboard", "--input"}
	default:
		return fmt.Errorf("no clipboard tool found")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = bytes.NewReader(data)
	return cmd.Run()
}

// ClipboardAvailable reports whether a local clipboard tool exists.
func ClipboardAvailable() bool {
	if runtime.GOOS == "darwin" {
		return hasTool("pbcopy")
	}
	return hasTool("wl-copy") || hasTool("xclip") || hasTool("xsel")
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
	if hasTool("wl-paste") && os.Getenv("WAYLAND_DISPLAY") != "" {
		return run("wl-paste", "--no-newline", "-t", mime)
	}
	if hasTool("xclip") {
		return run("xclip", "-selection", "clipboard", "-t", mime, "-o")
	}
	if hasTool("xsel") && !image {
		return run("xsel", "--clipboard", "--output")
	}
	return nil, fmt.Errorf("no clipboard tool found (install wl-clipboard or xclip)")
}

// ClipboardTypes lists the MIME types available, best effort.
func ClipboardTypes() []string {
	types := []string{"text/plain", "UTF8_STRING", "STRING"}
	if runtime.GOOS == "darwin" {
		if out, err := run("osascript", "-e", "clipboard info"); err == nil && (strings.Contains(string(out), "PNGf") || strings.Contains(string(out), "TIFF")) {
			types = append(types, "image/png")
		}
		return types
	}
	if hasTool("wl-paste") && os.Getenv("WAYLAND_DISPLAY") != "" {
		if out, err := run("wl-paste", "-l"); err == nil && strings.Contains(string(out), "image/png") {
			types = append(types, "image/png")
		}
		return types
	}
	if hasTool("xclip") {
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
	// Browsers and Preview often put only a TIFF on the pasteboard; sips
	// turns it into the PNG the application asked for.
	script := fmt.Sprintf(`set f to open for access POSIX file %q with write permission
set eof f to 0
try
  write (the clipboard as «class PNGf») to f
on error
  write (the clipboard as «class TIFF») to f
end try
close access f`, path)
	if _, err := run("osascript", "-e", script); err != nil {
		return nil, fmt.Errorf("no image on the clipboard")
	}
	if head, err := os.ReadFile(path); err == nil && len(head) >= 4 && !bytes.HasPrefix(head, []byte("\x89PNG")) {
		if _, err := run("sips", "-s", "format", "png", path, "--out", path); err != nil {
			return nil, fmt.Errorf("no image on the clipboard")
		}
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return nil, fmt.Errorf("no image on the clipboard")
	}
	return data, nil
}
