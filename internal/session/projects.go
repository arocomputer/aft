package session

import (
	"os"
	"path/filepath"
	"strings"
)

// ProjectSessions keeps agent sessions, plus host shells currently inside a project.
// Project paths belong to this host; ~ and symlinks are resolved here. General
// host connections stay running even when they are absent from this public list.
func ProjectSessions(records []Record, directories []string) []Record {
	var roots []string
	for _, dir := range directories {
		if dir == "~" || strings.HasPrefix(dir, "~/") {
			h, err := os.UserHomeDir()
			if err != nil {
				continue
			}
			dir = filepath.Join(h, strings.TrimPrefix(dir, "~"))
		}
		if !filepath.IsAbs(dir) {
			continue
		}
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			roots = append(roots, real)
		}
	}
	var out []Record
	for _, r := range records {
		if !IsShell(r.Command) {
			out = append(out, r)
			continue
		}
		// An ended shell has no current directory. Agent remains still appear.
		if r.Ended != nil || r.PID <= 0 || len(roots) == 0 {
			continue
		}
		dir, err := processDirectory(r.PID)
		if err != nil {
			continue // never classify an unknown cwd using the startup directory
		}
		for _, root := range roots {
			rel, err := filepath.Rel(root, dir)
			if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				r.Dir = dir
				out = append(out, r)
				break
			}
		}
	}
	return out
}
