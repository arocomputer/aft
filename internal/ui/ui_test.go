package ui

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
)

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
		{[]byte{' '}, keySpace},
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
	selected, ok := Checklist("x", []Option{{Label: "a", Checked: true}, {Label: "b"}})
	if !ok || !slices.Equal(selected, []int{0}) {
		t.Fatalf("checklist defaults: %v %v", selected, ok)
	}
}

// TestMessagesStayPlain preserves severity labels without a theme or ANSI styling.
func TestMessagesStayPlain(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stderr
	os.Stderr = writer
	defer func() { os.Stderr = saved }()
	Warn("check the host")
	Error("cannot connect")
	writer.Close()
	output, err := io.ReadAll(reader)
	reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != "warning: check the host\nerror: cannot connect\n" {
		t.Fatalf("unexpected message styling: %q", output)
	}
}

// TestChecklistSelectionAndCancellation drives a real terminal to prove that
// preselected programs can be unchecked and cancellation accepts no selection.
func TestChecklistSelectionAndCancellation(t *testing.T) {
	if mode := os.Getenv("AFT_CHECKLIST_CHILD"); mode != "" {
		options := []Option{{Label: "claude", Checked: true}, {Label: "codex"}}
		selected, ok := Checklist("Programs", options)
		if mode == "toggle" && (!ok || !slices.Equal(selected, []int{1})) {
			t.Fatalf("selection: %v, %v", selected, ok)
		}
		if mode == "cancel" && (ok || selected != nil) {
			t.Fatalf("cancel: %v, %v", selected, ok)
		}
		if !options[0].Checked || options[1].Checked {
			t.Fatal("prompt changed its input options")
		}
		fmt.Println("CHECKLIST_OK")
		return
	}
	for _, mode := range []string{"toggle", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			self, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(self, "-test.run=^TestChecklistSelectionAndCancellation$")
			cmd.Env = append(os.Environ(), "AFT_CHECKLIST_CHILD="+mode)
			terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: 100, Rows: 24})
			if err != nil {
				t.Fatal(err)
			}
			defer terminal.Close()
			defer cmd.Process.Kill()
			chunks := make(chan string, 64)
			go func() {
				defer close(chunks)
				buffer := make([]byte, 4096)
				for {
					n, err := terminal.Read(buffer)
					if n > 0 {
						chunks <- string(buffer[:n])
					}
					if err != nil {
						return
					}
				}
			}()
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			output := ""
			wait := func(text string) {
				t.Helper()
				timer := time.NewTimer(8 * time.Second)
				defer timer.Stop()
				for !strings.Contains(output, text) {
					select {
					case chunk, open := <-chunks:
						if !open {
							t.Fatalf("terminal ended before %q: %s", text, output)
						}
						output += chunk
					case <-timer.C:
						t.Fatalf("waiting for %q: %s", text, output)
					}
				}
			}
			send := func(key string) {
				t.Helper()
				if _, err := terminal.Write([]byte(key)); err != nil {
					t.Fatal(err)
				}
			}
			wait("> [x] claude")
			if mode == "toggle" {
				send(" ")
				wait("> [ ] claude")
				send("\x1b[B")
				wait("> [ ] codex")
				send(" ")
				wait("> [x] codex")
				send("\r")
			} else {
				send("\x03")
			}
			wait("CHECKLIST_OK")
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(8 * time.Second):
				t.Fatal("prompt did not exit")
			}
		})
	}
}
