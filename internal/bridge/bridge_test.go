package bridge

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"encoding/json"

	"github.com/arocomputer/aft/internal/proto"
)

// TestOpenURLRejectsNonHTTP ensures the bridge only opens web URLs, never an
// arbitrary scheme a remote program might send.
func TestOpenURLRejectsNonHTTP(t *testing.T) {
	for _, u := range []string{"ftp://x", "file:///etc/passwd", "vnc://x", "javascript:alert(1)"} {
		req := proto.BridgeRequest{Kind: "open-url", Args: mustJSON(map[string]string{"url": u})}
		if r := Handle(req); r.OK {
			t.Errorf("opened a non-web URL: %q", u)
		}
	}
	if r := Handle(proto.BridgeRequest{Kind: "nope"}); r.OK || r.Error == "" {
		t.Error("unknown kind should fail with a message")
	}
}

// TestClipboardReadsLocalTool checks the clipboard reader runs the local
// clipboard tool and returns its bytes, using a fake xclip on PATH.
func TestClipboardReadsLocalTool(t *testing.T) {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		t.Setenv("WAYLAND_DISPLAY", "")
	}
	name := "xclip"
	if runtime.GOOS == "darwin" {
		name = "pbpaste"
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, name)
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nprintf 'clip-contents'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	data, err := Clipboard("text/plain")
	if err != nil || string(data) != "clip-contents" {
		t.Fatalf("got %q %v", data, err)
	}
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
