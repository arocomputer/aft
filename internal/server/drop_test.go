package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestDropStoresUnderAftHome checks that a pasted file lands under the drop
// directory with its own name, and that a name cannot escape it.
func TestDropStoresUnderAftHome(t *testing.T) {
	t.Setenv("AFT_HOME", t.TempDir())
	path, err := Drop("Screen Recording.mov", strings.NewReader("video"))
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "video" || filepath.Base(path) != "Screen Recording.mov" {
		t.Fatalf("stored %q at %s", b, path)
	}
	if _, err := Drop("../escape", strings.NewReader("x")); err == nil {
		t.Fatal("a name with a path must be refused")
	}
}

// TestDropPrunesOldUploads checks that uploads older than a week are removed.
func TestDropPrunesOldUploads(t *testing.T) {
	t.Setenv("AFT_HOME", t.TempDir())
	old, _ := Drop("old.png", strings.NewReader("x"))
	week := time.Now().Add(-8 * 24 * time.Hour)
	os.Chtimes(filepath.Dir(old), week, week)
	if _, err := Drop("new.png", strings.NewReader("x")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("old upload was kept")
	}
}
