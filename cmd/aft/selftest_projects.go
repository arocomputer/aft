package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fschrhunt/aft/internal/config"
)

// selftestProjects checks project activation, subfolder matching and deactivation.
// The caller supplies the temporary workspace and private AFT_HOME.
func selftestProjects() int {
	directory, err := os.Getwd()
	if err != nil {
		return fail(err)
	}
	if code := cmdOn([]string{"--host", "local", "--dir", directory}); code != 0 {
		return code
	}
	state, err := config.LoadState()
	if err != nil {
		return fail(err)
	}
	_, project, enabled := state.FindProject(filepath.Join(directory, "subfolder"))
	if !enabled || project.Host != "local" || project.Dir != config.RealPath(directory) {
		return fail(fmt.Errorf("project activation did not include subfolders"))
	}
	if code := cmdOff(nil); code != 0 {
		return code
	}
	state, err = config.LoadState()
	if err != nil {
		return fail(err)
	}
	if _, _, enabled := state.FindProject(directory); enabled {
		return fail(fmt.Errorf("project remains enabled"))
	}
	fmt.Println("Subfolder matching: ok")
	return 0
}
