// Package launcher makes `claude`, `codex` and friends start on the server when
// the current project is on, and run normally when it is off. It owns the shim
// directory on PATH and the block Aft adds to shell config files, and it knows
// how to remove both exactly.
package launcher

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/arocomputer/aft/internal/client"
	"github.com/arocomputer/aft/internal/config"
	"github.com/arocomputer/aft/internal/home"
	"github.com/arocomputer/aft/internal/ports"
	"github.com/arocomputer/aft/internal/session"
	"github.com/arocomputer/aft/internal/transport"
)

// PassEnv are variables that always cross to the server.
var PassEnv = []string{"TERM", "COLORTERM", "TERM_PROGRAM", "TERM_PROGRAM_VERSION", "LANG", "LC_ALL", "LC_CTYPE"}

// Launch runs when aft is invoked through a shim named after an agent.
func Launch(name string, args []string) int {
	cwd, err := os.Getwd()
	if err != nil {
		return execReal(name, args)
	}
	st, err := config.LoadState()
	if err != nil {
		fmt.Fprintln(os.Stderr, "aft:", err)
		return execReal(name, args)
	}
	root, p, on := st.FindProject(cwd)
	if !on {
		return execReal(name, args)
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "aft:", err)
		return 1
	}
	h, err := cfg.Host(p.Host)
	if err != nil {
		fmt.Fprintln(os.Stderr, "aft:", err)
		return 1
	}
	opts, err := SessionOptions(cfg, p.Host, root, p)
	if err != nil {
		fmt.Fprintln(os.Stderr, "aft:", err)
		return 1
	}
	opts.Host = h
	opts.Command = append([]string{name}, args...)
	opts.Dir = config.RemoteDir(root, p, cwd)
	// Reattach to an existing detached session for this agent in this directory
	// rather than starting a second one.
	if id := existingSession(h, name, opts.Dir); id != "" {
		fmt.Fprintf(os.Stderr, "aft: reattaching to %s (aft end %s for a fresh one)\n", id, id)
		opts.SessionID = id
	}
	return client.Run(opts)
}

// existingSession returns the id of a session running the same agent in the
// same server directory, or empty. Errors mean "start fresh".
func existingSession(h transport.Host, name, dir string) string {
	out, err := transport.Output(h, nil, "ls")
	if err != nil {
		return ""
	}
	var recs []session.Record
	if json.Unmarshal(out, &recs) != nil {
		return ""
	}
	for _, r := range recs {
		if len(r.Command) > 0 && r.Command[0] == name && r.Dir == dir {
			return r.ID
		}
	}
	return ""
}

// SessionOptions builds client options for a project, resolving settings from
// the global config, the host, and the project's aft.toml.
func SessionOptions(cfg config.Config, host, root string, p config.Project) (client.Options, error) {
	var repo *config.RepoConfig
	if root != "" {
		if r, _, ok := config.LoadRepo(root); ok {
			repo = &r
		}
	}
	s := cfg.Resolve(host, repo)
	rng, err := ports.ParseRange(s.Ports)
	if err != nil {
		return client.Options{}, err
	}
	detach, err := config.DetachByte(cfg.DetachKey)
	if err != nil {
		return client.Options{}, err
	}
	prefix, err := cfg.PrefixKey()
	if err != nil {
		return client.Options{}, err
	}
	return client.Options{
		Env:       Environment(s.Env),
		Wrapper:   s.Wrapper,
		Predict:   s.Predict,
		Ports:     rng,
		DetachKey: detach,
		Prefix:    prefix,
	}, nil
}

// Environment collects the allowlisted variables to send to the server: the
// base set plus whatever the resolved settings add.
func Environment(extra []string) []string {
	var out []string
	for _, k := range append(append([]string{}, PassEnv...), extra...) {
		if v, ok := os.LookupEnv(k); ok {
			out = append(out, k+"="+v)
		}
	}
	return out
}

// execReal replaces this process with the local program of the same name.
func execReal(name string, args []string) int {
	path, err := RealBinary(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: not installed on this machine (aft is off here)\n", name)
		return 127
	}
	if err := syscall.Exec(path, append([]string{name}, args...), os.Environ()); err != nil {
		fmt.Fprintln(os.Stderr, name+":", err)
		return 126
	}
	return 0
}

// RealBinary finds name on PATH, skipping Aft's shim directory.
func RealBinary(name string) (string, error) {
	shims, _ := filepath.EvalSymlinks(home.Shims())
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		real, _ := filepath.EvalSymlinks(dir)
		if real == shims || filepath.Clean(dir) == home.Shims() {
			continue
		}
		p := filepath.Join(dir, name)
		fi, err := os.Stat(p)
		if err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0 {
			return p, nil
		}
	}
	return "", exec.ErrNotFound
}

// InstallShims creates a launcher symlink for each agent that points at self.
func InstallShims(self string, agents []string, st *config.State) error {
	if err := home.Ensure(); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, s := range st.Shims {
		seen[s] = true
	}
	for _, a := range agents {
		if a == "" || a == "aft" || strings.ContainsAny(a, "/ \t") {
			continue
		}
		link := filepath.Join(home.Shims(), a)
		os.Remove(link)
		if err := os.Symlink(self, link); err != nil {
			return err
		}
		if !seen[a] {
			st.Shims = append(st.Shims, a)
			seen[a] = true
		}
	}
	st.Binary = self
	return nil
}

// RemoveShims deletes every launcher symlink Aft created.
func RemoveShims(st *config.State) {
	for _, a := range st.Shims {
		os.Remove(filepath.Join(home.Shims(), a))
	}
	st.Shims = nil
}

const blockStart = "# >>> aft >>>"
const blockEnd = "# <<< aft <<<"

// Block returns the lines added to a POSIX shell config.
func Block() string {
	return blockStart + "\nexport PATH=\"$HOME/.aft/shims:$PATH\"\n" + blockEnd + "\n"
}

// FishBlock is the content of the fish drop-in file.
func FishBlock() string {
	return blockStart + "\nfish_add_path --prepend --move $HOME/.aft/shims\n" + blockEnd + "\n"
}

// ShellFiles returns the config files Aft should put the block in, based on
// the shells present for this user.
func ShellFiles() (posix []string, fish string) {
	h, err := os.UserHomeDir()
	if err != nil {
		return nil, ""
	}
	shell := filepath.Base(os.Getenv("SHELL"))
	exists := func(p string) bool { _, err := os.Stat(p); return err == nil }
	switch shell {
	case "zsh":
		posix = append(posix, filepath.Join(h, ".zshrc"))
	case "bash":
		if exists(filepath.Join(h, ".bash_profile")) && !exists(filepath.Join(h, ".bashrc")) {
			posix = append(posix, filepath.Join(h, ".bash_profile"))
		} else {
			posix = append(posix, filepath.Join(h, ".bashrc"))
		}
	case "fish":
		fish = filepath.Join(h, ".config", "fish", "conf.d", "aft.fish")
	default:
		posix = append(posix, filepath.Join(h, ".profile"))
	}
	// Also cover the other common shell if its config exists.
	if shell != "zsh" && exists(filepath.Join(h, ".zshrc")) {
		posix = append(posix, filepath.Join(h, ".zshrc"))
	}
	if shell != "bash" && exists(filepath.Join(h, ".bashrc")) {
		posix = append(posix, filepath.Join(h, ".bashrc"))
	}
	if shell != "fish" && exists(filepath.Join(h, ".config", "fish", "config.fish")) {
		fish = filepath.Join(h, ".config", "fish", "conf.d", "aft.fish")
	}
	return posix, fish
}

// InstallShellBlock adds the PATH block to each shell file and records it.
func InstallShellBlock(st *config.State) ([]string, error) {
	posix, fish := ShellFiles()
	var changed []string
	recorded := map[string]bool{}
	for _, b := range st.ShellBlocks {
		recorded[b.File] = true
	}
	for _, f := range posix {
		if recorded[f] {
			continue
		}
		existing, err := os.ReadFile(f)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return changed, err
		}
		if strings.Contains(string(existing), blockStart) {
			st.ShellBlocks = append(st.ShellBlocks, config.ShellBlock{File: f, Block: Block()})
			continue
		}
		if len(existing) > 0 {
			if err := os.WriteFile(f+".aft-backup", existing, 0o600); err != nil {
				return changed, err
			}
		}
		content := string(existing)
		if content != "" && !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += "\n" + Block()
		if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
			return changed, err
		}
		st.ShellBlocks = append(st.ShellBlocks, config.ShellBlock{File: f, Block: Block()})
		changed = append(changed, f)
	}
	if fish != "" && !recorded[fish] {
		if err := os.MkdirAll(filepath.Dir(fish), 0o755); err != nil {
			return changed, err
		}
		if err := os.WriteFile(fish, []byte(FishBlock()), 0o644); err != nil {
			return changed, err
		}
		st.ShellBlocks = append(st.ShellBlocks, config.ShellBlock{File: fish, Block: FishBlock()})
		changed = append(changed, fish)
	}
	return changed, nil
}

// RemoveShellBlocks takes the block out of each recorded file. A block the user
// edited is left in place and reported, never deleted.
func RemoveShellBlocks(st *config.State) (removed, kept []string) {
	var remaining []config.ShellBlock
	for _, b := range st.ShellBlocks {
		if filepath.Base(b.File) == "aft.fish" {
			existing, err := os.ReadFile(b.File)
			if err == nil && string(existing) != b.Block {
				kept = append(kept, b.File)
				remaining = append(remaining, b)
				continue
			}
			os.Remove(b.File)
			removed = append(removed, b.File)
			continue
		}
		existing, err := os.ReadFile(b.File)
		if err != nil {
			removed = append(removed, b.File)
			continue
		}
		content := string(existing)
		i := strings.Index(content, b.Block)
		if i < 0 {
			if strings.Contains(content, blockStart) {
				kept = append(kept, b.File)
				remaining = append(remaining, b)
			}
			continue
		}
		before := content[:i]
		after := content[i+len(b.Block):]
		before = strings.TrimRight(before, "\n")
		if before != "" {
			before += "\n"
		}
		if err := os.WriteFile(b.File, []byte(before+after), 0o644); err != nil {
			kept = append(kept, b.File)
			remaining = append(remaining, b)
			continue
		}
		os.Remove(b.File + ".aft-backup")
		removed = append(removed, b.File)
	}
	st.ShellBlocks = remaining
	return removed, kept
}

// Installed reports whether name has a launcher shim.
func Installed(name string) bool {
	_, err := os.Lstat(filepath.Join(home.Shims(), name))
	return err == nil
}

// OnPath reports whether the shim directory comes first on PATH.
func OnPath() bool {
	dirs := filepath.SplitList(os.Getenv("PATH"))
	if len(dirs) == 0 {
		return false
	}
	real, _ := filepath.EvalSymlinks(dirs[0])
	shims, _ := filepath.EvalSymlinks(home.Shims())
	return dirs[0] == home.Shims() || (real != "" && real == shims)
}

// DotfilesManaged guesses whether the user manages their shell config with a
// tool, so setup can default to printing the PATH line instead of editing.
// It looks for a symlinked rc file (stow, dotfiles repos, nix) or a known
// manager on PATH (chezmoi, yadm, home-manager).
func DotfilesManaged() bool {
	h, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	for _, rc := range []string{".zshrc", ".bashrc", ".bash_profile", ".profile"} {
		if fi, err := os.Lstat(filepath.Join(h, rc)); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			return true
		}
	}
	for _, tool := range []string{"chezmoi", "yadm", "home-manager"} {
		if _, err := exec.LookPath(tool); err == nil {
			return true
		}
	}
	return false
}
