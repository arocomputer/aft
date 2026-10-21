package main

import (
	"io"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

// TestRemoteArguments verifies shell metacharacters survive as data, not code.
func TestRemoteArguments(t *testing.T) {
	dir := "project's files; echo INJECTED; $(false)"
	args, err := sshArgs([]string{"connect", "--session", "api", "--dir", dir, "server"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	// A shell function captures tmux arguments after both remote quoting layers.
	script := "tmux() { printf '%s\\n' \"$@\"; }; "
	remote := args[len(args)-1]
	decoded, err := exec.Command("sh", "-c", "printf '%s' "+strings.TrimPrefix(remote, "sh -c ")).Output()
	if err != nil {
		t.Fatal(err)
	}
	script += strings.Replace(string(decoded), "exec tmux", "tmux", 1)
	got, err := exec.Command("sh", "-c", script).CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v", got, err)
	}
	want := "new-session\n-A\n-s\napi\n-c\n./" + dir + "\n"
	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestForward binds only loopback and never allows an occupied port to be hidden.
func TestForward(t *testing.T) {
	args, err := sshArgs([]string{"ports", "--local", "8080", "server", "3000"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=3", "-S", "none", "-N", "-T", "-o", "ExitOnForwardFailure=yes", "-L", "127.0.0.1:8080:localhost:3000", "--", "server"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("got %q, want %q", args, want)
	}
}

// TestRejectsUnsafeRequests ensures invalid input never reaches SSH.
func TestRejectsUnsafeRequests(t *testing.T) {
	for _, args := range [][]string{
		{"connect", "--", "-oProxyCommand=bad"},
		{"connect", "server\nother"},
		{"connect", "--session", "../bad", "server"},
		{"connect", "--dir", "bad\x00path", "server"},
		{"ports", "server", "65536"},
		{"ports", "--local", "22", "server", "3000"},
		{"connect", "server", "--session", "api"},
	} {
		if _, err := sshArgs(args, io.Discard); err == nil {
			t.Errorf("accepted %q", args)
		}
	}
}

// TestExecQuoting checks a remote command's argument vector survives shell parsing.
func TestExecQuoting(t *testing.T) {
	value := "a'; $(echo bad); * with spaces"
	args, err := sshArgs([]string{"exec", "server", "printf", "%s", value}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	got, err := exec.Command("sh", "-c", args[len(args)-1]).Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != value {
		t.Fatalf("got %q", got)
	}
}

// TestDetachedRunRequiresSession prevents accidental takeover of an existing shell.
func TestDetachedRunRequiresSession(t *testing.T) {
	if _, err := sshArgs([]string{"run", "server", "sleep", "60"}, io.Discard); err == nil {
		t.Fatal("accepted unnamed job")
	}
	args, err := sshArgs([]string{"run", "--session", "build", "server", "sleep", "60"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(args[len(args)-1], "new-session -d") {
		t.Fatal(args)
	}
}
