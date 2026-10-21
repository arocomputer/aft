package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// project records a remote workspace without owning or deleting its files.
type project struct {
	Name      string `json:"name"`
	Host      string `json:"host"`
	Dir       string `json:"directory"`
	Workspace string `json:"workspace,omitempty"`
}

// projectRoot locates records under the single Aft metadata root.
func projectRoot() (string, error) {
	dir, err := aftRoot()
	return filepath.Join(dir, "projects"), err
}

// projectPath accepts a single safe record name, never arbitrary filesystem paths.
func projectPath(name string) (string, error) {
	if !sessionName.MatchString(name) {
		return "", fmt.Errorf("invalid project name %q", name)
	}
	dir, err := projectRoot()
	return filepath.Join(dir, name+".json"), err
}

// validateProject checks saved records too; edited JSON is not trusted command input.
func validateProject(p project) error {
	if !sessionName.MatchString(p.Name) || len(p.Name) > 60 || !validHost(p.Host) {
		return fmt.Errorf("invalid project name or SSH host")
	}
	if p.Workspace != "" && !sessionName.MatchString(p.Workspace) {
		return fmt.Errorf("invalid workspace name")
	}
	if !path.IsAbs(p.Dir) || path.Clean(p.Dir) == "/" || strings.ContainsAny(p.Dir, "\x00\r\n") {
		return fmt.Errorf("project directory must be an absolute remote path other than /")
	}
	return nil
}

// saveProject atomically publishes a private record, refusing concurrent replacement.
func saveProject(p project) error {
	if err := validateProject(p); err != nil {
		return err
	}
	name, err := projectPath(p.Name)
	if err != nil {
		return err
	}
	if err := privateDir(filepath.Dir(name)); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(name), ".project-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := json.NewEncoder(tmp).Encode(p); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// Linking a complete file is atomic and fails if the record already exists.
	if err := os.Link(tmp.Name(), name); err != nil {
		return fmt.Errorf("add project (existing names are never overwritten): %w", err)
	}
	return nil
}

// loadProject rejects symlink records and mismatches between names and file contents.
func loadProject(name string) (project, error) {
	var p project
	file, err := projectPath(name)
	if err != nil {
		return p, err
	}
	if err := safeParents(filepath.Dir(file)); err != nil {
		return p, err
	}
	info, err := os.Lstat(file)
	if err != nil {
		return p, err
	}
	if !info.Mode().IsRegular() || info.Size() > 65536 {
		return p, fmt.Errorf("invalid project record")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return p, err
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return p, err
	}
	if p.Name != name {
		return p, fmt.Errorf("project record name mismatch")
	}
	return p, validateProject(p)
}

// runProjects manages metadata only; add never creates or uploads remote files.
func runProjects(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: aft project add NAME HOST /REMOTE/PATH | list | remove NAME | migrate [--from DIRECTORY]")
	}
	switch args[0] {
	case "migrate":
		return migrateProjects(args[1:])
	case "add":
		if len(args) != 4 {
			return fmt.Errorf("usage: aft project add NAME HOST /REMOTE/PATH")
		}
		p := project{Name: args[1], Host: args[2], Dir: args[3]}
		if err := saveProject(p); err != nil {
			return err
		}
		fmt.Printf("Saved %s → %s:%s (path checked when opened)\n", p.Name, p.Host, p.Dir)
	case "list":
		if len(args) != 1 {
			return fmt.Errorf("project list takes no arguments")
		}
		root, err := projectRoot()
		if err != nil {
			return err
		}
		if err := safeParents(root); err != nil {
			return err
		}
		entries, err := os.ReadDir(root)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		var names []string
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".json") {
				names = append(names, strings.TrimSuffix(entry.Name(), ".json"))
			}
		}
		sort.Strings(names)
		for _, name := range names {
			p, err := loadProject(name)
			if err != nil {
				return err
			}
			fmt.Printf("%s\t%s\t%s\n", p.Name, p.Host, p.Dir)
		}
	case "remove":
		if len(args) != 2 {
			return fmt.Errorf("usage: aft project remove NAME")
		}
		file, err := projectPath(args[1])
		if err != nil {
			return err
		}
		if err := safeParents(filepath.Dir(file)); err != nil {
			return err
		}
		if err := os.Remove(file); err != nil {
			return err
		}
		fmt.Println("Removed project record; remote files and sessions were not touched.")
	default:
		return fmt.Errorf("unknown project command %q", args[0])
	}
	return nil
}
