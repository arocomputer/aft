package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// defaultHost selects only the configured default workspace's SSH destination.
func defaultHost() (string, error) { _, w, e := selectedWorkspace(""); return w.Host, e }

// serviceDirectory returns the OS-required user-unit directory without creating it.
func serviceDirectory() (string, error) {
	if runtime.GOOS == "darwin" {
		h, e := os.UserHomeDir()
		return filepath.Join(h, "Library", "LaunchAgents"), e
	}
	c, e := os.UserConfigDir()
	return filepath.Join(c, "systemd", "user"), e
}

// runPaths prints effective locations, including legacy paths for explicit migration.
func runPaths(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("paths takes no arguments")
	}
	r, e := aftRoot()
	if e != nil {
		return e
	}
	h, e := os.UserHomeDir()
	if e != nil {
		return e
	}
	units, e := serviceDirectory()
	if e != nil {
		return e
	}
	legacy, e := os.UserConfigDir()
	if e != nil {
		return e
	}
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	fmt.Printf("metadata: %s\nconfig: %s\nprojects: %s\nlogs: %s\nintended executable: %s\nrunning executable: %s\nuser services: %s\nlegacy projects (migration source only): %s\n", r, filepath.Join(r, "config.json"), filepath.Join(r, "projects"), filepath.Join(r, "logs"), filepath.Join(h, ".local", "bin", "aft"), exe, units, filepath.Join(legacy, "aft", "projects"))
	return nil
}

// runStatus performs bounded read-only remote and local listener diagnostics.
func runStatus(args []string) error {
	f := flag.NewFlagSet("status", flag.ContinueOnError)
	name := f.String("project", "", "saved project")
	if e := f.Parse(args); e != nil {
		return e
	}
	if f.NArg() > 1 || (*name != "" && f.NArg() != 0) {
		return fmt.Errorf("usage: aft status [--project NAME] [HOST]")
	}
	host := ""
	if *name != "" {
		p, e := loadProject(*name)
		if e != nil {
			return e
		}
		host = p.Host
	} else if f.NArg() == 1 {
		host = f.Arg(0)
	} else {
		var e error
		host, e = defaultHost()
		if e != nil {
			return e
		}
	}
	if !validHost(host) {
		return fmt.Errorf("invalid SSH host")
	}
	fmt.Printf("SSH: %s\n", host)
	data, remoteErr := remoteRead(host, `printf 'connected\nsessions:\n'; if command -v tmux >/dev/null 2>&1; then tmux list-sessions 2>&1 || :; else printf 'tmux unavailable\n'; fi; printf 'remote IPv4 listeners:\n'; if command -v ss >/dev/null 2>&1; then ss -H -ltn4; else printf 'ss unavailable\n'; fi`)
	fmt.Println(data)
	fmt.Println("Local TCP listeners (observation only; ownership and end-to-end forwarding are not verified):")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.CommandContext(ctx, "lsof", "-nP", "-iTCP", "-sTCP:LISTEN")
	} else {
		cmd = exec.CommandContext(ctx, "ss", "-H", "-ltnp")
	}
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if e := cmd.Run(); e != nil {
		fmt.Printf("local listener inspection unavailable: %v\n", e)
	}
	return remoteErr
}

// runUninstall inventories cleanup candidates without following links or deleting files.
func runUninstall(args []string) error {
	f := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	dry := f.Bool("dry-run", false, "show cleanup inventory")
	yes := f.Bool("yes", false, "request removal (not supported)")
	if e := f.Parse(args); e != nil {
		return e
	}
	if f.NArg() != 0 || !*dry || *yes {
		return fmt.Errorf("only aft uninstall --dry-run is supported; executable ownership cannot be proven")
	}
	if e := runPaths(nil); e != nil {
		return e
	}
	r, e := aftRoot()
	if e != nil {
		return e
	}
	fmt.Println("Metadata inventory (review manually; no deletion):")
	if e = safeParents(filepath.Dir(r)); e != nil {
		return e
	}
	e = filepath.WalkDir(r, func(file string, d os.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		fmt.Printf("  %s [%s]\n", file, d.Type())
		return nil
	})
	if e != nil {
		return e
	}
	units, e := serviceDirectory()
	if e != nil {
		return e
	}
	if e = safeParents(units); e != nil {
		return e
	}
	entries, e := os.ReadDir(units)
	if e != nil && !os.IsNotExist(e) {
		return e
	}
	fmt.Println("Aft-named user service files (remove via aft service remove HOST after review):")
	for _, d := range entries {
		if strings.HasPrefix(d.Name(), "dev.aft.ports.") {
			file := filepath.Join(units, d.Name())
			label := "unverified; preserve"
			if d.Type().IsRegular() {
				b, e := os.ReadFile(file)
				if e != nil {
					return e
				}
				if strings.Contains(string(b), serviceMarker) {
					label = "Aft marker found; service-manager stop required"
				}
			}
			fmt.Printf("  %s [%s]\n", file, label)
		}
	}
	home, e := os.UserHomeDir()
	if e != nil {
		return e
	}
	executable := filepath.Join(home, ".local", "bin", "aft")
	if info, e := os.Lstat(executable); e == nil {
		fmt.Printf("Executable candidate: %s [%s; ownership unverified, preserve]\n", executable, info.Mode())
	} else if !os.IsNotExist(e) {
		return e
	}
	legacy, e := os.UserConfigDir()
	if e != nil {
		return e
	}
	cache, e := os.UserCacheDir()
	if e != nil {
		return e
	}
	for _, dir := range []string{filepath.Join(legacy, "aft"), filepath.Join(cache, "aft")} {
		if info, e := os.Lstat(dir); e == nil {
			fmt.Printf("Legacy metadata/logs: %s [%s; preserve, not migrated automatically]\n", dir, info.Mode())
		} else if !os.IsNotExist(e) {
			return e
		}
	}
	fmt.Println("Preserve repositories, remote files, all tmux sessions, SSH/shell config, legacy metadata, arbitrary executables and symlink targets. No installation ownership manifest exists; executable removal is manual. Service removal leaves logs. No files were changed.")
	return nil
}
