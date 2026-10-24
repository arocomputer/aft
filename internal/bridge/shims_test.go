package bridge

import (
	"encoding/json"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arocomputer/aft/internal/bridge/proto"
)

// TestShimPassthroughOutsideSession checks a shim without a session falls back
// to the real command rather than failing.
func TestShimPassthroughOutsideSession(t *testing.T) {
	t.Setenv("AFT_HOME", t.TempDir())
	t.Setenv("AFT_BRIDGE", "")
	if !IsShim("/x/.aft/shims/xdg-open") || IsShim("/usr/bin/server") {
		t.Fatal("shim detection")
	}
}

// TestOpenShimSendsBridgeRequest checks the `open` shim turns a URL into an
// open-url bridge request to the session's bridge socket.
func TestOpenShimSendsBridgeRequest(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "b.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	got := make(chan proto.BridgeRequest, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		var req proto.BridgeRequest
		json.NewDecoder(c).Decode(&req)
		got <- req
		json.NewEncoder(c).Encode(proto.BridgeResponse{ID: req.ID, OK: true})
	}()
	t.Setenv("AFT_BRIDGE", sock)
	if code := RunShim("/x/bridge/open", []string{"https://example.com/login"}); code != 0 {
		t.Fatalf("shim exit %d", code)
	}
	select {
	case req := <-got:
		if req.Kind != "open-url" || !strings.Contains(string(req.Args), "example.com/login") {
			t.Fatalf("wrong request: %s %s", req.Kind, req.Args)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no bridge request received")
	}
}
