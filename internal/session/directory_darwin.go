package session

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// processDirectory reads the shell's cwd with the lsof supplied by macOS.
func processDirectory(pid int) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "lsof", "-a", "-p", strconv.Itoa(pid), "-d", "cwd", "-Fn").Output()
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "n/") {
			return line[1:], nil
		}
	}
	return "", fmt.Errorf("no working directory for process %d", pid)
}
