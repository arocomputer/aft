package terminal

import (
	"bufio"
	"bytes"
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
