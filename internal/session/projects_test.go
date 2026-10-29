package session

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/fschrhunt/aft/internal/home"
)

// TestProjectSessionsFollowsShellDirectory checks actual cd transitions, symlink
// roots and path boundaries while retaining agents independently of the shell.
func TestProjectSessionsFollowsShellDirectory(t *testing.T) {
	t.Setenv("AFT_HOME", t.TempDir())
	base := t.TempDir()
	project := filepath.Join(base, "project")
	subdir := filepath.Join(project, "sub")
	sibling := filepath.Join(base, "project-other")
	for _, dir := range []string{subdir, sibling} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(project, alias); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", `while IFS= read -r dir; do cd "$dir" || exit; printf 'ready\n'; done`)
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { input.Close(); cmd.Process.Kill(); cmd.Wait() }()
	lines := bufio.NewScanner(output)
	records := []Record{
		{ID: "agent", Command: []string{"agent"}, Dir: sibling},
		{ID: "shell", Command: ShellCommand, Dir: project, PID: cmd.Process.Pid},
		{ID: "unknown", Command: ShellCommand, Dir: project},
		{ID: "ended", Command: ShellCommand, Dir: project, PID: cmd.Process.Pid, Ended: &Ending{}},
	}
	for _, step := range []struct {
		dir     string
		visible bool
	}{{base, false}, {subdir, true}, {sibling, false}, {project, true}} {
		fmt.Fprintln(input, step.dir)
		if !lines.Scan() || lines.Text() != "ready" {
			t.Fatalf("shell did not change directory: %v", lines.Err())
		}
		got := ProjectSessions(records, []string{alias})
		want := 1
		if step.visible {
			want++
		}
		if len(got) != want || got[0].ID != "agent" {
			t.Fatalf("in %s: got %+v, want %d visible records", step.dir, got, want)
		}
		if step.visible {
			real, _ := filepath.EvalSymlinks(step.dir)
			if got[1].ID != "shell" || got[1].Dir != real {
				t.Fatalf("wrong shell directory: %+v", got[1])
			}
		}
	}
	if got := ProjectSessions(records, nil); len(got) != 1 || got[0].ID != "agent" {
		t.Fatalf("shell exposed without enabled projects: %+v", got)
	}
	// Remote clients use the same filter through ls-projects, with repeatable
	// host-side roots. Internal ls still includes general connections.
	if err := home.Ensure(); err != nil {
		t.Fatal(err)
	}
	r := records[1]
	r.Server = os.Getpid()
	if err := writeJSON(RecordPath(r.ID), r); err != nil {
		t.Fatal(err)
	}
	for _, query := range []struct {
		args []string
		want int
	}{
		{[]string{"ls-projects", "--all"}, 0},
		{[]string{"ls-projects", "--all", "--project-dir", sibling, "--project-dir", alias}, 1},
		{[]string{"ls"}, 1},
	} {
		var out bytes.Buffer
		if code := Main(nil, query.args, nil, &out, io.Discard); code != 0 {
			t.Fatalf("%v returned %d", query.args, code)
		}
		var got []Record
		if err := json.Unmarshal(out.Bytes(), &got); err != nil || len(got) != query.want {
			t.Fatalf("%v: got %+v, %v", query.args, got, err)
		}
	}
}
