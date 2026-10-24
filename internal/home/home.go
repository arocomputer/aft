// Package home locates the directories Aft writes on either machine. Aft
// writes only these and the recorded shell block, which is what makes
// uninstall exact. There are three, each with one job.
//
// Dir holds what you read or edit:
//
//	~/.aft/
//	  config.toml   you:   hosts and preferences
//	  state.json    you:   which projects are on, and their host and directory
//	  logs/         host:  per-session logs
//
// Data holds the plumbing, which nobody needs to open:
//
//	~/.local/share/aft/
//	  launchers/    you:   the agent launcher symlinks
//	  server        host:  the installed server binary
//	  shims/        host:  the bridge shims (open, clipboard)
//	  sessions/     host:  one record per running session
//	  sockets/      host:  session, bridge and SSH connection sockets
//
// Temp holds disposable files, such as pasted files, in the platform's
// temporary directory.
//
// With AFT_HOME set (tests, the selftest, a server kept apart by
// AFT_REMOTE_HOME), Dir and Data are both $AFT_HOME and Temp is $AFT_HOME/tmp.
package home

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

// Dir returns $AFT_HOME, or ~/.aft.
func Dir() string {
	if d := os.Getenv("AFT_HOME"); d != "" {
		return d
	}
	h, err := os.UserHomeDir()
	if err != nil {
		h = "."
	}
	return filepath.Join(h, ".aft")
}

// Data returns $AFT_HOME, or ~/.local/share/aft on every platform. It is
// fixed rather than following XDG_DATA_HOME because the terminal names the
// server binary's path before it knows the server's environment. macOS has no
// better place: Application Support suits apps, and its space would end up
// in PATH.
func Data() string {
	if d := os.Getenv("AFT_HOME"); d != "" {
		return d
	}
	h, err := os.UserHomeDir()
	if err != nil {
		h = "."
	}
	return filepath.Join(h, ".local", "share", "aft")
}

// Launchers holds the agent launcher symlinks on a machine you type on.
func Launchers() string { return filepath.Join(Data(), "launchers") }

// Server is the path of the installed server binary on a host.
func Server() string { return filepath.Join(Data(), "server") }

// Shims holds the bridge shims a session puts on the host's PATH: open and the
// clipboard readers. It is separate from Launchers so an agent name on the host
// never resolves to a launcher, which would start a session inside a session.
func Shims() string { return filepath.Join(Data(), "shims") }

// Sessions holds one JSON record per live session on a host.
func Sessions() string { return filepath.Join(Data(), "sessions") }

// Sockets holds the session and bridge unix sockets on a host, and the SSH
// connection sockets on a machine you type on. A unix socket path is limited
// to 104 bytes on macOS, which bounds how long this path may be.
func Sockets() string { return filepath.Join(Data(), "sockets") }

// Logs holds per-session logs on a host.
func Logs() string { return filepath.Join(Dir(), "logs") }

// Temp is Aft's private directory for disposable files, such as files pasted
// from the machine you type on. It is not under Dir: it lives where each
// platform keeps larger temporary files. On macOS that is the per-user
// temporary directory, which only you can read and which the system clears
// of files unused for three days; over SSH $TMPDIR is often unset, so it is
// asked of the system. On Linux it is $TMPDIR when set, else /var/tmp, which
// is on disk (/tmp is often memory), survives reboots and is cleaned by the
// system after about a month. The aft-<uid> name keeps users apart in a
// shared directory; EnsureTemp creates it safely. With AFT_HOME set (tests,
// the selftest) it is $AFT_HOME/tmp, so nothing leaves the throwaway home.
func Temp() string {
	if d := os.Getenv("AFT_HOME"); d != "" {
		return filepath.Join(d, "tmp")
	}
	return filepath.Join(tempRoot(), fmt.Sprintf("aft-%d", os.Getuid()))
}

func tempRoot() string {
	if d := os.Getenv("TMPDIR"); d != "" {
		return d
	}
	if runtime.GOOS == "darwin" {
		if out, err := exec.Command("getconf", "DARWIN_USER_TEMP_DIR").Output(); err == nil {
			if d := strings.TrimSpace(string(out)); d != "" {
				return d
			}
		}
		return "/tmp"
	}
	return "/var/tmp"
}

// EnsureTemp creates Temp with private permissions and returns it. Because
// the parent may be shared by every user, it refuses a path that is a
// symlink or belongs to someone else instead of writing into it.
func EnsureTemp() (string, error) {
	d := Temp()
	if err := os.MkdirAll(d, 0o700); err != nil {
		return "", err
	}
	fi, err := os.Lstat(d)
	if err != nil {
		return "", err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !fi.IsDir() || !ok || int(st.Uid) != os.Getuid() {
		return "", fmt.Errorf("%s is not a directory owned by you; remove it and try again", d)
	}
	if fi.Mode().Perm() != 0o700 {
		if err := os.Chmod(d, 0o700); err != nil {
			return "", err
		}
	}
	return d, nil
}

// ShimNames are the commands a session intercepts on the host so a remote
// application can reach the machine you are typing on.
var ShimNames = []string{"open", "xdg-open", "pbpaste", "wl-paste", "xclip", "xsel"}

// InstallShims points every bridge shim at self, creating the directory. self
// must be a binary that handles these names as shims (aft or the server).
func InstallShims(self string) error {
	if err := os.MkdirAll(Shims(), 0o700); err != nil {
		return err
	}
	for _, n := range ShimNames {
		link := filepath.Join(Shims(), n)
		if cur, err := os.Readlink(link); err == nil && cur == self {
			continue
		}
		os.Remove(link)
		if err := os.Symlink(self, link); err != nil {
			return err
		}
	}
	return nil
}

// Ensure creates the standard directories with private permissions.
func Ensure() error {
	for _, d := range []string{Dir(), Data(), Launchers(), Shims(), Sessions(), Sockets(), Logs()} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return err
		}
	}
	return nil
}

// Migrate moves the plumbing an older Aft kept in Dir into Data, and reports
// whether it moved launchers, whose PATH line in the shell config must follow.
// A listening socket survives a rename, so live sessions keep working; their
// processes and older clients still use the old server, shims, sockets and
// sessions paths, so links are left there until no session is running, and a
// later Migrate removes them. The shims are rebuilt for the moved server.
// It is safe to call on every start. Remove it once no installation predates
// Data.
func Migrate() (launchers bool) {
	old := Dir()
	if old == Data() {
		return false
	}
	busy := false
	for _, n := range []string{"server", "launchers", "shims", "sessions", "sockets"} {
		src, dst := filepath.Join(old, n), filepath.Join(Data(), n)
		fi, err := os.Lstat(src)
		if err != nil || fi.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if !fi.IsDir() {
			// The server binary: keep it running sessions until a newer one is
			// installed over it.
			if _, err := os.Lstat(dst); err == nil || os.MkdirAll(Data(), 0o700) != nil || os.Rename(src, dst) != nil {
				os.Remove(src)
			}
			continue
		}
		entries, _ := os.ReadDir(src)
		if err := os.MkdirAll(dst, 0o700); err != nil {
			continue
		}
		for _, e := range entries {
			// Shims are recreated pointing at the new server.
			if n != "shims" && os.Rename(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())) == nil {
				launchers = launchers || n == "launchers"
				busy = busy || n == "sessions"
			}
		}
		os.RemoveAll(src)
	}
	if _, err := os.Stat(Server()); err == nil {
		InstallShims(Server())
	}
	links := []string{"server", "shims", "sessions", "sockets"}
	if busy {
		for _, n := range links {
			os.Symlink(filepath.Join(Data(), n), filepath.Join(old, n))
		}
	} else if entries, _ := os.ReadDir(Sessions()); len(entries) == 0 {
		for _, n := range links {
			if fi, err := os.Lstat(filepath.Join(old, n)); err == nil && fi.Mode()&os.ModeSymlink != 0 {
				os.Remove(filepath.Join(old, n))
			}
		}
	}
	return launchers
}
