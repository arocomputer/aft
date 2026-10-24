package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/arocomputer/aft/internal/bridge/transport"
	"github.com/arocomputer/aft/internal/config"
	"github.com/arocomputer/aft/internal/home"
	"github.com/arocomputer/aft/internal/launcher"
	"github.com/arocomputer/aft/internal/session"
	"github.com/arocomputer/aft/internal/session/install"
	"github.com/arocomputer/aft/internal/terminal"
	"github.com/arocomputer/aft/internal/ui"
)

func flags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

func ask(prompt, def string) string {
	if def != "" {
		fmt.Printf("%s [%s]: ", prompt, def)
	} else {
		fmt.Printf("%s: ", prompt)
	}
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	return line
}

// cmdSetup adds a server and prepares this machine.
func cmdSetup(args []string) int {
	fs := flags("setup")
	hostFlag := fs.String("host", "", "SSH destination, usually an alias from ~/.ssh/config")
	name := fs.String("name", "", "name for the host (default: the SSH destination)")
	yes := fs.Bool("yes", false, "do not ask; also skip the server if --host is empty")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		return fail(err)
	}
	st, err := config.LoadState()
	if err != nil {
		return fail(err)
	}
	self, err := os.Executable()
	if err != nil {
		return fail(err)
	}
	self, _ = filepath.EvalSymlinks(self)

	dest := *hostFlag
	if dest == "" && !*yes {
		dest = ask("SSH destination for your server (alias from ~/.ssh/config; empty to skip)", "")
	}
	if dest != "" {
		n := *name
		if n == "" {
			n = dest
		}
		cfg.Hosts[n] = config.HostConfig{SSH: dest}
		h, _ := cfg.Host(n)
		fmt.Printf("Connecting to %s…\n", dest)
		if err := transport.EnsureMaster(h); err != nil {
			return fail(err)
		}
		info, err := install.Ensure(h)
		if err != nil {
			return fail(err)
		}
		fmt.Printf("Server ready on %s (%s/%s).\n", n, info.OS, info.Arch)
		found := agentsIn(info.Agents)
		if len(found) == 0 {
			fmt.Println("No agents found on the server yet. Install one there (claude, codex, opencode, pi) and run aft doctor.")
		} else {
			fmt.Printf("Agents on the server: %s\n", strings.Join(found, ", "))
		}
	}
	if len(cfg.Agents) == 0 {
		cfg.Agents = config.DefaultAgents
	}
	if err := launcher.InstallLaunchers(self, cfg.Agents, &st); err != nil {
		return fail(err)
	}
	fmt.Printf("Launchers for %s are in %s.\n", strings.Join(cfg.Agents, ", "), home.Launchers())

	// Decide how the PATH line reaches the shell. People who manage their
	// dotfiles want to add it themselves, so ask rather than edit blindly.
	if cfg.EditDotfiles == nil && !*yes {
		b := chooseEditDotfiles()
		cfg.EditDotfiles = &b
	}
	if cfg.EditingDotfiles() {
		changed, err := launcher.InstallShellBlock(&st)
		if err != nil {
			return fail(err)
		}
		for _, f := range changed {
			ui.Note("added the aft PATH line to %s", f)
		}
		if len(changed) > 0 {
			fmt.Println("Open a new terminal so it takes effect.")
		}
	} else {
		fmt.Println("Add this line wherever your shell sets PATH, then apply it with your dotfile manager:")
		fmt.Println("\n    export PATH=\"$HOME/.aft/launchers:$PATH\"")
	}
	if err := config.Save(cfg); err != nil {
		return fail(err)
	}
	if err := config.SaveState(st); err != nil {
		return fail(err)
	}
	fmt.Println("Next: cd into a project and run aft on, then start your agent as usual.")
	return 0
}

// chooseEditDotfiles asks whether aft should edit the shell config itself,
// defaulting to no when the shell config looks managed.
func chooseEditDotfiles() bool {
	def := 0
	if launcher.DotfilesManaged() {
		def = 1
	}
	i := ui.Choose("How should aft put its launchers on your PATH?", []ui.Option{
		{Label: "Edit my shell config for me", Hint: "~/.zshrc or ~/.bashrc"},
		{Label: "Show me the line, I manage my dotfiles", Hint: "chezmoi, stow, nix, a repo"},
	}, def)
	return i == 0
}

// cmdHost manages the [host.<name>] tables in config.toml. With no verb it
// lists them; add and rm edit the tables. Which host is used by default is the
// separate `use` key (see cmdUse). These only touch local config — removing a
// host leaves whatever is installed on it.
func cmdHost(args []string) int {
	cfg, err := config.Load()
	if err != nil {
		return fail(err)
	}
	verb := ""
	if len(args) > 0 {
		verb = args[0]
		args = args[1:]
	}
	switch verb {
	case "", "ls", "list":
		if len(cfg.Hosts) == 0 {
			fmt.Println("No hosts yet. Add one with: aft host add NAME SSH-DESTINATION")
			return 0
		}
		used := cfg.UsedHost()
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		for _, n := range cfg.HostNames() {
			mark := ""
			if n == used {
				mark = "(used by default)"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n", n, cfg.Hosts[n].SSH, mark)
		}
		w.Flush()
		return 0

	case "add":
		if len(args) != 2 {
			fmt.Fprintln(os.Stderr, "usage: aft host add NAME SSH-DESTINATION")
			return 2
		}
		name, dest := args[0], args[1]
		if _, ok := cfg.Hosts[name]; ok {
			return fail(fmt.Errorf("host %q already exists; remove it first or edit config.toml", name))
		}
		cfg.Hosts[name] = config.HostConfig{SSH: dest}
		if err := config.Save(cfg); err != nil {
			return fail(err)
		}
		fmt.Printf("Added host %s (%s). Installing the server…\n", name, dest)
		h, _ := cfg.Host(name)
		if err := transport.EnsureMaster(h); err != nil {
			fmt.Fprintf(os.Stderr, "aft: saved, but could not connect yet: %v\n", err)
			fmt.Fprintln(os.Stderr, "Fix the SSH destination, then run: aft doctor --fix")
			return 1
		}
		info, err := install.Ensure(h)
		if err != nil {
			return fail(err)
		}
		fmt.Printf("Server ready on %s (%s/%s).\n", name, info.OS, info.Arch)
		switch {
		case cfg.UsedHost() == name:
			fmt.Printf("%s is used by default.\n", name)
		default:
			fmt.Printf("Run aft use %s to make it the default, or pass --host %s per command.\n", name, name)
		}
		return 0

	case "rm", "remove":
		if len(args) != 1 {
			fmt.Fprintln(os.Stderr, "usage: aft host rm NAME")
			return 2
		}
		name := args[0]
		if _, ok := cfg.Hosts[name]; !ok {
			return fail(fmt.Errorf("unknown host %q", name))
		}
		delete(cfg.Hosts, name)
		if cfg.Use == name {
			cfg.Use = "" // fall back to the only remaining host, or to aft use
		}
		if err := config.Save(cfg); err != nil {
			return fail(err)
		}
		fmt.Printf("Removed host %s from your config. Whatever Aft installed on it is left as is; run aft uninstall on that machine to clean it up.\n", name)
		switch used := cfg.UsedHost(); {
		case used != "":
			fmt.Printf("%s is now used by default.\n", used)
		case len(cfg.Hosts) > 1:
			fmt.Println("Pick which host to use by default with: aft use NAME")
		}
		return 0
	}
	fmt.Fprintf(os.Stderr, "aft host: unknown verb %q (use: add, rm, or no verb to list)\n", verb)
	return 2
}

// cmdUse sets the host used when none is given (config key `use`), like
// `docker context use`. With no name it prints the current one.
func cmdUse(args []string) int {
	cfg, err := config.Load()
	if err != nil {
		return fail(err)
	}
	if len(args) == 0 {
		if used := cfg.UsedHost(); used != "" {
			fmt.Println(used)
			return 0
		}
		fmt.Fprintln(os.Stderr, "no host is used by default; set one with: aft use NAME")
		return 1
	}
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: aft use NAME")
		return 2
	}
	name := args[0]
	if _, ok := cfg.Hosts[name]; !ok {
		return fail(fmt.Errorf("unknown host %q; add it with aft host add", name))
	}
	cfg.Use = name
	if err := config.Save(cfg); err != nil {
		return fail(err)
	}
	fmt.Printf("Using %s by default.\n", name)
	return 0
}

func agentsIn(m map[string]string) []string {
	var out []string
	for _, a := range config.DefaultAgents {
		if _, ok := m[a]; ok {
			out = append(out, a)
		}
	}
	return out
}

// cmdOn maps the current directory to a server directory.
func cmdOn(args []string) int {
	fs := flags("on")
	hostName := fs.String("host", "", "host name from aft setup")
	dir := fs.String("dir", "", "directory on the server (default: same path relative to home)")
	noClone := fs.Bool("no-clone", false, "never offer to clone")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fail(err)
	}
	cfg, err := config.Load()
	if err != nil {
		return fail(err)
	}
	st, err := config.LoadState()
	if err != nil {
		return fail(err)
	}
	root := cwd
	repo, repoRoot, hasRepo := config.LoadRepo(cwd)
	if hasRepo {
		root = repoRoot
		if *hostName == "" {
			*hostName = repo.Host
		}
		if *dir == "" {
			*dir = repo.Dir
		}
	} else if gitRoot, err := gitTop(cwd); err == nil {
		root = gitRoot
	}
	// A project on a network mount already lives on the server; map it in place
	// with no clone and no sync. This runs unless the user forced --dir.
	mounted := false
	if *dir == "" {
		if mh, mr, ok := resolveLocalDir(&cfg, root); ok {
			if *hostName == "" {
				*hostName = mh
			}
			*dir = mr
			mounted = true
		}
	}
	h, err := cfg.Host(*hostName)
	if err != nil {
		return fail(err)
	}
	if *dir == "" {
		*dir = config.DefaultRemoteDir(root)
	}
	if err := transport.EnsureMaster(h); err != nil {
		return fail(err)
	}
	if _, err := install.Ensure(h); err != nil {
		return fail(err)
	}
	if !mounted {
		// Resolve a home-relative mapping to an absolute server path, since the
		// Mac and the server have different home directories and the session uses
		// the path as a real working directory, not a shell word.
		resolved, err := resolveRemoteDir(h, *dir)
		if err != nil {
			return fail(err)
		}
		*dir = resolved
	}
	exists, err := remoteDirExists(h, *dir)
	if err != nil {
		return fail(err)
	}
	if !exists {
		if mounted {
			return fail(fmt.Errorf("the mount maps to %s on %s, but that path is missing; check aft mount list", *dir, h.Name))
		}
		url := gitRemote(root)
		if url == "" || *noClone {
			return fail(fmt.Errorf("%s does not exist on %s; create it there or pass --dir", *dir, h.Name))
		}
		fmt.Printf("%s does not exist on %s.\n", *dir, h.Name)
		if !ui.Confirm(fmt.Sprintf("Clone %s there?", url), false) {
			return 1
		}
		if err := transport.Shell(h, "mkdir -p "+transport.Quote(filepath.Dir(*dir))+" && git clone "+transport.Quote(url)+" "+transport.Quote(*dir)); err != nil {
			return fail(fmt.Errorf("clone failed: %w", err))
		}
	}
	st.Projects[root] = config.Project{Host: h.Name, Dir: *dir}
	if err := config.SaveState(st); err != nil {
		return fail(err)
	}
	fmt.Printf("On. Agents started under %s run on %s in %s.\n", root, h.Name, *dir)
	if !launcher.OnPath() {
		fmt.Println("Note: Aft's launchers are not first on PATH in this shell. Open a new terminal or run aft doctor.")
	}
	return 0
}

// cmdOff removes the mapping for the current directory's project.
func cmdOff(args []string) int {
	cwd, err := os.Getwd()
	if err != nil {
		return fail(err)
	}
	st, err := config.LoadState()
	if err != nil {
		return fail(err)
	}
	root, _, ok := st.FindProject(cwd)
	if !ok {
		fmt.Println("Already off here.")
		return 0
	}
	delete(st.Projects, root)
	if err := config.SaveState(st); err != nil {
		return fail(err)
	}
	fmt.Printf("Off. Agents started under %s run locally.\n", root)
	return 0
}

// cmdClone clones on the server and creates a local handle that is on.
func cmdClone(args []string) int {
	fs := flags("clone")
	hostName := fs.String("host", "", "host name from aft setup")
	if err := fs.Parse(args); err != nil || fs.NArg() < 1 || fs.NArg() > 2 {
		fmt.Fprintln(os.Stderr, "usage: aft clone [--host H] URL [DIR]")
		return 2
	}
	url := fs.Arg(0)
	local := fs.Arg(1)
	if local == "" {
		local = strings.TrimSuffix(filepath.Base(strings.TrimSuffix(url, "/")), ".git")
	}
	local, _ = filepath.Abs(local)
	cfg, err := config.Load()
	if err != nil {
		return fail(err)
	}
	h, err := cfg.Host(*hostName)
	if err != nil {
		return fail(err)
	}
	if err := transport.EnsureMaster(h); err != nil {
		return fail(err)
	}
	if _, err := install.Ensure(h); err != nil {
		return fail(err)
	}
	dir, err := resolveRemoteDir(h, config.DefaultRemoteDir(local))
	if err != nil {
		return fail(err)
	}
	if err := transport.Shell(h, "mkdir -p "+transport.Quote(filepath.Dir(dir))+" && git clone "+transport.Quote(url)+" "+transport.Quote(dir)); err != nil {
		return fail(fmt.Errorf("clone failed: %w", err))
	}
	if err := os.MkdirAll(local, 0o755); err != nil {
		return fail(err)
	}
	st, err := config.LoadState()
	if err != nil {
		return fail(err)
	}
	st.Projects[local] = config.Project{Host: h.Name, Dir: dir}
	if err := config.SaveState(st); err != nil {
		return fail(err)
	}
	fmt.Printf("Cloned to %s:%s. cd %s and start your agent.\n", h.Name, dir, local)
	return 0
}

// cmdShell opens a login shell on the server in a session.
func cmdShell(args []string) int {
	fs := flags("shell")
	hostName := fs.String("host", "", "host name from aft setup")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		return fail(err)
	}
	st, _ := config.LoadState()
	cwd, _ := os.Getwd()
	root, p, on := st.FindProject(cwd)
	if *hostName == "" && on {
		*hostName = p.Host
	}
	h, err := cfg.Host(*hostName)
	if err != nil {
		return fail(err)
	}
	opts, err := launcher.SessionOptions(cfg, *hostName, root, p)
	if err != nil {
		return fail(err)
	}
	opts.Host = h
	opts.Command = []string{"sh", "-c", `exec "${SHELL:-/bin/sh}" -l`}
	if on {
		opts.Dir = config.RemoteDir(root, p, cwd)
	}
	return terminal.Run(opts)
}

type hostSessions struct {
	host transport.Host
	list []session.Record
	err  error
}

func allSessions(cfg config.Config) []hostSessions {
	var out []hostSessions
	names := cfg.HostNames()
	if list, _ := session.List(); len(list) > 0 {
		out = append(out, hostSessions{host: transport.Local, list: list})
	}
	for _, n := range names {
		h, err := cfg.Host(n)
		if err != nil {
			continue
		}
		hs := hostSessions{host: h}
		if !transport.MasterAlive(h) {
			hs.err = errors.New("not connected")
			out = append(out, hs)
			continue
		}
		raw, err := transport.Output(h, nil, "ls")
		if err != nil {
			hs.err = err
		} else {
			json.Unmarshal(raw, &hs.list)
		}
		out = append(out, hs)
	}
	return out
}

// cmdLs lists sessions on every host.
func cmdLs(args []string) int {
	cfg, err := config.Load()
	if err != nil {
		return fail(err)
	}
	type row struct{ host, id, cmd, dir, started string }
	var rows []row
	for _, hs := range allSessions(cfg) {
		if hs.err != nil {
			rows = append(rows, row{hs.host.Name, "(" + hs.err.Error() + ")", "", "", ""})
			continue
		}
		for _, r := range hs.list {
			rows = append(rows, row{hs.host.Name, r.ID, strings.Join(r.Command, " "), r.Dir, r.Started})
		}
	}
	if len(rows) == 0 {
		fmt.Println("No sessions.")
		return 0
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "HOST\tSESSION\tCOMMAND\tDIRECTORY\tSTARTED")
	for _, r := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", r.host, r.id, r.cmd, r.dir, r.started)
	}
	w.Flush()
	return 0
}

// findSession locates a session id across hosts.
func findSession(cfg config.Config, id string) (transport.Host, error) {
	for _, hs := range allSessions(cfg) {
		for _, r := range hs.list {
			if r.ID == id {
				return hs.host, nil
			}
		}
	}
	return transport.Host{}, fmt.Errorf("no session %q on any connected host", id)
}

// cmdAttach reattaches to a session.
func cmdAttach(args []string) int {
	fs := flags("attach")
	hostName := fs.String("host", "", "host the session is on")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		return fail(err)
	}
	id := fs.Arg(0)
	var h transport.Host
	if id == "" {
		all := allSessions(cfg)
		var found []struct {
			h transport.Host
			r session.Record
		}
		for _, hs := range all {
			for _, r := range hs.list {
				found = append(found, struct {
					h transport.Host
					r session.Record
				}{hs.host, r})
			}
		}
		if len(found) == 0 {
			return fail(errors.New("no sessions to attach to"))
		}
		if len(found) > 1 {
			fmt.Fprintln(os.Stderr, "aft: several sessions; pass an id:")
			for _, f := range found {
				fmt.Fprintf(os.Stderr, "  %s on %s: %s\n", f.r.ID, f.h.Name, strings.Join(f.r.Command, " "))
			}
			return 2
		}
		h, id = found[0].h, found[0].r.ID
	} else if *hostName != "" {
		h, err = cfg.Host(*hostName)
		if err != nil {
			return fail(err)
		}
	} else {
		h, err = findSession(cfg, id)
		if err != nil {
			return fail(err)
		}
	}
	opts, err := launcher.SessionOptions(cfg, h.Name, "", config.Project{})
	if err != nil {
		return fail(err)
	}
	opts.Host = h
	opts.SessionID = id
	return terminal.Run(opts)
}

// cmdControl ends a session or detaches its client.
func cmdControl(what string, args []string) int {
	fs := flags(what)
	hostName := fs.String("host", "", "host the session is on")
	if err := fs.Parse(args); err != nil || fs.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "usage: aft %s [--host H] ID\n", map[string]string{"kick": "detach", "end": "end"}[what])
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		return fail(err)
	}
	id := fs.Arg(0)
	var h transport.Host
	if *hostName != "" {
		h, err = cfg.Host(*hostName)
	} else {
		h, err = findSession(cfg, id)
	}
	if err != nil {
		return fail(err)
	}
	if _, err := transport.Output(h, nil, what, id); err != nil {
		return fail(err)
	}
	if what == "end" {
		fmt.Println("Ended", id)
	} else {
		fmt.Println("Detached", id)
	}
	return 0
}

// cmdStatus summarizes hosts, connections, sessions and mirrors.
func cmdStatus(args []string) int {
	cfg, err := config.Load()
	if err != nil {
		return fail(err)
	}
	st, _ := config.LoadState()
	cwd, _ := os.Getwd()
	if root, p, ok := st.FindProject(cwd); ok {
		fmt.Printf("Here: on (%s → %s:%s)\n", root, p.Host, p.Dir)
	} else {
		fmt.Println("Here: off")
	}
	for _, n := range cfg.HostNames() {
		h, _ := cfg.Host(n)
		state := "connected"
		if !transport.MasterAlive(h) {
			state = "not connected"
		}
		mark := ""
		if n == cfg.UsedHost() {
			mark = " (default)"
		}
		fmt.Printf("Host %s%s: %s, %s\n", n, mark, h.SSH, state)
	}
	for _, hs := range allSessions(cfg) {
		for _, r := range hs.list {
			fmt.Printf("Session %s on %s: %s in %s\n", r.ID, hs.host.Name, strings.Join(r.Command, " "), r.Dir)
		}
	}
	if len(st.Projects) > 0 {
		fmt.Println("Projects on:")
		roots := make([]string, 0, len(st.Projects))
		for r := range st.Projects {
			roots = append(roots, r)
		}
		sort.Strings(roots)
		for _, r := range roots {
			fmt.Printf("  %s → %s:%s\n", r, st.Projects[r].Host, st.Projects[r].Dir)
		}
	}
	return 0
}

// docFinding is one line of a doctor report: a check, its level, and if it is
// not ok, the exact command that fixes it. Fixed is set when --fix repaired it.
type docFinding struct {
	Section string `json:"section"`
	Level   string `json:"level"` // ok | info | warn | fix
	Text    string `json:"text"`
	Hint    string `json:"hint,omitempty"`
	Fixed   bool   `json:"fixed,omitempty"`
}

// doctor accumulates findings under the current section and, when fixing, runs
// each repair as its check is added.
type doctor struct {
	fix      bool
	cur      string
	findings []docFinding
}

func (d *doctor) section(s string) { d.cur = s }
func (d *doctor) ok(text string) {
	d.findings = append(d.findings, docFinding{d.cur, "ok", text, "", false})
}
func (d *doctor) info(text string) {
	d.findings = append(d.findings, docFinding{d.cur, "info", text, "", false})
}
func (d *doctor) warn(text, h string) {
	d.findings = append(d.findings, docFinding{d.cur, "warn", text, h, false})
}
func (d *doctor) bad(text, hint string) {
	d.findings = append(d.findings, docFinding{d.cur, "fix", text, hint, false})
}

// repair records a fixable finding. Under --fix it runs fn and, on success,
// turns the finding into an ok line describing what it did.
func (d *doctor) repair(text, hint string, fn func() error, done string) {
	f := docFinding{d.cur, "fix", text, hint, false}
	if d.fix {
		if err := fn(); err == nil {
			f.Level, f.Text, f.Hint, f.Fixed = "ok", text+" — "+done, "", true
		} else {
			f.Hint = err.Error()
		}
	}
	d.findings = append(d.findings, f)
}

// cmdDoctor inspects everything Aft depends on, grouped by where a problem would
// be, and prints the exact command to fix each one. --fix applies the safe
// repairs (reinstall a stale or missing server, re-add the PATH line); --json
// prints machine-readable findings; --host limits host checks to one host.
func cmdDoctor(args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	doFix := fs.Bool("fix", false, "apply the safe repairs")
	asJSON := fs.Bool("json", false, "print findings as JSON")
	only := fs.String("host", "", "check only this host")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		return fail(err)
	}
	st, _ := config.LoadState()
	self, _ := os.Executable()
	d := &doctor{fix: *doFix}

	d.section("This machine")
	d.info("aft " + version + ", home " + home.Dir())
	if _, err := exec.LookPath("ssh"); err == nil {
		d.ok("ssh present")
	} else {
		d.bad("ssh missing", "install OpenSSH")
	}
	switch {
	case launcher.OnPath():
		d.ok("launchers on PATH")
	case cfg.EditingDotfiles():
		d.repair("launchers not on PATH", "run aft setup", func() error {
			if _, err := launcher.InstallShellBlock(&st); err != nil {
				return err
			}
			return config.SaveState(st)
		}, "added the PATH line; open a new terminal")
	default:
		d.bad("launchers not on PATH (manual mode)", `add $HOME/.aft/launchers to your PATH`)
	}
	for _, a := range cfg.Agents {
		if !launcher.Installed(a) {
			d.repair("launcher "+a+" missing", "run aft setup", func() error {
				return launcher.InstallLaunchers(self, cfg.Agents, &st)
			}, "reinstalled launchers")
			continue
		}
		if _, err := launcher.RealBinary(a); err == nil {
			d.info("launcher " + a + " (local copy found)")
		} else {
			d.info("launcher " + a + " (no local copy; runs only on a host)")
		}
	}

	d.section("Here")
	cwd, _ := os.Getwd()
	if root, p, ok := st.FindProject(cwd); ok {
		d.info("on: " + root + " → " + p.Host + ":" + p.Dir)
	} else {
		d.info("off (aft on makes agents here run on a host)")
	}

	if len(cfg.Hosts) == 0 {
		d.section("Hosts")
		d.bad("no host configured", "run aft setup")
	}
	for _, n := range cfg.HostNames() {
		if *only != "" && n != *only {
			continue
		}
		title := "Host " + n
		if n == cfg.UsedHost() {
			title += " (default)"
		}
		d.section(title)
		h, _ := cfg.Host(n)
		if err := transport.EnsureMaster(h); err != nil {
			d.bad("cannot connect ("+h.SSH+")", err.Error())
			continue
		}
		d.ok("connected (" + h.SSH + ")")
		info, err := install.Info(h)
		switch {
		case err != nil || info.Version == "":
			d.repair("server not installed", "run aft setup", func() error { _, e := install.Ensure(h); return e }, "installed the server")
		case !info.Shims:
			d.repair("bridge shims missing", "run aft setup", func() error { _, e := install.Ensure(h); return e }, "reinstalled the server")
		default:
			if ok, e := install.UpToDate(info); e == nil && !ok {
				d.repair("server out of date ("+info.Version+")", "run aft setup", func() error { _, e := install.Ensure(h); return e }, "updated the server")
			} else {
				d.ok("server ready (" + info.OS + "/" + info.Arch + ", " + info.Version + ")")
			}
			if found := agentsIn(info.Agents); len(found) > 0 {
				d.info("agents: " + strings.Join(found, ", "))
			} else {
				d.warn("no agents on the host", "install one there (claude, codex, ...)")
			}
		}
	}

	if *asJSON {
		b, _ := json.MarshalIndent(d.findings, "", "  ")
		fmt.Println(string(b))
	} else {
		d.print()
	}
	fixes := 0
	for _, f := range d.findings {
		if f.Level == "fix" {
			fixes++
		}
	}
	if fixes > 0 {
		return 1
	}
	return 0
}

// print writes the findings grouped by section, each line marked by level.
func (d *doctor) print() {
	mark := map[string]string{"ok": "ok  ", "info": "    ", "warn": "warn", "fix": "fix "}
	section, fixes, warns := "", 0, 0
	for _, f := range d.findings {
		if f.Section != section {
			fmt.Printf("\n%s\n", f.Section)
			section = f.Section
		}
		line := "  " + mark[f.Level] + " " + f.Text
		if f.Hint != "" {
			line += " — " + f.Hint
		}
		fmt.Println(line)
		switch f.Level {
		case "fix":
			fixes++
		case "warn":
			warns++
		}
	}
	fmt.Println()
	switch {
	case fixes > 0 && d.fix:
		fmt.Printf("%d still to fix (could not be repaired automatically).\n", fixes)
	case fixes > 0:
		fmt.Printf("%d to fix, %d recommended. Run aft doctor --fix to apply the safe repairs.\n", fixes, warns)
	case warns > 0:
		fmt.Printf("All good, %d recommended improvement(s) above.\n", warns)
	default:
		fmt.Println("Everything looks good.")
	}
}

// cmdExec runs one command on the server without a session.
func cmdExec(args []string) int {
	fs := flags("exec")
	hostName := fs.String("host", "", "host name from aft setup")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) > 0 && rest[0] == "--" {
		rest = rest[1:]
	}
	if len(rest) == 0 {
		fmt.Fprintln(os.Stderr, "usage: aft exec [--host H] -- COMMAND [ARG...]")
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		return fail(err)
	}
	st, _ := config.LoadState()
	cwd, _ := os.Getwd()
	root, p, on := st.FindProject(cwd)
	if *hostName == "" && on {
		*hostName = p.Host
	}
	h, err := cfg.Host(*hostName)
	if err != nil {
		return fail(err)
	}
	if err := transport.EnsureMaster(h); err != nil {
		return fail(err)
	}
	script := ""
	if on {
		rdir, err := resolveRemoteDir(h, config.RemoteDir(root, p, cwd))
		if err != nil {
			return fail(err)
		}
		script = "cd " + transport.Quote(rdir) + " && "
	}
	q := make([]string, len(rest))
	for i, a := range rest {
		q[i] = transport.Quote(a)
	}
	script += strings.Join(q, " ")
	if err := transport.Shell(h, script); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode()
		}
		return fail(err)
	}
	return 0
}

// cmdPorts shows the mirrored ports recorded by running clients.
func cmdPorts(args []string) int {
	entries, _ := os.ReadDir(home.Sockets())
	any := false
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "mirrors-") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(home.Sockets(), e.Name()))
		if err != nil {
			continue
		}
		var ports []int
		if json.Unmarshal(b, &ports) == nil && len(ports) > 0 {
			any = true
			fmt.Printf("%s: %v\n", strings.TrimSuffix(strings.TrimPrefix(e.Name(), "mirrors-"), ".json"), ports)
		}
	}
	if !any {
		fmt.Println("No ports are mirrored right now.")
	}
	return 0
}

// cmdAgents adds a launcher for another program.
func cmdAgents(args []string) int {
	if len(args) != 2 || args[0] != "add" {
		fmt.Fprintln(os.Stderr, "usage: aft agents add NAME")
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		return fail(err)
	}
	st, err := config.LoadState()
	if err != nil {
		return fail(err)
	}
	self, err := os.Executable()
	if err != nil {
		return fail(err)
	}
	self, _ = filepath.EvalSymlinks(self)
	name := args[1]
	for _, a := range cfg.Agents {
		if a == name {
			fmt.Println(name, "already has a launcher.")
			return 0
		}
	}
	cfg.Agents = append(cfg.Agents, name)
	if err := launcher.InstallLaunchers(self, []string{name}, &st); err != nil {
		return fail(err)
	}
	if err := config.Save(cfg); err != nil {
		return fail(err)
	}
	if err := config.SaveState(st); err != nil {
		return fail(err)
	}
	fmt.Printf("%s now runs on the server wherever aft is on.\n", name)
	return 0
}

// cmdUninstall removes everything Aft added on every machine.
func cmdUninstall(args []string) int {
	fs := flags("uninstall")
	yes := fs.Bool("yes", false, "do not ask for confirmation")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		return fail(err)
	}
	st, err := config.LoadState()
	if err != nil {
		return fail(err)
	}
	fmt.Println("This will:")
	for _, n := range cfg.HostNames() {
		fmt.Printf("  end Aft sessions and remove ~/.aft and pasted files on %s\n", n)
	}
	for _, b := range st.ShellBlocks {
		fmt.Printf("  remove the aft block from %s\n", b.File)
	}
	fmt.Printf("  remove %s (launchers, config, state)\n", home.Dir())
	fmt.Println("Your code, SSH config and each agent's own data stay.")
	if !*yes && !ui.Confirm("Continue?", false) {
		return 1
	}
	code := 0
	for _, n := range cfg.HostNames() {
		h, _ := cfg.Host(n)
		if err := transport.EnsureMaster(h); err != nil {
			fmt.Fprintf(os.Stderr, "aft: could not reach %s: %v (remove ~/.aft there by hand)\n", n, err)
			code = 1
			continue
		}
		if _, err := transport.Output(h, nil, "uninstall"); err != nil {
			fmt.Fprintf(os.Stderr, "aft: %s: %v\n", n, err)
			code = 1
		} else {
			fmt.Printf("Removed Aft from %s.\n", n)
		}
		transport.CloseMaster(h)
	}
	if list, _ := session.List(); len(list) > 0 {
		for _, r := range list {
			session.Control(r.ID, 0x06)
		}
	}
	removed, kept := launcher.RemoveShellBlocks(&st)
	for _, f := range removed {
		fmt.Printf("Removed the aft block from %s.\n", f)
	}
	for _, f := range kept {
		fmt.Printf("Left %s alone: its aft block was edited. Remove the lines between %q and %q yourself.\n", f, "# >>> aft >>>", "# <<< aft <<<")
		code = 1
	}
	launcher.RemoveLaunchers(&st)
	if err := os.RemoveAll(home.Dir()); err != nil {
		return fail(err)
	}
	os.RemoveAll(home.Temp()) // pasted files, if the local host received any
	fmt.Printf("Removed %s.\n", home.Dir())
	if self, err := os.Executable(); err == nil {
		fmt.Printf("The aft binary itself is at %s; delete it however you installed it.\n", self)
	}
	return code
}

// cmdCompletion prints shell completion for the top-level commands.
func cmdCompletion(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: aft completion bash|zsh|fish")
		return 2
	}
	words := "setup on off clone shell ls attach detach end status doctor exec ports agents uninstall completion version help"
	switch args[0] {
	case "bash":
		fmt.Printf("_aft() { COMPREPLY=( $(compgen -W '%s' -- \"${COMP_WORDS[COMP_CWORD]}\") ); }\ncomplete -F _aft aft\n", words)
	case "zsh":
		fmt.Printf("#compdef aft\n_aft() { compadd -- %s; }\ncompdef _aft aft\n", words)
	case "fish":
		for _, w := range strings.Fields(words) {
			fmt.Printf("complete -c aft -n '__fish_use_subcommand' -a %s\n", w)
		}
	default:
		fmt.Fprintln(os.Stderr, "supported shells: bash, zsh, fish")
		return 2
	}
	return 0
}

func gitTop(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitRemote(dir string) string {
	out, err := exec.Command("git", "-C", dir, "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func remoteDirExists(h transport.Host, dir string) (bool, error) {
	out, err := transport.RunRaw(h, nil, "test -d "+transport.Quote(dir)+" && echo yes || echo no")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) == "yes", nil
}

// resolveRemoteDir expands a leading ~ in a server path to the server's real
// home directory, so the stored path is absolute. The Mac and the server have
// different home directories, and the session uses the path as a real
// working directory, not a shell word, so a literal ~ would never expand.
func resolveRemoteDir(h transport.Host, dir string) (string, error) {
	if dir != "~" && !strings.HasPrefix(dir, "~/") {
		return dir, nil
	}
	out, err := transport.RunRaw(h, nil, `printf %s "$HOME"`)
	if err != nil {
		return "", err
	}
	home := strings.TrimSpace(string(out))
	if home == "" || !strings.HasPrefix(home, "/") {
		return "", fmt.Errorf("could not resolve the home directory on %s", h.Name)
	}
	return expandHome(dir, home), nil
}

// expandHome replaces a leading ~ or ~/ in dir with home.
func expandHome(dir, home string) string {
	if dir == "~" {
		return home
	}
	if strings.HasPrefix(dir, "~/") {
		return home + dir[1:]
	}
	return dir
}
