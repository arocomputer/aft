package home

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

// TestMigrateKeepsLiveSessions moves an older layout out of ~/.aft while a
// session is listening: the socket still answers at both paths, the server and
// its shims move, config stays, and the links in ~/.aft go once no session is
// left.
func TestMigrateKeepsLiveSessions(t *testing.T) {
	h, _ := os.MkdirTemp("", "aft") // short: unix socket paths are limited
	t.Cleanup(func() { os.RemoveAll(h) })
	t.Setenv("HOME", h)
	t.Setenv("AFT_HOME", "")
	old := filepath.Join(h, ".aft")
	for _, d := range []string{"launchers", "sessions", "sockets", "shims"} {
		os.MkdirAll(filepath.Join(old, d), 0o700)
	}
	os.WriteFile(filepath.Join(old, "config.toml"), []byte("use = \"box\"\n"), 0o600)
	os.WriteFile(filepath.Join(old, "server"), nil, 0o700)
	os.Symlink("/usr/bin/aft", filepath.Join(old, "launchers", "claude"))
	os.WriteFile(filepath.Join(old, "sessions", "s1.json"), []byte("{}"), 0o600)
	ln, err := net.Listen("unix", filepath.Join(old, "sockets", "s1.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	if !Migrate() {
		t.Fatal("launchers not reported as moved")
	}
	if _, err := os.Lstat(filepath.Join(Launchers(), "claude")); err != nil {
		t.Fatal("launcher not moved:", err)
	}
	if _, err := os.Stat(filepath.Join(old, "config.toml")); err != nil {
		t.Fatal("config moved:", err)
	}
	if dst, _ := os.Readlink(filepath.Join(Shims(), "pbpaste")); dst != Server() {
		t.Fatalf("shim points at %q, want the moved server", dst)
	}
	if _, err := os.Stat(filepath.Join(old, "server")); err != nil {
		t.Fatal("old server path no longer reaches the server:", err)
	}
	for _, p := range []string{filepath.Join(Sockets(), "s1.sock"), filepath.Join(old, "sockets", "s1.sock")} {
		c, err := net.Dial("unix", p)
		if err != nil {
			t.Fatalf("live session unreachable at %s: %v", p, err)
		}
		c.Close()
	}

	os.Remove(filepath.Join(Sessions(), "s1.json"))
	if Migrate() {
		t.Fatal("moved launchers twice")
	}
	for _, n := range []string{"server", "shims", "sessions", "sockets"} {
		if _, err := os.Lstat(filepath.Join(old, n)); err == nil {
			t.Fatalf("%s link left after the last session ended", n)
		}
	}
}

// TestMigrateRenamesServer gives a server binary installed under its old name
// its new one, aft-server.
func TestMigrateRenamesServer(t *testing.T) {
	h := t.TempDir()
	t.Setenv("HOME", h)
	t.Setenv("AFT_HOME", "")
	os.MkdirAll(Data(), 0o700)
	os.WriteFile(filepath.Join(Data(), "server"), []byte("bin"), 0o700)
	Migrate()
	if b, err := os.ReadFile(Server()); err != nil || string(b) != "bin" || filepath.Base(Server()) != "aft-server" {
		t.Fatalf("server not renamed: %q %v", b, err)
	}
	if _, err := os.Lstat(filepath.Join(Data(), "server")); err == nil {
		t.Fatal("the old name is still there")
	}
}
