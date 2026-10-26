// Package ui provides plain command output, prompts and transient progress.
// The session client uses diagnostics only after restoring the terminal.
// Each prompt falls back to a default when there is no terminal, so
// scripted and piped runs never block.
//
// Single-choice prompts show just their choices. Checklists also explain
// Space to toggle and Enter to continue.
//
// The prompts are driven directly in raw mode rather than with a TUI framework:
// Bubble Tea queries the terminal (an OSC 11 background probe) merely on import,
// which would corrupt a session client sharing the terminal. Nothing in this
// package writes to or reads from the terminal until a prompt actually runs.
package ui

import (
	"fmt"
	"os"

	"github.com/charmbracelet/x/ansi"
	"golang.org/x/term"
)

// Interactive reports whether prompts can run.
func Interactive() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stderr.Fd()))
}

// key is a decoded keypress.
type key int

const (
	keyOther key = iota
	keyUp
	keyDown
	keyLeft
	keyRight
	keyEnter
	keyCancel
	keyYes
	keyNo
	keySpace
)

// readKey reads one keypress in raw mode.
func readKey() key {
	var b [3]byte
	n, err := os.Stdin.Read(b[:])
	if err != nil || n == 0 {
		return keyCancel
	}
	return decodeKey(b[:n])
}

// decodeKey classifies a short input sequence. It is pure, so it is tested
// directly without a terminal.
func decodeKey(b []byte) key {
	if len(b) == 0 {
		return keyCancel
	}
	switch b[0] {
	case 27: // ESC: an arrow sequence, or a bare ESC that cancels
		if len(b) >= 3 && b[1] == '[' {
			switch b[2] {
			case 'A':
				return keyUp
			case 'B':
				return keyDown
			case 'C':
				return keyRight
			case 'D':
				return keyLeft
			}
			return keyOther
		}
		return keyCancel
	case 3, 'q':
		return keyCancel
	case '\r', '\n':
		return keyEnter
	case 'k':
		return keyUp
	case 'j':
		return keyDown
	case 'h':
		return keyLeft
	case 'l', '\t':
		return keyRight
	case 'y', 'Y':
		return keyYes
	case 'n', 'N':
		return keyNo
	case ' ':
		return keySpace
	}
	return keyOther
}

// screen draws a block of lines to stderr and redraws it in place on each
// update, so the prompt stays on the same spot. It assumes raw mode, so line
// breaks are explicit carriage-return plus newline.
type screen struct{ prev int }

func (s *screen) render(lines []string) {
	if s.prev > 0 {
		fmt.Fprintf(os.Stderr, "\x1b[%dA", s.prev) // move to the block's first line
	}
	// A line wider than the terminal would wrap and throw the redraw off.
	width := 0
	if w, _, err := term.GetSize(int(os.Stderr.Fd())); err == nil && w > 1 {
		width = w - 1
	}
	for _, l := range lines {
		if width > 0 && ansi.StringWidth(l) > width {
			l = ansi.Truncate(l, width, "")
		}
		fmt.Fprint(os.Stderr, "\r\x1b[K"+l+"\r\n")
	}
	s.prev = len(lines)
}

// clear erases the drawn block, leaving the cursor where the prompt began.
func (s *screen) clear() {
	if s.prev == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "\x1b[%dA", s.prev)
	for i := 0; i < s.prev; i++ {
		fmt.Fprint(os.Stderr, "\r\x1b[K\r\n")
	}
	fmt.Fprintf(os.Stderr, "\x1b[%dA", s.prev)
	s.prev = 0
}

// Confirm asks a yes/no question, returning def when there is no terminal
// and false when the user backs out with Escape, q or Ctrl-C.
func Confirm(question string, def bool) bool {
	if !Interactive() {
		return def
	}
	st, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return def
	}
	defer term.Restore(int(os.Stdin.Fd()), st)
	value := def
	var sc screen
	render := func() {
		yes, no := "  Yes  ", "  No  "
		if value {
			yes = "[ Yes ]"
		} else {
			no = "[ No ]"
		}
		sc.render([]string{question, "  " + yes + " " + no})
	}
	render()
	for {
		switch readKey() {
		case keyYes:
			sc.clear()
			return true
		case keyNo:
			sc.clear()
			return false
		case keyLeft, keyRight, keyUp, keyDown:
			value = !value
			render()
		case keyEnter:
			sc.clear()
			return value
		case keyCancel:
			sc.clear()
			return false
		}
	}
}

// Option is a prompt entry; Checked supplies a checklist's initial selection.
type Option struct {
	Label   string
	Hint    string
	Checked bool
}

// Choose presents a single-choice list and returns the selected index: def
// when there is no terminal, and -1 when the user backs out with Escape, q
// or Ctrl-C, which callers treat as "do nothing".
func Choose(title string, options []Option, def int) int {
	if !Interactive() || len(options) == 0 {
		return def
	}
	st, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return def
	}
	defer term.Restore(int(os.Stdin.Fd()), st)
	index := def
	if index < 0 || index >= len(options) {
		index = 0
	}
	var sc screen
	render := func() {
		lines := []string{title}
		for i, o := range options {
			mark, label := "  ", o.Label
			if i == index {
				mark, label = "> ", o.Label
			}
			line := mark + label
			if o.Hint != "" {
				line += "  " + o.Hint
			}
			lines = append(lines, line)
		}
		sc.render(lines)
	}
	render()
	for {
		switch readKey() {
		case keyUp:
			if index > 0 {
				index--
				render()
			}
		case keyDown:
			if index < len(options)-1 {
				index++
				render()
			}
		case keyEnter:
			sc.clear()
			return index
		case keyCancel:
			sc.clear()
			return -1
		}
	}
}

// Checklist returns selected option indices and false on cancellation. Without
// a terminal it returns the initial choices. It never mutates the options.
func Checklist(title string, options []Option) ([]int, bool) {
	checked := make([]bool, len(options))
	for i, option := range options {
		checked[i] = option.Checked
	}
	selection := func() []int {
		selected := []int{}
		for i, value := range checked {
			if value {
				selected = append(selected, i)
			}
		}
		return selected
	}
	if !Interactive() || len(options) == 0 {
		return selection(), true
	}
	state, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return selection(), true
	}
	defer term.Restore(int(os.Stdin.Fd()), state)
	index := 0
	var sc screen
	render := func() {
		lines := []string{title + " (Space to toggle, Enter to continue)"}
		for i, option := range options {
			cursor, mark := "  ", "[ ]"
			if i == index {
				cursor = "> "
			}
			if checked[i] {
				mark = "[x]"
			}
			line := cursor + mark + " " + option.Label
			if option.Hint != "" {
				line += "  " + option.Hint
			}
			lines = append(lines, line)
		}
		sc.render(lines)
	}
	render()
	for {
		switch readKey() {
		case keyUp:
			if index > 0 {
				index--
				render()
			}
		case keyDown:
			if index < len(options)-1 {
				index++
				render()
			}
		case keySpace:
			checked[index] = !checked[index]
			render()
		case keyYes:
			checked[index] = true
			render()
		case keyNo:
			checked[index] = false
			render()
		case keyEnter:
			sc.clear()
			return selection(), true
		case keyCancel:
			sc.clear()
			return nil, false
		}
	}
}
