package client

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestPastedFilesReadsTerminalPaths pins the forms terminals paste for a
// dropped or copied file: backslash-escaped spaces and quoted paths, several
// separated by spaces.
func TestPastedFilesReadsTerminalPaths(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "Screen Recording 1.44.00 AM.mov")
	b := filepath.Join(dir, "shot.png")
	for _, p := range []string{a, b} {
		os.WriteFile(p, []byte("x"), 0o600)
	}
	got, ok := pastedFiles(shellEscape(a) + " '" + b + "' ")
	if !ok || !reflect.DeepEqual(got, []string{a, b}) {
		t.Fatalf("got %q ok=%v", got, ok)
	}
}

// TestPastedFilesLeavesTextAlone checks that a paste is only rewritten when it
// is nothing but existing local files.
func TestPastedFilesLeavesTextAlone(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "notes.txt")
	os.WriteFile(f, []byte("x"), 0o600)
	for _, body := range []string{
		"look at " + f,                // prose mentioning a path
		filepath.Join(dir, "missing"), // not on this machine
		dir,                           // a directory
		"notes.txt",                   // relative
		"'" + f,                       // unterminated quote
	} {
		if _, ok := pastedFiles(body); ok {
			t.Errorf("rewrote %q", body)
		}
	}
}
