package terminal

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/arocomputer/aft/internal/bridge/proto"
	"github.com/arocomputer/aft/internal/bridge/transport"
)

// TestPastedMountFileNamesTheServerPath pastes a file that a mounted folder
// already shares with the host: the paste names it on the host, at once, and
// nothing is uploaded.
func TestPastedMountFileNamesTheServerPath(t *testing.T) {
	dir := t.TempDir()
	local := filepath.Join(dir, "a b.txt")
	os.WriteFile(local, []byte("x"), 0o600)
	c := &session{outbound: make(chan []byte, 4), sendDone: make(chan struct{})}
	c.o.Host = transport.Host{Name: "box", SSH: "box"}
	c.o.ServerPath = func(p string) (string, bool) { return "/srv/share/" + filepath.Base(p), p == local }
	// The tokenizer hands the start marker over on its own, so a paste token
	// is the content and the end marker.
	paste := append([]byte(dir+`/a\ b.txt`), pasteEnd...)
	if !c.startUpload(Token{Kind: TokPaste, Bytes: paste}) || c.holding {
		t.Fatal("the paste was not handled at once")
	}
	f, err := proto.Read(bufio.NewReader(bytes.NewReader(<-c.outbound)))
	if err != nil || string(f.Payload) != `/srv/share/a\ b.txt`+string(pasteEnd) {
		t.Fatalf("sent %q, %v", f.Payload, err)
	}
}

// TestCancelledUploadClosesThePaste cancels an upload with Ctrl-C: the paste's
// start marker already went to the app, so an end marker follows, or the app
// would take every later key as pasted text.
func TestCancelledUploadClosesThePaste(t *testing.T) {
	c := &session{outbound: make(chan []byte, 4), sendDone: make(chan struct{}), pred: NewPredictor(40, 5, io.Discard, true), unflash: make(chan int, 1)}
	c.holding = true
	c.finishUpload(uploadResult{paste: []byte("/x.png" + string(pasteEnd)), err: context.Canceled})
	f, err := proto.Read(bufio.NewReader(bytes.NewReader(<-c.outbound)))
	if err != nil || string(f.Payload) != string(pasteEnd) {
		t.Fatalf("sent %q, want just the end marker", f.Payload)
	}
}

// TestCtrlCAfterUploadReachesTheApp presses Ctrl-C once the upload has
// finished but before its result is handled: the key is held and sent, not
// spent on a cancel that can no longer happen.
func TestCtrlCAfterUploadReachesTheApp(t *testing.T) {
	c := &session{outbound: make(chan []byte, 4), sendDone: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // the upload finished
	c.holding, c.uploadCtx, c.cancelUpload = true, ctx, cancel
	c.handleKeys([]Token{{Kind: TokOther, Bytes: []byte{0x03}}})
	if len(c.held) != 1 {
		t.Fatalf("held %d keys, want the Ctrl-C", len(c.held))
	}
}

// TestSecondCtrlCDuringCancelIsNotSent presses Ctrl-C twice while an upload
// runs: the first cancels it, and the second is the same request, never a ^C
// for the app, where two of them quit an agent.
func TestSecondCtrlCDuringCancelIsNotSent(t *testing.T) {
	c := &session{outbound: make(chan []byte, 4), sendDone: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	c.holding, c.uploadCtx, c.cancelUpload = true, ctx, cancel
	ctrlC := Token{Kind: TokOther, Bytes: []byte{0x03}}
	c.handleKeys([]Token{ctrlC, ctrlC})
	if len(c.held) != 0 || ctx.Err() == nil {
		t.Fatalf("held %d keys, cancelled %v", len(c.held), ctx.Err() != nil)
	}
}
