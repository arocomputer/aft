package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/arocomputer/aft/internal/home"
)

// selftestSetup runs the real wizard with private HOME and AFT_HOME directories.
// Its SSH stub rejects remote operations; the installed client is not replaced.
func selftestSetup(args []string) int {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		return cmdSetup(args)
	}
	directory, err := os.MkdirTemp("/tmp", "aft-setup-")
	if err != nil {
		return fail(err)
	}
	defer os.RemoveAll(directory)
	bin := filepath.Join(directory, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		return fail(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte("#!/bin/sh\nprintf '%s\\n' 'SSH disabled in setup preview' >&2\nexit 1\n"), 0o700); err != nil {
		return fail(err)
	}
	path := selftestPath(os.Getenv("PATH"), home.Launchers())
	if userHome, err := os.UserHomeDir(); err == nil {
		path = selftestPath(path, filepath.Join(userHome, ".local/share/aft/launchers"))
	}
	environment := selftestChildEnv(map[string]string{
		"HOME": directory, "AFT_HOME": filepath.Join(directory, "state"), "AFT_REMOTE_HOME": "", "AFT_SESSION": "", "BASH_ENV": "", "ENV": "",
		"XDG_CONFIG_HOME": filepath.Join(directory, ".config"), "XDG_DATA_HOME": filepath.Join(directory, ".local/share"),
		"PATH": bin + string(os.PathListSeparator) + path,
	})
	self, err := os.Executable()
	if err != nil {
		return fail(err)
	}
	fmt.Printf("Preview home: %s\n", directory)
	command := exec.Command(self, append([]string{"setup"}, args...)...)
	command.Env = environment
	command.Dir = directory
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return exit.ExitCode()
		}
		return fail(err)
	}
	if data, err := os.ReadFile(filepath.Join(directory, "state/config.toml")); err == nil {
		fmt.Printf("\nconfig.toml\n%s", data)
	}
	return 0
}

// selftestChildEnv replaces named environment entries without duplicate keys.
func selftestChildEnv(values map[string]string) []string {
	environment := []string{}
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if _, replace := values[name]; !replace {
			environment = append(environment, entry)
		}
	}
	for name, value := range values {
		environment = append(environment, name+"="+value)
	}
	return environment
}
