package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// workspace maps a local name to an SSH destination and canonical remote root.
type workspace struct {
	Host string `json:"host"`
	Root string `json:"root"`
}

// configuration contains only workspace mappings and the default selection.
type configuration struct {
	Workspaces map[string]workspace `json:"workspaces"`
	Default    string               `json:"default_workspace"`
}

// aftRoot resolves metadata without consulting legacy platform config locations.
func aftRoot() (string, error) { h, e := os.UserHomeDir(); return filepath.Join(h, ".aft"), e }

// safeParents refuses symlink or non-directory ancestors, including metadata roots.
func safeParents(dir string) error {
	for d := filepath.Clean(dir); ; d = filepath.Dir(d) {
		i, e := os.Lstat(d)
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		if e == nil && !i.IsDir() {
			return fmt.Errorf("refusing non-directory or symlink ancestor %s", d)
		}
		if filepath.Dir(d) == d {
			break
		}
	}
	return nil
}

// privateDir creates private directories only through ordinary directory ancestors.
func privateDir(dir string) error {
	if e := safeParents(dir); e != nil {
		return e
	}
	return os.MkdirAll(dir, 0700)
}

// readJSON loads bounded regular metadata without following symlink records.
func readJSON(file string, value any) error {
	if e := safeParents(filepath.Dir(file)); e != nil {
		return e
	}
	i, e := os.Lstat(file)
	if e != nil {
		return e
	}
	if !i.Mode().IsRegular() || i.Size() > 65536 {
		return fmt.Errorf("invalid metadata file %s", file)
	}
	b, e := os.ReadFile(file)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, value)
}

// loadConfig validates all mappings before using hand-edited metadata.
func loadConfig() (configuration, error) {
	c := configuration{Workspaces: map[string]workspace{}}
	r, e := aftRoot()
	if e != nil {
		return c, e
	}
	e = readJSON(filepath.Join(r, "config.json"), &c)
	if os.IsNotExist(e) {
		return c, nil
	}
	if e != nil {
		return c, e
	}
	if c.Workspaces == nil {
		c.Workspaces = map[string]workspace{}
	}
	for n, w := range c.Workspaces {
		if !sessionName.MatchString(n) || !validHost(w.Host) || !path.IsAbs(w.Root) || w.Root == "/" || path.Clean(w.Root) != w.Root || strings.ContainsAny(w.Root, "\x00\r\n") {
			return c, fmt.Errorf("invalid workspace %q", n)
		}
	}
	if c.Default != "" {
		if _, ok := c.Workspaces[c.Default]; !ok {
			return c, fmt.Errorf("default workspace is missing")
		}
	}
	return c, nil
}

// editConfig serializes writers and atomically replaces a private complete config.
func editConfig(change func(*configuration) error) error {
	r, e := aftRoot()
	if e != nil {
		return e
	}
	if e = privateDir(r); e != nil {
		return e
	}
	lock := filepath.Join(r, "config.lock")
	if e = os.Mkdir(lock, 0700); e != nil {
		return fmt.Errorf("config locked (inspect %s if a writer crashed): %w", lock, e)
	}
	defer os.Remove(lock)
	c, e := loadConfig()
	if e != nil {
		return e
	}
	if e = change(&c); e != nil {
		return e
	}
	f, e := os.CreateTemp(r, ".config-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if e = json.NewEncoder(f).Encode(c); e != nil {
		return e
	}
	if e = f.Sync(); e != nil {
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(f.Name(), filepath.Join(r, "config.json"))
}

// probeOutput caps captured diagnostic output so a remote probe cannot exhaust memory.
type probeOutput struct{ bytes.Buffer }

// Write rejects output beyond one MiB; command deadlines still bound process cleanup.
func (b *probeOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 1024*1024 {
		return 0, fmt.Errorf("remote output exceeds one MiB")
	}
	return b.Buffer.Write(p)
}

// remoteRead runs bounded noninteractive SSH; callers supply probes or explicit creation.
func remoteRead(host, script string) (string, error) {
	if !validHost(host) {
		return "", fmt.Errorf("invalid SSH host")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ssh", "-T", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "UpdateHostKeys=no", "-o", "ConnectTimeout=5", "-o", "ConnectionAttempts=1", "--", host, "sh -c "+quote(script))
	cmd.Stderr = os.Stderr
	cmd.WaitDelay = time.Second
	var output probeOutput
	cmd.Stdout = &output
	e := cmd.Run()
	if e != nil {
		return "", fmt.Errorf("SSH validation failed: %w", e)
	}
	return strings.TrimSuffix(output.String(), "\n"), nil
}

// canonicalRoot expands a quoted home prefix on the remote and reads its physical path.
func canonicalRoot(host, root string) (string, error) {
	if root == "" || strings.ContainsAny(root, "\x00\r\n") {
		return "", fmt.Errorf("invalid remote root")
	}
	expr := quote(root)
	if root == "~" {
		expr = `"$HOME"`
	} else if strings.HasPrefix(root, "~/") {
		expr = `"$HOME"/` + quote(root[2:])
	} else if !path.IsAbs(root) {
		return "", fmt.Errorf("root must be absolute or start with ~/")
	}
	result, e := remoteRead(host, "cd "+expr+" && pwd -P")
	if e != nil {
		return "", e
	}
	if result == "/" || !path.IsAbs(result) || path.Clean(result) != result || strings.ContainsAny(result, "\r\n\x00") {
		return "", fmt.Errorf("remote did not return a canonical absolute path")
	}
	return result, nil
}

// selectedWorkspace resolves an explicit name or the configured default, never a host fallback.
func selectedWorkspace(name string) (string, workspace, error) {
	c, e := loadConfig()
	if e != nil {
		return "", workspace{}, e
	}
	if name == "" {
		name = c.Default
	}
	w, ok := c.Workspaces[name]
	if !ok {
		return "", w, fmt.Errorf("unknown workspace %q; run aft setup", name)
	}
	return name, w, nil
}

// runWorkspace manages named mappings; removal never touches remote directories.
func runWorkspace(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: aft workspace add|list|default|remove")
	}
	switch args[0] {
	case "list":
		if len(args) != 1 {
			return fmt.Errorf("list takes no arguments")
		}
		c, e := loadConfig()
		if e != nil {
			return e
		}
		for _, n := range sortedWorkspaces(c) {
			w := c.Workspaces[n]
			mark := ""
			if n == c.Default {
				mark = " (default)"
			}
			fmt.Printf("%s%s\t%s\t%s\n", n, mark, w.Host, w.Root)
		}
		return nil
	case "add":
		f := flag.NewFlagSet("workspace add", flag.ContinueOnError)
		def := f.Bool("default", false, "select as default")
		if e := f.Parse(args[1:]); e != nil {
			return e
		}
		if f.NArg() != 3 || !sessionName.MatchString(f.Arg(0)) || !validHost(f.Arg(1)) {
			return fmt.Errorf("usage: aft workspace add [--default] NAME HOST ROOT")
		}
		root, e := canonicalRoot(f.Arg(1), f.Arg(2))
		if e != nil {
			return e
		}
		return editConfig(func(c *configuration) error {
			if _, ok := c.Workspaces[f.Arg(0)]; ok {
				return fmt.Errorf("workspace already exists")
			}
			c.Workspaces[f.Arg(0)] = workspace{f.Arg(1), root}
			if *def || c.Default == "" {
				c.Default = f.Arg(0)
			}
			return nil
		})
	case "default", "remove":
		if len(args) != 2 {
			return fmt.Errorf("expected workspace name")
		}
		return editConfig(func(c *configuration) error {
			if _, ok := c.Workspaces[args[1]]; !ok {
				return fmt.Errorf("unknown workspace %q", args[1])
			}
			if args[0] == "default" {
				c.Default = args[1]
			} else {
				ps, e := projectNames()
				if e != nil {
					return e
				}
				for _, n := range ps {
					p, e := loadProject(n)
					if e != nil {
						return e
					}
					if p.Workspace == args[1] {
						return fmt.Errorf("workspace is referenced by project %s", n)
					}
				}
				delete(c.Workspaces, args[1])
				if c.Default == args[1] {
					c.Default = ""
				}
			}
			return nil
		})
	}
	return fmt.Errorf("unknown workspace command %q", args[0])
}

// runSetup collects missing terminal values or accepts complete flags for scripts.
func runSetup(args []string) error {
	f := flag.NewFlagSet("setup", flag.ContinueOnError)
	name := f.String("name", "default", "workspace name")
	host := f.String("host", "", "existing SSH alias")
	root := f.String("root", "", "existing remote root")
	if e := f.Parse(args); e != nil {
		return e
	}
	if f.NArg() != 0 {
		return fmt.Errorf("setup accepts flags only")
	}
	if (*host == "" || *root == "") && terminalInput() {
		reader := bufio.NewReader(os.Stdin)
		for _, v := range []struct {
			label string
			p     *string
		}{{"SSH alias", host}, {"Remote root (e.g. ~/repos)", root}} {
			if *v.p == "" {
				fmt.Printf("%s: ", v.label)
				line, e := reader.ReadString('\n')
				if e != nil {
					return e
				}
				*v.p = strings.TrimSpace(line)
			}
		}
	}
	if *host == "" || *root == "" {
		return fmt.Errorf("scripting requires --host and --root")
	}
	return runWorkspace([]string{"add", "--default", *name, *host, *root})
}

// relativeProjectPath rejects traversal and ambiguous components before any SSH call.
func relativeProjectPath(value string) error {
	if value == "" || path.IsAbs(value) || strings.ContainsAny(value, "\x00\r\n") {
		return fmt.Errorf("expected a relative project path")
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("empty, . and .. path components are forbidden")
		}
	}
	return nil
}

// containmentScript requires unchanged physical root and a canonical contained directory.
func containmentScript(root, dir string) string {
	return "r=" + quote(root) + "; d=" + quote(dir) + `; [ "$(cd "$r" && pwd -P)" = "$r" ] || exit 1; p=$(cd "$d" && pwd -P) || exit 1; [ "$p" = "$d" ] || exit 1; case "$p" in "$r"/*) ;; *) exit 1;; esac; `
}

// registerProject validates existing paths or creates one leaf without overwriting anything.
func registerProject(action string, args []string) error {
	f := flag.NewFlagSet(action, flag.ContinueOnError)
	ws := f.String("workspace", "", "workspace name")
	name := f.String("name", "", "project alias")
	if e := f.Parse(args); e != nil {
		return e
	}
	if f.NArg() != 1 {
		return fmt.Errorf("expected one relative project path")
	}
	rel := f.Arg(0)
	if e := relativeProjectPath(rel); e != nil {
		return e
	}
	n, w, e := selectedWorkspace(*ws)
	if e != nil {
		return e
	}
	if *name == "" {
		*name = strings.Trim(regexp.MustCompile(`[^A-Za-z0-9_-]+`).ReplaceAllString(path.Base(rel), "-"), "-_")
		if len(*name) > 60 {
			*name = (*name)[:60]
		}
	}
	p := project{Name: *name, Host: w.Host, Dir: path.Join(w.Root, rel), Workspace: n}
	if e = validateProject(p); e != nil {
		return e
	}
	file, e := projectPath(p.Name)
	if e != nil {
		return e
	}
	if _, e = os.Lstat(file); !os.IsNotExist(e) {
		return fmt.Errorf("project record exists or is inaccessible")
	}
	script := containmentScript(w.Root, p.Dir) + "printf '%s\\n' \"$p\""
	if action == "new" {
		parent := path.Dir(p.Dir)
		check := containmentScript(w.Root, parent)
		if parent == w.Root {
			check = "r=" + quote(w.Root) + `; [ "$(cd "$r" && pwd -P)" = "$r" ] || exit 1; `
		}
		script = check + "cd " + quote(parent) + ` && [ "$(pwd -P)" = ` + quote(parent) + " ] && mkdir -- " + quote(path.Base(p.Dir)) + " || exit 1; " + script
	}
	if _, e = remoteRead(w.Host, script); e != nil {
		return e
	}
	if e = saveProject(p); e != nil {
		return fmt.Errorf("remote directory retained at %s; metadata save failed: %w", p.Dir, e)
	}
	fmt.Printf("Saved %s → %s:%s\n", p.Name, p.Host, p.Dir)
	return nil
}
