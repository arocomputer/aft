package session

import (
	"os"
	"path/filepath"
	"testing"
)

// TestScanShare finds a share's path case-insensitively and ignores others.
func TestScanShare(t *testing.T) {
	conf := `[global]
   workgroup = WG
[Room]
   comment = Room working files
   path = /home/fschr/room
   read only = no
[media]
   path = /srv/media
`
	dir := t.TempDir()
	p := filepath.Join(dir, "smb.conf")
	os.WriteFile(p, []byte(conf), 0o644)
	f, _ := os.Open(p)
	defer f.Close()
	path, found := scanShare(f, "room") // lowercase on purpose
	if !found || path != "/home/fschr/room" {
		t.Fatalf("got %q found=%v", path, found)
	}
	f2, _ := os.Open(p)
	defer f2.Close()
	if _, found := scanShare(f2, "nope"); found {
		t.Fatal("found a share that does not exist")
	}
}
