// Package ui is how aft's commands talk to people: small interactive prompts
// for setup and configuration, and the shared output styles, spinner and
// tables in out.go. The session client, which must pass the terminal through
// untouched, uses only the output, and only once it has restored the terminal. Each prompt falls back to a default when there is no terminal, so
// scripted and piped runs never block.
//
// The prompts are driven directly in raw mode rather than with a TUI framework:
// Bubble Tea queries the terminal (an OSC 11 background probe) merely on import,
// which would corrupt a session client sharing the terminal. Nothing in this
// package writes to or reads from the terminal until a prompt actually runs.
package ui

import (
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

func styles() (accent, dim, cursor lipgloss.Style) {
	accent = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
	dim = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	cursor = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	return
}

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
	for _, l := range lines {
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

// Confirm asks a yes/no question, returning def when there is no terminal.
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
	accent, dim, _ := styles()
	render := func() {
		yes, no := "  Yes  ", "  No  "
		if value {
			yes = accent.Render("[ Yes ]")
		} else {
			no = accent.Render("[ No ]")
		}
		sc.render([]string{accent.Render("? ") + question, "  " + yes + " " + no, dim.Render("  y / n / arrows / enter")})
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
			return def
		}
	}
}

// Option is one entry in a Choose list.
type Option struct {
	Label string
	Hint  string
}

// Choose presents a single-choice list and returns the selected index, or def
// when there is no terminal.
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
	accent, dim, cursor := styles()
	var sc screen
	render := func() {
		lines := []string{accent.Render("? ") + title}
		for i, o := range options {
			mark, label := "  ", o.Label
			if i == index {
				mark, label = cursor.Render("> "), accent.Render(o.Label)
			}
			line := mark + label
			if o.Hint != "" {
				line += dim.Render("  " + o.Hint)
			}
			lines = append(lines, line)
		}
		sc.render(append(lines, dim.Render("  arrows / enter")))
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
			return def
		}
	}
}
