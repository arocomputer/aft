package main

import (
	"os"
	"path/filepath"
	"slices"

	"github.com/arocomputer/aft/internal/launcher"
	"github.com/arocomputer/aft/internal/session"
	"github.com/arocomputer/aft/internal/ui"
)

// setupProgramOptions discovers executable candidates without running programs
// or reading their configuration. Existing selections override discovery defaults.
func setupProgramOptions(catalog, current, other []string, info session.Info, host string, fresh bool) []ui.Option {
	var options []ui.Option
	seen := map[string]bool{}
	for _, name := range append(slices.Clone(catalog), current...) {
		if seen[name] || slices.Contains(other, name) || launcher.ValidName(name) != nil {
			continue
		}
		seen[name] = true
		_, err := launcher.RealBinary(name)
		local := err == nil
		remote := info.Agents[name] != ""
		hint := "not detected"
		switch {
		case local && remote:
			hint = "found on this device and " + host
		case remote:
			hint = "found on " + host
		case local && host != "":
			hint = "found on this device; not detected on " + host
		case local:
			hint = "found on this device"
		case agentConfigFound(name):
			hint = "configuration found on this device; executable not found"
		}
		available := remote || (host == "" && local)
		options = append(options, ui.Option{Label: name, Hint: hint, Checked: slices.Contains(current, name) || (fresh && available)})
	}
	return options
}

// selectSetupPrograms uses explicit selections, or accepts discovery defaults
// for --yes. Cancellation leaves configuration and launchers unchanged.
func selectSetupPrograms(title string, options []ui.Option, yes bool) ([]string, bool) {
	indices := []int{}
	if yes {
		for i, option := range options {
			if option.Checked {
				indices = append(indices, i)
			}
		}
	} else {
		var ok bool
		indices, ok = ui.Checklist(title, options)
		if !ok {
			return nil, false
		}
	}
	selected := []string{}
	for _, index := range indices {
		selected = append(selected, options[index].Label)
	}
	return selected, true
}

// agentConfigFound recognizes configuration locations as hints only: a leftover
// directory never makes a missing executable selected or reads its credentials.
func agentConfigFound(name string) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	paths := map[string][]string{
		"claude": {".claude"}, "codex": {".codex"},
		"opencode": {".config/opencode"}, "pi": {".pi"},
		"gemini": {".gemini"}, "amp": {".config/amp"},
		"aider": {".aider.conf.yml"}, "goose": {".config/goose"},
	}
	for _, path := range paths[name] {
		if _, err := os.Stat(filepath.Join(home, path)); err == nil {
			return true
		}
	}
	return false
}
