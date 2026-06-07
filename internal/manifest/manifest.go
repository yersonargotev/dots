package manifest

import (
	"fmt"
	"os"

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
	Source   string   `yaml:"source"`
	Target   string   `yaml:"target"`
	Strategy string   `yaml:"strategy"`
	Tags     []string `yaml:"tags"`
	OS       []string `yaml:"os,omitempty"`
}

func LoadFile(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var manifest Manifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
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
		for j, osName := range entry.OS {
			if !allowedOS(osName) {
				return fmt.Errorf("entries[%d].os[%d] must be one of darwin, linux", i, j)
			}
		}
	}

	return nil
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
