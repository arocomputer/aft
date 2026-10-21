package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// fakeTool places a test-owned executable ahead of real tools in PATH.
func fakeTool(t *testing.T, name, body string) {
	t.Helper()
	dir := t.TempDir()
	if e := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0700); e != nil {
		t.Fatal(e)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// fakeRemote runs quoted remote shell commands only in disposable test directories.
func fakeRemote(t *testing.T) string {
	t.Helper()
	isolateProjects(t)
	remote := physicalTempDir(t)
	t.Setenv("AFT_TEST_REMOTE", remote)
	fakeTool(t, "ssh", `for arg do last=$arg; done
HOME=$AFT_TEST_REMOTE; export HOME
exec sh -c "$last"`)
	return remote
}

// TestWorkspaceProjects protects home expansion, containment, creation and record-only removal.
func TestWorkspaceProjects(t *testing.T) {
	remote := fakeRemote(t)
	root := filepath.Join(remote, "work's files")
	if e := os.Mkdir(root, 0700); e != nil {
		t.Fatal(e)
	}
	if e := runSetup([]string{"--name", "dev", "--host", "server", "--root", "~/work's files"}); e != nil {
		t.Fatal(e)
	}
	c, e := loadConfig()
	if e != nil || c.Workspaces["dev"].Root != root || c.Default != "dev" {
		t.Fatalf("%+v %v", c, e)
	}
	if e = runWorkspace([]string{"add", "dev", "other", root}); e == nil {
		t.Fatal("replaced workspace")
	}
	if e = registerProject("new", []string{"myapp"}); e != nil {
		t.Fatal(e)
	}
	if e = registerProject("new", []string{"--name", "another", "myapp"}); e == nil {
		t.Fatal("accepted existing directory")
	}
	p, e := loadProject("myapp")
	if e != nil || p.Workspace != "dev" {
		t.Fatalf("%+v %v", p, e)
	}
	if e = runWorkspace([]string{"remove", "dev"}); e == nil {
		t.Fatal("removed referenced workspace")
	}
	if e = runProjects([]string{"remove", "myapp"}); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(p.Dir); e != nil {
		t.Fatal("removed project directory", e)
	}
	outside := filepath.Join(remote, "outside")
	if e = os.Mkdir(outside, 0700); e != nil {
		t.Fatal(e)
	}
	if e = os.Symlink(outside, filepath.Join(root, "escape")); e != nil {
		t.Fatal(e)
	}
	for _, req := range []struct{ action, path string }{{"add", "../outside"}, {"add", "escape"}, {"new", "escape/child"}, {"add", "missing"}} {
		if e = registerProject(req.action, []string{req.path}); e == nil {
			t.Fatalf("accepted %+v", req)
		}
	}
	if _, e = os.Stat(filepath.Join(outside, "child")); !os.IsNotExist(e) {
		t.Fatal("created outside workspace")
	}
	if e = registerProject("add", []string{"myapp"}); e != nil {
		t.Fatal(e)
	}
	ssh, e := executionArgs([]string{"exec", "myapp", "--", "pwd"})
	if e != nil {
		t.Fatal(e)
	}
	data, e := exec.Command("sh", "-c", ssh[len(ssh)-1]).Output()
	if e != nil || strings.TrimSpace(string(data)) != p.Dir {
		t.Fatalf("%s %v", data, e)
	}
	if e = os.Rename(p.Dir, p.Dir+"-old"); e != nil {
		t.Fatal(e)
	}
	if e = os.Symlink(outside, p.Dir); e != nil {
		t.Fatal(e)
	}
	if e = exec.Command("sh", "-c", ssh[len(ssh)-1]).Run(); e == nil {
		t.Fatal("executed through replaced symlink")
	}
}

// TestProjectExecution protects explicit host selection, literal arguments and remote exit codes.
func TestProjectExecution(t *testing.T) {
	remote := fakeRemote(t)
	if e := saveProject(project{Name: "app", Host: "server", Dir: remote}); e != nil {
		t.Fatal(e)
	}
	for _, args := range [][]string{{"exec", "missing", "--", "true"}, {"exec", "server", "true"}, {"exec", "--host", "server", "app", "--", "true"}, {"exec", "--dir", "/tmp", "app", "--", "true"}} {
		if _, e := executionArgs(args); e == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	if got := run([]string{"exec", "app", "--", "sh", "-c", "exit 37"}); got != 37 {
		t.Fatalf("exit=%d", got)
	}
	if got := run([]string{"exec", "--host", "server", "--", "sh", "-c", "exit 23"}); got != 23 {
		t.Fatalf("exit=%d", got)
	}
	ssh, e := executionArgs([]string{"run", "app", "--", "sleep", "60"})
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(ssh[len(ssh)-1], "new-session -A") || !strings.Contains(ssh[len(ssh)-1], "aft-app") {
		t.Fatal(ssh)
	}
	for _, args := range [][]string{{"ports", "--project", "app", "--local", "8080", "3000"}, {"ports", "watch", "--min", "3000", "--project=app"}, {"ports", "list", "--project", "app"}} {
		got, e := resolvePortProject(args)
		if e != nil {
			t.Fatal(e)
		}
		found := false
		for _, v := range got {
			if v == "server" {
				found = true
			}
		}
		if !found {
			t.Fatal(got)
		}
	}
	got, e := resolvePortProject([]string{"ports", "--project", "app", "3000"})
	if e != nil || !reflect.DeepEqual(got, []string{"ports", "server", "3000"}) {
		t.Fatalf("%v %v", got, e)
	}
}

// TestPrivateConfigAndSymlinks ensures atomic private writes and refuses redirected metadata.
func TestPrivateConfigAndSymlinks(t *testing.T) {
	isolateProjects(t)
	change := func(c *configuration) error {
		c.Workspaces["dev"] = workspace{"server", "/work"}
		c.Default = "dev"
		return nil
	}
	if e := editConfig(change); e != nil {
		t.Fatal(e)
	}
	root, _ := aftRoot()
	file := filepath.Join(root, "config.json")
	info, e := os.Stat(file)
	if e != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("%v %v", info, e)
	}
	before, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	if e = runWorkspace([]string{"default", "missing"}); e == nil {
		t.Fatal("accepted missing default")
	}
	after, _ := os.ReadFile(file)
	if string(before) != string(after) {
		t.Fatal("failed edit changed config")
	}
	target := filepath.Join(t.TempDir(), "target")
	if e = os.WriteFile(target, before, 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.Remove(file); e != nil {
		t.Fatal(e)
	}
	if e = os.Symlink(target, file); e != nil {
		t.Fatal(e)
	}
	if e = editConfig(change); e == nil {
		t.Fatal("accepted symlink config")
	}
	projects, _ := projectRoot()
	if e = os.Symlink(t.TempDir(), projects); e != nil {
		t.Fatal(e)
	}
	if e = saveProject(project{Name: "app", Host: "server", Dir: "/work/app"}); e == nil {
		t.Fatal("followed metadata directory symlink")
	}
}

// TestMigrationPreflight refuses conflicts before copying and leaves all originals intact.
func TestMigrationPreflight(t *testing.T) {
	isolateProjects(t)
	source := physicalTempDir(t)
	records := []project{{Name: "a", Host: "host", Dir: "/a"}, {Name: strings.Repeat("b", 60), Host: "host", Dir: "/b"}}
	for _, p := range records {
		b, _ := json.Marshal(p)
		if e := os.WriteFile(filepath.Join(source, p.Name+".json"), b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	if e := saveProject(records[1]); e != nil {
		t.Fatal(e)
	}
	if e := migrateProjects([]string{"--from", source}); e == nil {
		t.Fatal("accepted conflict")
	}
	if _, e := loadProject("a"); !os.IsNotExist(e) {
		t.Fatal("partial copy on preflight conflict")
	}
	if e := runProjects([]string{"remove", records[1].Name}); e != nil {
		t.Fatal(e)
	}
	if e := migrateProjects([]string{"--from", source}); e != nil {
		t.Fatal(e)
	}
	for _, p := range records {
		if _, e := loadProject(p.Name); e != nil {
			t.Fatal(e)
		}
		if _, e := os.Stat(filepath.Join(source, p.Name+".json")); e != nil {
			t.Fatal(e)
		}
	}
}

// TestFailedServiceRemoval distinguishes stopped units from unavailable managers.
func TestFailedServiceRemoval(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("systemd removal branch")
	}
	isolateProjects(t)
	dir, e := serviceDirectory()
	if e != nil {
		t.Fatal(e)
	}
	if e = os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	file := filepath.Join(dir, serviceID("server")+".service")
	if e = os.WriteFile(file, []byte("# "+serviceMarker), 0600); e != nil {
		t.Fatal(e)
	}
	fakeTool(t, "systemctl", `case "$*" in
 *'disable --now'*) exit 1;;
 *'show '*) printf '%s\n' "$AFT_TEST_STATE"; exit 0;;
 *) exit 0;;
esac`)
	t.Setenv("AFT_TEST_STATE", "active")
	if e = runService([]string{"remove", "server"}); e == nil {
		t.Fatal("removed active unit")
	}
	if _, e = os.Stat(file); e != nil {
		t.Fatal(e)
	}
	t.Setenv("AFT_TEST_STATE", "failed")
	if e = runService([]string{"remove", "server"}); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(file); !os.IsNotExist(e) {
		t.Fatal("unit remains")
	}
}

// TestLaunchdMissingService accepts only an explicit not-loaded diagnosis.
func TestLaunchdMissingService(t *testing.T) {
	fakeTool(t, "launchctl", `printf '%s\n' "$AFT_TEST_LAUNCH_ERROR" >&2; exit 113`)
	t.Setenv("AFT_TEST_LAUNCH_ERROR", "Could not find service dev.aft.ports.test")
	if e := serviceStopped("darwin", "gui/1/test"); e != nil {
		t.Fatal(e)
	}
	t.Setenv("AFT_TEST_LAUNCH_ERROR", "Could not connect to service manager")
	if e := serviceStopped("darwin", "gui/1/test"); e == nil {
		t.Fatal("accepted unavailable manager")
	}
}

// TestCompletionAndUninstallNeverConnect ensures local UX cannot invoke SSH or remove data.
func TestCompletionAndUninstallNeverConnect(t *testing.T) {
	isolateProjects(t)
	marker := filepath.Join(t.TempDir(), "called")
	t.Setenv("AFT_TEST_MARKER", marker)
	fakeTool(t, "ssh", `touch "$AFT_TEST_MARKER"; exit 99`)
	if e := saveProject(project{Name: "app", Host: "server", Dir: "/work/app"}); e != nil {
		t.Fatal(e)
	}
	for _, args := range [][]string{{"_complete", "projects"}, {"_complete", "workspaces"}, {"completion", "bash"}, {"completion", "zsh"}, {"uninstall", "--dry-run"}} {
		if n := run(args); n != 0 {
			t.Fatalf("%v exit %d", args, n)
		}
	}
	if _, e := os.Stat(marker); !os.IsNotExist(e) {
		t.Fatal("SSH called")
	}
	if e := runUninstall([]string{"--yes"}); e == nil {
		t.Fatal("accepted actual removal")
	}
	if _, e := loadProject("app"); e != nil {
		t.Fatal(e)
	}
}

// TestDefaultDiagnostics selects the configured host and keeps probes noninteractive.
func TestDefaultDiagnostics(t *testing.T) {
	isolateProjects(t)
	if e := editConfig(func(c *configuration) error {
		c.Workspaces["dev"] = workspace{"expected-host", "/work"}
		c.Default = "dev"
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	fakeTool(t, "ssh", `case "$*" in *BatchMode=yes*StrictHostKeyChecking=yes*expected-host*) printf 'connected\n';; *) exit 91;; esac`)
	fakeTool(t, "ss", `exit 0`)
	fakeTool(t, "lsof", `exit 0`)
	if e := runDoctor(nil); e != nil {
		t.Fatal(e)
	}
	if e := runStatus(nil); e != nil {
		t.Fatal(e)
	}
}
