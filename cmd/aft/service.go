package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/xml"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const serviceMarker = "Managed by aft: port mirrors only"

// serviceID separates host configurations without interpolating host names into paths.
func serviceID(host string) string {
	sum := sha256.Sum256([]byte(host))
	return fmt.Sprintf("dev.aft.ports.%x", sum[:8])
}

// unitWord quotes a systemd argument, including its specifier/environment expansions.
func unitWord(s string) string {
	s = strings.ReplaceAll(s, "%", "%%")
	s = strings.ReplaceAll(s, "$", "$$")
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return "\"" + s + "\""
}

// xmlText encodes untrusted paths and hosts as plist text, not XML markup.
func xmlText(s string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

// serviceContent generates user-only units; no system configuration or root is needed.
func serviceContent(platform, executable, host, log string, min, max int) (string, error) {
	for _, s := range []string{executable, host, log} {
		if strings.ContainsAny(s, "\x00\r\n") {
			return "", fmt.Errorf("service values cannot contain control characters")
		}
	}
	args := []string{executable, "ports", "watch", "--min", strconv.Itoa(min), "--max", strconv.Itoa(max), host}
	switch platform {
	case "linux":
		var words []string
		for _, a := range args {
			words = append(words, unitWord(a))
		}
		return "# " + serviceMarker + "\n[Unit]\nDescription=Aft localhost port mirrors\n\n[Service]\nExecStart=" + strings.Join(words, " ") + "\nStandardOutput=append:" + strings.ReplaceAll(log, "%", "%%") + "\nStandardError=append:" + strings.ReplaceAll(log, "%", "%%") + "\nRestart=on-failure\nRestartSec=5\nKillMode=control-group\nUMask=0077\n\n[Install]\nWantedBy=default.target\n", nil
	case "darwin":
		var words []string
		for _, a := range args {
			words = append(words, "<string>"+xmlText(a)+"</string>")
		}
		return "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<!-- " + serviceMarker + " -->\n<plist version=\"1.0\"><dict>\n<key>Label</key><string>" + serviceID(host) + "</string>\n<key>ProgramArguments</key><array>" + strings.Join(words, "") + "</array>\n<key>RunAtLoad</key><true/>\n<key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict>\n<key>ThrottleInterval</key><integer>5</integer>\n<key>StandardOutPath</key><string>" + xmlText(log) + "</string>\n<key>StandardErrorPath</key><string>" + xmlText(log) + "</string>\n</dict></plist>\n", nil
	default:
		return "", fmt.Errorf("background service supports macOS and Linux only")
	}
}

// manager runs a service-management command with bounded runtime and visible errors.
func manager(name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = time.Second
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

// runService installs or removes only Aft-owned user port-watch units.
func runService(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: aft service install [--min PORT --max PORT] HOST | status HOST | remove HOST")
	}
	action := args[0]
	if action != "install" && action != "status" && action != "remove" {
		return fmt.Errorf("unknown service action %q", action)
	}
	f := flag.NewFlagSet("service "+action, flag.ContinueOnError)
	min, max := 3000, 9999
	if action == "install" {
		f.IntVar(&min, "min", min, "lowest mirrored port")
		f.IntVar(&max, "max", max, "highest mirrored port")
	}
	if err := f.Parse(args[1:]); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if f.NArg() != 1 || !validHost(f.Arg(0)) || min < 1024 || max > 65535 || min > max {
		return fmt.Errorf("expected SSH host and valid 1024–65535 range")
	}
	host := f.Arg(0)
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	id := serviceID(host)
	var file string
	domain := "gui/" + strconv.Itoa(os.Getuid())
	switch runtime.GOOS {
	case "darwin":
		file = filepath.Join(home, "Library", "LaunchAgents", id+".plist")
	case "linux":
		config, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		file = filepath.Join(config, "systemd", "user", id+".service")
	default:
		return fmt.Errorf("background service supports macOS and Linux only")
	}
	unit := id + ".service"
	if action == "status" {
		if runtime.GOOS == "darwin" {
			return manager("launchctl", "print", domain+"/"+id)
		}
		return manager("systemctl", "--user", "status", "--no-pager", unit)
	}
	if action == "remove" {
		if err := safeParents(filepath.Dir(file)); err != nil {
			return err
		}
		info, err := os.Lstat(file)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refusing non-regular service file")
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		if !strings.Contains(string(data), serviceMarker) {
			return fmt.Errorf("refusing service not marked as Aft-owned")
		}
		if runtime.GOOS == "darwin" {
			if err := manager("launchctl", "bootout", domain+"/"+id); err != nil {
				if check := serviceStopped("darwin", domain+"/"+id); check != nil {
					return fmt.Errorf("stop service before removal: %w; %v", err, check)
				}
			}
		} else if err := manager("systemctl", "--user", "disable", "--now", unit); err != nil {
			if check := serviceStopped("linux", unit); check != nil {
				return fmt.Errorf("cannot confirm stopped service: %w; %v", err, check)
			}
			if err := manager("systemctl", "--user", "disable", unit); err != nil {
				return err
			}
		}
		if err := os.Remove(file); err != nil {
			return err
		}
		if runtime.GOOS == "linux" {
			return manager("systemctl", "--user", "daemon-reload")
		}
		return nil
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return err
	}
	root, err := aftRoot()
	if err != nil {
		return err
	}
	log := filepath.Join(root, "logs", id+".log")
	content, err := serviceContent(runtime.GOOS, executable, host, log, min, max)
	if err != nil {
		return err
	}
	if err := privateDir(filepath.Dir(file)); err != nil {
		return err
	}
	if err := privateDir(filepath.Dir(log)); err != nil {
		return err
	}
	if info, err := os.Lstat(log); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("refusing non-regular log")
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	l, err := os.OpenFile(log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	l.Close()
	output, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("refusing to overwrite service; remove it first: %w", err)
	}
	if _, err := output.WriteString(content); err != nil {
		output.Close()
		os.Remove(file)
		return err
	}
	if err := output.Close(); err != nil {
		os.Remove(file)
		return err
	}
	fmt.Println("Installed", file)
	if runtime.GOOS == "darwin" {
		err = manager("launchctl", "bootstrap", domain, file)
	} else {
		err = manager("systemctl", "--user", "daemon-reload")
		if err == nil {
			err = manager("systemctl", "--user", "enable", "--now", unit)
		}
	}
	if err != nil {
		return fmt.Errorf("service file retained at %s; activation failed (check user session): %w", file, err)
	}
	fmt.Println("Port mirroring enabled. Authentication must work without prompts; check service status.")
	return nil
}

// serviceStopped distinguishes unloaded/failed units from manager access failures.
func serviceStopped(platform, target string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if platform == "linux" {
		b, e := exec.CommandContext(ctx, "systemctl", "--user", "show", "--property=ActiveState", "--value", target).CombinedOutput()
		state := strings.TrimSpace(string(b))
		if e == nil && (state == "inactive" || state == "failed") {
			return nil
		}
		return fmt.Errorf("service state not safely inactive: %s (%v)", state, e)
	}
	b, e := exec.CommandContext(ctx, "launchctl", "print", target).CombinedOutput()
	if _, ok := e.(*exec.ExitError); ok && (strings.Contains(string(b), "Could not find service") || strings.Contains(string(b), "Could not find specified service")) {
		return nil
	}
	return fmt.Errorf("service is loaded or launchd state unavailable: %v", e)
}
