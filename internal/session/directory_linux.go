package session

import (
	"os"
	"path/filepath"
	"strconv"
)

// processDirectory reads the shell's own cwd, rather than a foreground child's.
func processDirectory(pid int) (string, error) {
	return os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "cwd"))
}
