package config

import (
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

// TestSchemaKeys pins the public config.toml key names: a host is a singular
// [host.<name>] table, and env/ports/detach/edit_dotfiles are flat top-level
// keys. These names are the user-facing contract, so a tag typo must fail here.
func TestSchemaKeys(t *testing.T) {
	src := `
use = "mini"
agents = ["claude"]
detach = "ctrl-]"
edit_dotfiles = false
env = ["FOO"]
ports = "3000-3999"

[host.mini]
ssh = "mini"
ports = "4000-4100"

[[mount]]
local = "/Volumes/Room"
host = "mini"
path = "/home/fschr/room"
`
	var c Config
	if err := toml.Unmarshal([]byte(src), &c); err != nil {
		t.Fatal(err)
	}
	if c.Use != "mini" || c.DetachKey != "ctrl-]" {
		t.Fatalf("top-level keys wrong: %+v", c)
	}
	if c.EditDotfiles == nil || *c.EditDotfiles {
		t.Fatal("edit_dotfiles=false did not parse")
	}
	if len(c.Env) != 1 || c.Env[0] != "FOO" || c.Ports != "3000-3999" {
		t.Fatalf("flat env/ports wrong: %+v", c)
	}
	h, ok := c.Hosts["mini"]
	if !ok || h.SSH != "mini" || h.Ports != "4000-4100" {
		t.Fatalf("[host.mini] wrong: %+v", c.Hosts)
	}
	if len(c.Mounts) != 1 || c.Mounts[0].Path != "/home/fschr/room" || c.Mounts[0].Host != "mini" {
		t.Fatalf("[[mount]] wrong: %+v", c.Mounts)
	}

	// Marshaling uses the same singular table names.
	out, err := toml.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"[host.mini]", "[[mount]]"} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("marshaled config missing %q:\n%s", want, out)
		}
	}
}
