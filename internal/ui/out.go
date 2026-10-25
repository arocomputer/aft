package ui

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/term"
)

// Only stderr warnings and errors use color. Renderer creation never probes or
// writes to the terminal; piped output and NO_COLOR retain readable labels.
var stderr = lipgloss.NewRenderer(os.Stderr)

// Prefix labels warnings and errors; routine output needs no marker.
func Prefix(level string) string {
	switch level {
	case "warn":
		return "warning: "
	case "fix":
		return "error: "
	}
	return ""
}

// OK reports a finished action on stdout.
func OK(format string, a ...any) {
	fmt.Printf(format+"\n", a...)
}

// Note reports a change on stderr.
func Note(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
}

// Warn prints a labeled warning to stderr.
func Warn(format string, a ...any) {
	fmt.Fprintln(os.Stderr, stderr.NewStyle().Foreground(lipgloss.Color("3")).Render("warning:")+" "+fmt.Sprintf(format, a...))
}

// Error prints a labeled error to stderr.
func Error(format string, a ...any) {
	fmt.Fprintln(os.Stderr, stderr.NewStyle().Foreground(lipgloss.Color("1")).Render("error:")+" "+fmt.Sprintf(format, a...))
}

// Hint prints recovery instructions indented beneath a warning or error.
func Hint(format string, a ...any) {
	fmt.Fprintln(os.Stderr, "  "+fmt.Sprintf(format, a...))
}

// Suggest prints the command the user probably meant beneath an error.
func Suggest(cmd string) {
	Hint("Did you mean %s?", cmd)
}

// Path shortens a local path under the home directory to ~.
func Path(p string) string {
	h, err := os.UserHomeDir()
	if err != nil || h == "" {
		return p
	}
	if p == h {
		return "~"
	}
	if strings.HasPrefix(p, h+string(filepath.Separator)) {
		return "~" + p[len(h):]
	}
	return p
}

// Ago renders how long ago t was, coarsely: "just now", "5m ago", "3h ago".
func Ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

// Table prints aligned columns with an optional header. Widths ignore styles.
func Table(header []string, rows [][]string) {
	cols := len(header)
	for _, r := range rows {
		cols = max(cols, len(r))
	}
	widths := make([]int, cols)
	for _, r := range append([][]string{header}, rows...) {
		for i, c := range r {
			if w := lipgloss.Width(c); i < len(widths) && w > widths[i] {
				widths[i] = w
			}
		}
	}
	line := func(r []string) {
		var b strings.Builder
		for i, c := range r {
			b.WriteString(c)
			if i < len(r)-1 && i < len(widths) {
				b.WriteString(strings.Repeat(" ", max(widths[i]-lipgloss.Width(c), 0)+2))
			}
		}
		fmt.Println(b.String())
	}
	if len(header) > 0 {
		line(header)
	}
	for _, r := range rows {
		line(r)
	}
}

// Live is a spinner on stderr for work that takes a moment. Lines printed
// through it land above the spinner, leaving a clean log when it stops.
// When stderr is not a terminal it draws nothing and only passes lines through.
type Live struct {
	mu          sync.Mutex
	tty         bool
	label       string
	frame       int
	paused      bool
	stop, ended chan struct{}
	sig         chan os.Signal
}

var spinFrames = []string{"|", "/", "-", "\\"}

// Start begins a spinner showing label.
func Start(label string) *Live {
	l := &Live{label: label, tty: term.IsTerminal(int(os.Stderr.Fd())), stop: make(chan struct{}), ended: make(chan struct{})}
	if !l.tty {
		close(l.ended)
		return l
	}
	// Restore the cursor if interrupted while it is hidden.
	l.sig = make(chan os.Signal, 1)
	signal.Notify(l.sig, os.Interrupt)
	fmt.Fprint(os.Stderr, "\x1b[?25l")
	go l.run()
	return l
}

func (l *Live) run() {
	defer close(l.ended)
	t := time.NewTicker(80 * time.Millisecond)
	defer t.Stop()
	for {
		l.mu.Lock()
		l.draw()
		l.mu.Unlock()
		select {
		case <-l.stop:
			return
		case <-l.sig:
			l.mu.Lock()
			fmt.Fprint(os.Stderr, "\r\x1b[K\x1b[?25h")
			os.Exit(130)
		case <-t.C:
			l.frame++
		}
	}
}

// draw renders the spinner line; the caller holds mu.
func (l *Live) draw() {
	if l.paused {
		return
	}
	line := spinFrames[l.frame%len(spinFrames)] + " " + l.label
	// A line wider than the terminal would wrap and break the redraw.
	if w, _, err := term.GetSize(int(os.Stderr.Fd())); err == nil && w > 0 && lipgloss.Width(line) >= w {
		line = ansi.Truncate(line, w-1, "")
	}
	fmt.Fprint(os.Stderr, "\r\x1b[K"+line)
}

// Step changes what the spinner says is happening.
func (l *Live) Step(label string) {
	l.mu.Lock()
	l.label = label
	l.mu.Unlock()
}

// Println prints a finished line to stdout above the spinner.
func (l *Live) Println(s string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.tty && !l.paused {
		fmt.Fprint(os.Stderr, "\r\x1b[K")
	}
	fmt.Println(s)
}

// Pause clears the spinner so something else can use the terminal, such as an
// SSH passphrase prompt; Resume brings it back.
func (l *Live) Pause() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.tty && !l.paused {
		fmt.Fprint(os.Stderr, "\r\x1b[K\x1b[?25h")
	}
	l.paused = true
}

func (l *Live) Resume() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.tty && l.paused {
		fmt.Fprint(os.Stderr, "\x1b[?25l")
	}
	l.paused = false
}

// Stop clears the spinner and shows the cursor again.
func (l *Live) Stop() {
	if !l.tty {
		return
	}
	select {
	case <-l.stop:
		return
	default:
		close(l.stop)
	}
	<-l.ended
	signal.Stop(l.sig)
	fmt.Fprint(os.Stderr, "\r\x1b[K\x1b[?25h")
}

// Spin shows label with a spinner while fn runs.
func Spin(label string, fn func() error) error {
	l := Start(label)
	defer l.Stop()
	return fn()
}
