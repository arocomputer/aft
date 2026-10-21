package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// runDoctor performs read-only dependency and disk checks; it never installs tools.
func runDoctor(args []string) error {
	if len(args) == 0 {
		host, err := defaultHost()
		if err != nil {
			return err
		}
		args = []string{host}
	}
	if len(args) != 1 || !validHost(args[0]) {
		return fmt.Errorf("usage: aft doctor [HOST]")
	}
	p, err := exec.LookPath("ssh")
	if err != nil {
		return err
	}
	fmt.Printf("local ssh: %s\n", p)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	script := `missing=0
printf 'remote OS: '; uname -srm
for tool in tmux ss; do
  if command -v "$tool" >/dev/null 2>&1; then printf 'remote %s: available\n' "$tool";
  else printf 'remote %s: missing\n' "$tool"; missing=1; fi
done
printf 'remote home disk:\n'; df -Pk "$HOME"
exit "$missing"`
	cmd := exec.CommandContext(ctx, "ssh", "-T", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "UpdateHostKeys=no", "-o", "ConnectTimeout=5", "-o", "ConnectionAttempts=1", "--", args[0], "sh -c "+quote(script))
	cmd.WaitDelay = time.Second
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("remote checks failed: %w", err)
	}
	return nil
}
