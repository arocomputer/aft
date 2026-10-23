package ui

import "testing"

// TestDecodeKey pins the keypress classification the prompts rely on.
func TestDecodeKey(t *testing.T) {
	cases := []struct {
		in   []byte
		want key
	}{
		{[]byte{'y'}, keyYes},
		{[]byte{'Y'}, keyYes},
		{[]byte{'n'}, keyNo},
		{[]byte{'\r'}, keyEnter},
		{[]byte{'\n'}, keyEnter},
		{[]byte{3}, keyCancel},
		{[]byte{'q'}, keyCancel},
		{[]byte{27}, keyCancel},
		{[]byte{27, '[', 'A'}, keyUp},
		{[]byte{27, '[', 'B'}, keyDown},
		{[]byte{27, '[', 'C'}, keyRight},
		{[]byte{27, '[', 'D'}, keyLeft},
		{[]byte{'k'}, keyUp},
		{[]byte{'j'}, keyDown},
		{[]byte{'\t'}, keyRight},
		{[]byte{'z'}, keyOther},
		{nil, keyCancel},
	}
	for _, tc := range cases {
		if got := decodeKey(tc.in); got != tc.want {
			t.Errorf("decodeKey(%v) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// TestNonInteractiveFallback checks prompts return the default without a tty,
// so scripted runs never block.
func TestNonInteractiveFallback(t *testing.T) {
	if Interactive() {
		t.Skip("stdin is a terminal in this run")
	}
	if !Confirm("x", true) || Confirm("x", false) {
		t.Fatal("Confirm must return its default without a terminal")
	}
	if Choose("x", []Option{{Label: "a"}, {Label: "b"}}, 1) != 1 {
		t.Fatal("Choose must return its default without a terminal")
	}
}
