package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestSSHSignalCleanup verifies terminating Aft also terminates and reaps its SSH child.
func TestSSHSignalCleanup(t *testing.T) {
	if os.Getenv("AFT_TEST_SSH_CHILD") == "1" {
		os.Exit(executeSSH([]string{"unused"}))
	}
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "pid")
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte("#!/bin/sh\necho $$ > \"$AFT_TEST_PID\"\nexec sleep 60\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSSHSignalCleanup$")
	cmd.Env = append(os.Environ(), "AFT_TEST_SSH_CHILD=1", "AFT_TEST_PID="+pidFile, "PATH="+dir+":"+os.Getenv("PATH"))
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	var pid int
	for ctx.Err() == nil {
		data, err := os.ReadFile(pidFile)
		if err == nil {
			pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
			if pid > 0 {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("SSH child did not start")
	}
	defer syscall.Kill(pid, syscall.SIGKILL)
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	if ctx.Err() != nil {
		t.Fatal("Aft did not stop promptly")
	}
	if err := syscall.Kill(pid, 0); err != syscall.ESRCH {
		t.Fatalf("SSH child remains after Aft exits: %v", err)
	}
}
