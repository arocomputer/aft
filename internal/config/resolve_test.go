package config

import (
	"reflect"
	"testing"
)

func boolp(b bool) *bool { return &b }

// TestResolveLayers checks project overrides host overrides global, and that
// env accumulates across the layers.
func TestResolveLayers(t *testing.T) {
	yes := true
	c := Config{
		Predict: &yes,
		Wrapper: []string{"login"},
		Ports:   "3000-9999",
		Env:     []string{"GLOBAL"},
		Hosts: map[string]HostConfig{
			"box": {Wrapper: []string{"nix", "develop", "-c"}, Predict: boolp(false), Ports: "4000-4100", Env: []string{"HOSTV"}},
		},
	}
	// Host only.
	s := c.Resolve("box", nil)
	if !reflect.DeepEqual(s.Wrapper, []string{"nix", "develop", "-c"}) {
		t.Fatalf("host wrapper not applied: %v", s.Wrapper)
	}
	if s.Predict || s.Ports != "4000-4100" {
		t.Fatalf("host overrides not applied: %+v", s)
	}
	if !reflect.DeepEqual(s.Env, []string{"GLOBAL", "HOSTV"}) {
		t.Fatalf("env should accumulate global+host: %v", s.Env)
	}
	// Project overrides host.
	repo := &RepoConfig{Wrapper: []string{"docker", "exec", "-i", "dev"}, Ports: "5000-5001", Env: []string{"REPOV"}}
	s = c.Resolve("box", repo)
	if !reflect.DeepEqual(s.Wrapper, []string{"docker", "exec", "-i", "dev"}) || s.Ports != "5000-5001" {
		t.Fatalf("project overrides not applied: %+v", s)
	}
	if !reflect.DeepEqual(s.Env, []string{"GLOBAL", "HOSTV", "REPOV"}) {
		t.Fatalf("env should accumulate all layers: %v", s.Env)
	}
	// Unknown host falls back to global.
	if g := c.Resolve("", nil); !reflect.DeepEqual(g.Wrapper, []string{"login"}) || !g.Predict {
		t.Fatalf("global fallback wrong: %+v", g)
	}
}

// TestEditingDotfiles pins the default-on and the explicit opt-out.
func TestEditingDotfiles(t *testing.T) {
	if !(Config{}).EditingDotfiles() {
		t.Fatal("default should edit")
	}
	no := false
	if (Config{EditDotfiles: &no}).EditingDotfiles() {
		t.Fatal("edit_dotfiles=false should not edit")
	}
}
