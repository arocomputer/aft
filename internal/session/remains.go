package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/arocomputer/aft/internal/home"
	"github.com/arocomputer/aft/internal/vt"
)

// A session that ends with no terminal attached leaves remains, so nothing is
// lost without a word: its record, marked with how it ended, and its last
// screen as plain text. Both live in the logs directory, where people look.
// aft ls lists them, aft attach shows one and clears it, aft kill clears it.

// Ending says how a session ended when no one was watching.
type Ending struct {
	At     string `json:"at"`               // RFC 3339
	Code   int    `json:"code"`             // the application's exit code
	Signal string `json:"signal,omitempty"` // the signal that ended it, if one did
	Lost   bool   `json:"lost,omitempty"`   // its server died, so how is unknown
}

// keepRemains is how long remains stay before they are pruned.
const keepRemains = 7 * 24 * time.Hour

// ScreenPath is a session's last screen, kept as plain text.
func ScreenPath(id string) string { return filepath.Join(home.Logs(), id+".screen") }

func remainsPath(id string) string { return filepath.Join(home.Logs(), id+".ended.json") }

// screenText renders the scrollback and screen as plain text, keeping the
// last lines that matter.
func screenText(sd vt.SnapshotData) string {
	lines := make([]string, 0, len(sd.Lines))
	for _, row := range sd.Lines {
		var b strings.Builder
		for _, c := range row {
			switch {
			case c.Width == 0:
			case c.Rune == 0:
				b.WriteByte(' ')
			default:
				b.WriteRune(c.Rune)
			}
		}
		lines = append(lines, strings.TrimRight(b.String(), " "))
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > 2000 {
		lines = lines[len(lines)-2000:]
	}
	return strings.Join(lines, "\n") + "\n"
}

// leaveRemains records that r ended with e while no one was attached.
func leaveRemains(r Record, e Ending) {
	r.Ended = &e
	writeJSON(remainsPath(r.ID), r)
}

// Remains lists sessions that ended unseen, pruning old ones.
func Remains() []Record {
	entries, _ := os.ReadDir(home.Logs())
	var out []Record
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".ended.json")
		if !ok {
			continue
		}
		var r Record
		b, err := os.ReadFile(remainsPath(id))
		if err != nil || json.Unmarshal(b, &r) != nil || r.Ended == nil {
			continue
		}
		if at, err := time.Parse(time.RFC3339, r.Ended.At); err == nil && time.Since(at) > keepRemains {
			ClearRemains(id)
			continue
		}
		out = append(out, r)
	}
	return out
}

// ClearRemains forgets a session that ended unseen.
func ClearRemains(id string) {
	os.Remove(remainsPath(id))
	os.Remove(ScreenPath(id))
}
