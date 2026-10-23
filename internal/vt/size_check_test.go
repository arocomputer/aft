package vt

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"testing"
)

// TestSnapshotBounded checks a full, wide scrollback encodes well under the
// protocol's frame limit, so copy mode never fails on a big buffer.
func TestSnapshotBounded(t *testing.T) {
	term := New(100, 40)
	for i := 0; i < 6000; i++ {
		term.Write([]byte(fmt.Sprintf("line %d with a fair amount of words to fill the row out more\r\n", i)))
	}
	var b bytes.Buffer
	if err := gob.NewEncoder(&b).Encode(term.Snapshot()); err != nil {
		t.Fatal(err)
	}
	t.Logf("snapshot gob size: %.2f MB", float64(b.Len())/1e6)
	if b.Len() > 12<<20 { // comfortably under the 16 MiB frame limit
		t.Fatalf("snapshot too large: %d bytes", b.Len())
	}
	// A screen full of blanks trims to almost nothing.
	blank := New(100, 40)
	var bb bytes.Buffer
	gob.NewEncoder(&bb).Encode(blank.Snapshot())
	if bb.Len() > 4096 {
		t.Fatalf("blank snapshot not trimmed: %d bytes", bb.Len())
	}
}
