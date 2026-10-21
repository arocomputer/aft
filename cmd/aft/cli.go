package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// sortedWorkspaces returns deterministic local names for lists and completions.
func sortedWorkspaces(c configuration) []string {
	var names []string
	for n := range c.Workspaces {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// projectNames lists local record names without networking or creating directories.
func projectNames() ([]string, error) {
	r, e := projectRoot()
	if e != nil {
		return nil, e
	}
	if e = safeParents(r); e != nil {
		return nil, e
	}
	entries, e := os.ReadDir(r)
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	var names []string
	for _, entry := range entries {
		n := strings.TrimSuffix(entry.Name(), ".json")
		if entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), ".json") && sessionName.MatchString(n) {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return names, nil
}

// guardProject prepends containment checks to the same SSH invocation as the workload.
func guardProject(ssh []string, p project) ([]string, error) {
	if p.Workspace == "" {
		return ssh, nil
	}
	_, w, e := selectedWorkspace(p.Workspace)
	if e != nil {
		return nil, e
	}
	if p.Host != w.Host || !strings.HasPrefix(p.Dir, w.Root+"/") || path.Clean(p.Dir) != p.Dir {
		return nil, fmt.Errorf("project no longer belongs to workspace")
	}
	last := len(ssh) - 1
	ssh[last] = "sh -c " + quote(containmentScript(w.Root, p.Dir)+"exec "+ssh[last])
	return ssh, nil
}

// executionArgs resolves projects explicitly and passes command arguments literally.
func executionArgs(args []string) ([]string, error) {
	split := -1
	for i, a := range args {
		if a == "--" {
			split = i
			break
		}
	}
	if split < 0 || split == len(args)-1 {
		return nil, fmt.Errorf("usage: aft %s [flags] PROJECT -- COMMAND [ARG...]", args[0])
	}
	f := flag.NewFlagSet(args[0], flag.ContinueOnError)
	host := f.String("host", "", "explicit SSH destination (omit PROJECT)")
	dir := f.String("dir", "", "host-mode directory")
	session := f.String("session", "", "new detached session name")
	if e := f.Parse(args[1:split]); e != nil {
		return nil, e
	}
	var p project
	var e error
	if *host != "" {
		if f.NArg() != 0 || !validHost(*host) {
			return nil, fmt.Errorf("--host requires a valid host and no project")
		}
	} else {
		if f.NArg() != 1 || *dir != "" {
			return nil, fmt.Errorf("expected one project; --dir requires --host")
		}
		p, e = loadProject(f.Arg(0))
		if e != nil {
			return nil, fmt.Errorf("unknown or invalid project %q: %w", f.Arg(0), e)
		}
		*host = p.Host
		*dir = p.Dir
	}
	low := []string{args[0], "--dir", *dir}
	if args[0] == "run" {
		if *session == "" {
			if p.Name != "" {
				*session = "aft-" + p.Name
			} else {
				*session = "aft-run"
			}
		}
		low = append(low, "--session", *session)
	} else if *session != "" {
		return nil, fmt.Errorf("--session is only valid for run")
	}
	low = append(low, *host)
	low = append(low, args[split+1:]...)
	ssh, e := sshArgs(low, io.Discard)
	if e != nil {
		return nil, e
	}
	return guardProject(ssh, p)
}

// resolvePortProject substitutes an explicit project host without changing port semantics.
func resolvePortProject(args []string) ([]string, error) {
	result := []string{args[0]}
	var host string
	for i := 1; i < len(args); i++ {
		a := args[i]
		if a == "--project" || strings.HasPrefix(a, "--project=") {
			if host != "" {
				return nil, fmt.Errorf("duplicate --project")
			}
			n := strings.TrimPrefix(a, "--project=")
			if a == "--project" {
				i++
				if i >= len(args) {
					return nil, fmt.Errorf("--project requires a name")
				}
				n = args[i]
			}
			p, e := loadProject(n)
			if e != nil {
				return nil, e
			}
			host = p.Host
		} else {
			result = append(result, a)
		}
	}
	if host == "" {
		return result, nil
	}
	insert := 1
	if len(result) > 1 && (result[1] == "list" || result[1] == "watch") {
		insert = 2
	}
	for insert < len(result) && strings.HasPrefix(result[insert], "--") {
		if strings.Contains(result[insert], "=") {
			insert++
		} else {
			insert += 2
		}
	}
	if insert > len(result) {
		return nil, fmt.Errorf("missing port option value")
	}
	result = append(result, "")
	copy(result[insert+1:], result[insert:])
	result[insert] = host
	return result, nil
}

// terminalInput permits prompting only when stdin and stdout are terminal devices.
func terminalInput() bool {
	in, e := os.Stdin.Stat()
	if e != nil {
		return false
	}
	out, e := os.Stdout.Stat()
	if e != nil || in.Mode()&os.ModeCharDevice == 0 || out.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for _, f := range []*os.File{os.Stdin, os.Stdout} {
		cmd := exec.CommandContext(ctx, "stty", "-g")
		cmd.Stdin = f
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		cmd.WaitDelay = time.Second
		if cmd.Run() != nil {
			return false
		}
	}
	return true
}

// pickProject presents local records and opens only a deliberate numeric selection.
func pickProject() int {
	names, e := projectNames()
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		return 1
	}
	if len(names) == 0 {
		fmt.Println("No projects. Run aft setup, then aft add PATH or aft new NAME.")
		return 0
	}
	for i, n := range names {
		fmt.Printf("%d) %s\n", i+1, n)
	}
	fmt.Print("Open project (number; empty cancels): ")
	line, e := bufio.NewReader(os.Stdin).ReadString('\n')
	if e != nil {
		return 0
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return 0
	}
	n, e := strconv.Atoi(line)
	if e != nil || n < 1 || n > len(names) {
		fmt.Fprintln(os.Stderr, "Invalid selection")
		return 2
	}
	return run([]string{"open", names[n-1]})
}

// migrateProjects preflights every legacy record and copies without replacement or deletion.
func migrateProjects(args []string) error {
	f := flag.NewFlagSet("project migrate", flag.ContinueOnError)
	from := f.String("from", "", "legacy projects directory")
	if e := f.Parse(args); e != nil {
		return e
	}
	if f.NArg() != 0 {
		return fmt.Errorf("migrate accepts --from only")
	}
	if *from == "" {
		c, e := os.UserConfigDir()
		if e != nil {
			return e
		}
		*from = filepath.Join(c, "aft", "projects")
	}
	if e := safeParents(*from); e != nil {
		return e
	}
	entries, e := os.ReadDir(*from)
	if e != nil {
		return e
	}
	var records []project
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		var p project
		if e = readJSON(filepath.Join(*from, entry.Name()), &p); e != nil {
			return e
		}
		if e = validateProject(p); e != nil {
			return e
		}
		if entry.Name() != p.Name+".json" || p.Workspace != "" {
			return fmt.Errorf("not a legacy project record: %s", entry.Name())
		}
		dest, e := projectPath(p.Name)
		if e != nil {
			return e
		}
		if _, e = os.Lstat(dest); !os.IsNotExist(e) {
			return fmt.Errorf("refusing overwrite of %s", dest)
		}
		records = append(records, p)
	}
	for _, p := range records {
		if e = saveProject(p); e != nil {
			return fmt.Errorf("migration stopped; previously copied records and originals retained: %w", e)
		}
	}
	fmt.Printf("Copied %d records; originals retained in %s\n", len(records), *from)
	return nil
}

// runCompletion emits shell functions or local-only dynamic completion candidates.
func runCompletion(action string, args []string) error {
	if action == "_complete" {
		if len(args) != 1 {
			return fmt.Errorf("expected projects or workspaces")
		}
		switch args[0] {
		case "projects":
			names, e := projectNames()
			if e != nil {
				return e
			}
			for _, n := range names {
				fmt.Println(n)
			}
		case "workspaces":
			c, e := loadConfig()
			if e != nil {
				return e
			}
			for _, n := range sortedWorkspaces(c) {
				fmt.Println(n)
			}
		default:
			return fmt.Errorf("invalid completion kind")
		}
		return nil
	}
	if len(args) != 1 {
		return fmt.Errorf("usage: aft completion bash|zsh")
	}
	switch args[0] {
	case "bash":
		fmt.Print(`_aft_complete() {
  local cur prev kind
  cur=${COMP_WORDS[COMP_CWORD]}; prev=${COMP_WORDS[COMP_CWORD-1]}
  kind=projects
  if [[ $prev == --workspace || ${COMP_WORDS[1]} == workspace ]]; then kind=workspaces; fi
  if (( COMP_CWORD == 1 )); then
    COMPREPLY=( $(compgen -W 'setup workspace new add project open exec run connect sessions ports doctor status paths completion uninstall service help' -- "$cur") )
  else
    COMPREPLY=( $(compgen -W "$(aft _complete "$kind" 2>/dev/null)" -- "$cur") )
  fi
}
complete -F _aft_complete aft
`)
	case "zsh":
		fmt.Print(`#compdef aft
_aft() {
  local kind=projects
  local -a candidates
  if [[ $words[CURRENT-1] == --workspace || $words[2] == workspace ]]; then kind=workspaces; fi
  if (( CURRENT == 2 )); then
    candidates=(setup workspace new add project open exec run connect sessions ports doctor status paths completion uninstall service help)
  else
    candidates=("${(@f)$(aft _complete "$kind" 2>/dev/null)}")
  fi
  compadd -- $candidates
}
compdef _aft aft
`)
	default:
		return fmt.Errorf("supported shells: bash, zsh")
	}
	return nil
}
