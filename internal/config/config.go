// Package config reads and writes Aft's settings and state on the user's
// machine: the TOML config, the record of which projects are on, and the
// optional aft.toml a repository may commit.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/arocomputer/aft/internal/home"
	"github.com/arocomputer/aft/internal/transport"
)

// DefaultAgents are the launchers setup creates when they are installed.
var DefaultAgents = []string{"claude", "codex", "opencode", "pi", "gemini", "amp", "aider", "goose"}

// Config is ~/.aft/config.toml. Global keys set defaults; a host or a
// repository's aft.toml can override most of them.
type Config struct {
	DefaultHost     string                `toml:"default_host"`
	Agents          []string              `toml:"agents"`
	Predict         *bool                 `toml:"predict,omitempty"`
	DetachKey       string                `toml:"detach_key"`
	Prefix          *string               `toml:"prefix,omitempty"` // key for copy mode and detach; nil means the default ctrl-]
	PathIntegration string                `toml:"path_integration"` // "edit" (default) or "manual"
	Wrapper         []string              `toml:"wrapper"`          // default command wrapper on the server
	Hosts           map[string]HostConfig `toml:"hosts"`
	Env             EnvConfig             `toml:"env"`
	Ports           PortsConfig           `toml:"ports"`
	Mounts          []Mount               `toml:"mounts"`
}

// HostConfig describes one server and overrides global settings for it.
type HostConfig struct {
	SSH     string   `toml:"ssh"`               // destination, may carry ssh options
	Home    string   `toml:"home,omitempty"`    // server home on the server, default ~/.aft
	Wrapper []string `toml:"wrapper,omitempty"` // command wrapper on the server
	Predict *bool    `toml:"predict,omitempty"` // prediction for sessions on this host
	Ports   string   `toml:"ports,omitempty"`   // mirror range for this host
	Env     []string `toml:"env,omitempty"`     // extra variables to pass to this host
	Alive   int      `toml:"alive,omitempty"`   // ServerAlive interval seconds, 0 = default
	Persist int      `toml:"persist,omitempty"` // ControlPersist seconds, 0 = default
}

// Mount maps a locally mounted tree to the server directory it comes from, for
// network mounts such as SMB or NFS where the files are the server's and the
// Mac only views them. A project under Local needs no clone and no sync; aft
// just runs commands against the real path on the server.
type Mount struct {
	Local  string `toml:"local"`  // the local mountpoint, e.g. /Volumes/Room
	Host   string `toml:"host"`   // the aft host the mount comes from
	Remote string `toml:"remote"` // the server path the mount exposes, e.g. /home/fschr/room
}

// EnvConfig lists extra variables that may cross to the server.
type EnvConfig struct {
	Pass []string `toml:"pass"`
}

// PortsConfig sets the mirrored range.
type PortsConfig struct {
	Mirror string `toml:"mirror"`
}

// Project is one directory that is on.
type Project struct {
	Host string `json:"host"`
	Dir  string `json:"dir"`
}

// ShellBlock records a block Aft added to a shell config file.
type ShellBlock struct {
	File  string `json:"file"`
	Block string `json:"block"`
}

// State is ~/.aft/state.json: everything Aft changed that uninstall must undo.
type State struct {
	Projects    map[string]Project `json:"projects"`
	ShellBlocks []ShellBlock       `json:"shell_blocks"`
	Shims       []string           `json:"shims"`
	Binary      string             `json:"binary"`
}

// RepoConfig is an optional aft.toml at a repository root.
type RepoConfig struct {
	Host    string      `toml:"host"`
	Dir     string      `toml:"dir"`
	Ports   PortsConfig `toml:"ports"`
	Wrapper []string    `toml:"wrapper"` // command wrapper for this project
	Env     []string    `toml:"env"`     // extra variables for this project
}

// Settings are the effective per-session settings after merging global, host
// and project configuration. Project overrides host overrides global.
type Settings struct {
	Wrapper []string
	Predict bool
	Ports   string
	Env     []string
	Alive   int
	Persist int
}

// Resolve merges settings for a host and an optional repository config.
func (c Config) Resolve(hostName string, repo *RepoConfig) Settings {
	h := c.Hosts[hostName]
	s := Settings{
		Wrapper: c.Wrapper,
		Predict: c.PredictOn(),
		Ports:   c.Ports.Mirror,
		Env:     append([]string(nil), c.Env.Pass...),
		Alive:   h.Alive,
		Persist: h.Persist,
	}
	if len(h.Wrapper) > 0 {
		s.Wrapper = h.Wrapper
	}
	if h.Predict != nil {
		s.Predict = *h.Predict
	}
	if h.Ports != "" {
		s.Ports = h.Ports
	}
	s.Env = append(s.Env, h.Env...)
	if repo != nil {
		if len(repo.Wrapper) > 0 {
			s.Wrapper = repo.Wrapper
		}
		if repo.Ports.Mirror != "" {
			s.Ports = repo.Ports.Mirror
		}
		s.Env = append(s.Env, repo.Env...)
	}
	return s
}

// EditDotfiles reports whether setup should edit shell config files itself.
// Manual mode prints the line instead, for people who manage dotfiles.
func (c Config) EditDotfiles() bool { return c.PathIntegration != "manual" }

// Path is the config file.
func Path() string { return filepath.Join(home.Dir(), "config.toml") }

// StatePath is the state file.
func StatePath() string { return filepath.Join(home.Dir(), "state.json") }

// Load reads the config, returning defaults when none exists.
func Load() (Config, error) {
	c := Config{Hosts: map[string]HostConfig{}}
	b, err := os.ReadFile(Path())
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := toml.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("%s: %w", Path(), err)
	}
	if c.Hosts == nil {
		c.Hosts = map[string]HostConfig{}
	}
	return c, nil
}

// Save writes the config.
func Save(c Config) error {
	if err := home.Ensure(); err != nil {
		return err
	}
	b, err := toml.Marshal(c)
	if err != nil {
		return err
	}
	return writeFile(Path(), b)
}

// PredictOn reports the prediction setting, on by default.
func (c Config) PredictOn() bool { return c.Predict == nil || *c.Predict }

// Host resolves a host name. "local" is always this machine.
func (c Config) Host(name string) (transport.Host, error) {
	if name == "" {
		name = c.DefaultHost
	}
	if name == "local" {
		return transport.Local, nil
	}
	if name == "" {
		return transport.Host{}, errors.New("no host configured; run aft setup")
	}
	h, ok := c.Hosts[name]
	if !ok {
		return transport.Host{}, fmt.Errorf("unknown host %q; run aft setup", name)
	}
	if h.SSH == "" {
		return transport.Host{}, fmt.Errorf("host %q has no ssh destination", name)
	}
	return transport.Host{Name: name, SSH: h.SSH, Alive: h.Alive, Persist: h.Persist}, nil
}

// HostNames lists configured hosts in order.
func (c Config) HostNames() []string {
	names := make([]string, 0, len(c.Hosts))
	for n := range c.Hosts {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// LoadState reads the state file.
func LoadState() (State, error) {
	s := State{Projects: map[string]Project{}}
	b, err := os.ReadFile(StatePath())
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return s, fmt.Errorf("%s: %w", StatePath(), err)
	}
	if s.Projects == nil {
		s.Projects = map[string]Project{}
	}
	return s, nil
}

// SaveState writes the state file.
func SaveState(s State) error {
	if err := home.Ensure(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(StatePath(), b)
}

func writeFile(path string, b []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// FindProject returns the on project containing dir, if any, with the local
// root it was turned on at.
func (s State) FindProject(dir string) (root string, p Project, ok bool) {
	dir = filepath.Clean(dir)
	best := ""
	for r := range s.Projects {
		if dir == r || strings.HasPrefix(dir, r+string(filepath.Separator)) {
			if len(r) > len(best) {
				best = r
			}
		}
	}
	if best == "" {
		return "", Project{}, false
	}
	return best, s.Projects[best], true
}

// RemoteDir maps a local directory inside a project to its server directory.
func RemoteDir(root string, p Project, dir string) string {
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == "." {
		return p.Dir
	}
	return p.Dir + "/" + filepath.ToSlash(rel)
}

// LoadRepo reads aft.toml from dir or its parents, returning the root it was
// found in.
func LoadRepo(dir string) (RepoConfig, string, bool) {
	for d := filepath.Clean(dir); ; d = filepath.Dir(d) {
		b, err := os.ReadFile(filepath.Join(d, "aft.toml"))
		if err == nil {
			var r RepoConfig
			if toml.Unmarshal(b, &r) == nil {
				return r, d, true
			}
			return RepoConfig{}, d, false
		}
		if filepath.Dir(d) == d {
			return RepoConfig{}, "", false
		}
	}
}

// DefaultRemoteDir maps a local directory to the same path relative to home.
func DefaultRemoteDir(local string) string {
	h, err := os.UserHomeDir()
	if err == nil {
		if rel, err := filepath.Rel(h, local); err == nil && !strings.HasPrefix(rel, "..") {
			if rel == "." {
				return "~"
			}
			return "~/" + filepath.ToSlash(rel)
		}
	}
	return local
}

// PrefixKey is the control byte that opens aft's own commands (copy mode,
// detach). It defaults to Ctrl-] when unset; "", "off" or "none" disable it.
func (c Config) PrefixKey() (byte, error) {
	s := "ctrl-]"
	if c.Prefix != nil {
		s = *c.Prefix
	}
	return DetachByte(s)
}

// DetachByte turns a detach_key setting such as "ctrl-]" into a byte.
func DetachByte(s string) (byte, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || s == "off" || s == "none" {
		return 0, nil
	}
	s = strings.TrimPrefix(strings.TrimPrefix(s, "ctrl-"), "^")
	if len(s) != 1 {
		return 0, fmt.Errorf("detach_key must look like \"ctrl-]\"")
	}
	ch := s[0]
	switch {
	case ch >= 'a' && ch <= 'z':
		return ch - 'a' + 1, nil
	case ch >= '[' && ch <= '_':
		return ch - '[' + 27, nil
	}
	return 0, fmt.Errorf("detach_key must be a control character such as \"ctrl-]\"")
}

// MapLocal returns the host and server directory for a local path that lives
// under a configured mount, choosing the most specific mount. The local path
// should be resolved of symlinks by the caller, since a mountpoint is physical.
func (c Config) MapLocal(local string) (host, remote string, ok bool) {
	local = filepath.Clean(local)
	best := -1
	var chosen Mount
	for _, m := range c.Mounts {
		ml := filepath.Clean(m.Local)
		if local == ml || strings.HasPrefix(local, ml+string(filepath.Separator)) {
			if len(ml) > best {
				best, chosen = len(ml), m
			}
		}
	}
	if best < 0 {
		return "", "", false
	}
	suffix := filepath.ToSlash(strings.TrimPrefix(local, filepath.Clean(chosen.Local)))
	remote = strings.TrimRight(chosen.Remote, "/") + suffix
	return chosen.Host, remote, true
}
