package manifest

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Manifest struct {
	Version  int                `yaml:"version"`
	Profiles map[string]Profile `yaml:"profiles"`
	Entries  []Entry            `yaml:"entries"`
}

type Profile struct {
	Tags []string `yaml:"tags"`
}

type Entry struct {
	Source       string       `yaml:"source"`
	Target       string       `yaml:"target"`
	Strategy     string       `yaml:"strategy"`
	Tags         []string     `yaml:"tags"`
	OS           []string     `yaml:"os,omitempty"`
	Dependencies []Dependency `yaml:"dependencies,omitempty"`
}

// Dependency is an external tool a Managed Entry needs to work correctly. dots
// checks for it and offers OS-aware installation guidance but never installs it
// automatically in v1. The package fields (Brew/Apt/Dnf/Pacman) carry the
// per-platform package identifier used to build advisory install guidance.
type Dependency struct {
	Name    string `yaml:"name"`
	Command string `yaml:"command,omitempty"`
	Brew    string `yaml:"brew,omitempty"`
	Apt     string `yaml:"apt,omitempty"`
	Dnf     string `yaml:"dnf,omitempty"`
	Pacman  string `yaml:"pacman,omitempty"`
}

// Probe is the command name used to detect a Dependency's presence in PATH. It
// defaults to Name when an entry does not declare an explicit command.
func (d Dependency) Probe() string {
	if command := strings.TrimSpace(d.Command); command != "" {
		return command
	}
	return strings.TrimSpace(d.Name)
}

func LoadFile(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var manifest Manifest
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return nil, err
	}

	return &manifest, nil
}

func (m Manifest) Validate() error {
	if m.Version == 0 {
		return fmt.Errorf("version is required")
	}
	if m.Version != 1 {
		return fmt.Errorf("version must be 1")
	}
	if len(m.Profiles) == 0 {
		return fmt.Errorf("profiles is required")
	}
	if len(m.Entries) == 0 {
		return fmt.Errorf("entries is required")
	}

	names := make([]string, 0, len(m.Profiles))
	for name := range m.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		tags := m.Profiles[name].Tags
		if len(tags) == 0 {
			return fmt.Errorf("profiles[%q].tags is required", name)
		}
		if i, ok := indexOfEmptyTag(tags); ok {
			return fmt.Errorf("profiles[%q].tags[%d] must not be empty", name, i)
		}
	}

	for i, entry := range m.Entries {
		if entry.Source == "" {
			return fmt.Errorf("entries[%d].source is required", i)
		}
		if entry.Target == "" {
			return fmt.Errorf("entries[%d].target is required", i)
		}
		if !allowedStrategy(entry.Strategy) {
			return fmt.Errorf("entries[%d].strategy must be one of copy, symlink, template", i)
		}
		if len(entry.Tags) == 0 {
			return fmt.Errorf("entries[%d].tags is required", i)
		}
		if j, ok := indexOfEmptyTag(entry.Tags); ok {
			return fmt.Errorf("entries[%d].tags[%d] must not be empty", i, j)
		}
		for j, osName := range entry.OS {
			if !allowedOS(osName) {
				return fmt.Errorf("entries[%d].os[%d] must be one of darwin, linux", i, j)
			}
		}
		for j, dep := range entry.Dependencies {
			if strings.TrimSpace(dep.Name) == "" {
				return fmt.Errorf("entries[%d].dependencies[%d].name is required", i, j)
			}
			if dep.Command != "" && strings.TrimSpace(dep.Command) == "" {
				return fmt.Errorf("entries[%d].dependencies[%d].command must not be empty", i, j)
			}
		}
	}

	return nil
}

func indexOfEmptyTag(tags []string) (int, bool) {
	for i, tag := range tags {
		if strings.TrimSpace(tag) == "" {
			return i, true
		}
	}
	return -1, false
}

func allowedStrategy(strategy string) bool {
	switch strategy {
	case "copy", "symlink", "template":
		return true
	default:
		return false
	}
}

func allowedOS(osName string) bool {
	switch osName {
	case "darwin", "linux":
		return true
	default:
		return false
	}
}
