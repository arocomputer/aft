package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/arocomputer/aft/internal/home"
)

// dropKeep is how long pasted files stay on the host. Older uploads are
// removed by the next drop, so pasted recordings do not pile up forever.
const dropKeep = 7 * 24 * time.Hour

// Drop stores one file pasted on the user's machine, read from r, as
// ~/.aft/drop/<unique>/<name> and returns its absolute path. The client pastes
// that path in place of the local one. name must be a bare file name; the
// original name is kept so the application sees what the user pasted.
func Drop(name string, r io.Reader) (string, error) {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
		return "", fmt.Errorf("invalid file name %q", name)
	}
	pruneDrops(time.Now().Add(-dropKeep))
	var id [4]byte
	rand.Read(id[:])
	dir := filepath.Join(home.Drops(), time.Now().UTC().Format("20060102-150405")+"-"+hex.EncodeToString(id[:]))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		os.RemoveAll(dir)
		return "", err
	}
	if err := f.Close(); err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return path, nil
}

// pruneDrops removes upload directories last changed before cutoff.
func pruneDrops(cutoff time.Time) {
	entries, _ := os.ReadDir(home.Drops())
	for _, e := range entries {
		if fi, err := e.Info(); err == nil && e.IsDir() && fi.ModTime().Before(cutoff) {
			os.RemoveAll(filepath.Join(home.Drops(), e.Name()))
		}
	}
}
