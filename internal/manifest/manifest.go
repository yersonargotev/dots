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
	Version      int                `yaml:"version"`
	Profiles     map[string]Profile `yaml:"profiles"`
	Entries      []Entry            `yaml:"entries"`
	Provisioners []Provisioner      `yaml:"provisioners,omitempty"`
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

// Provisioner declares an allowlisted external agent-configuration tool that
// dots drives declaratively after dependency installs and file entries. dots
// versions the invocation (tool + flag spec), never the rendered content the
// tool regenerates. Tags and OS reuse the same Profile scoping as Entries.
type Provisioner struct {
	Tool         string          `yaml:"tool"`
	Tags         []string        `yaml:"tags"`
	OS           []string        `yaml:"os,omitempty"`
	Spec         ProvisionerSpec `yaml:"spec"`
	Dependencies []Dependency    `yaml:"dependencies,omitempty"`
}

// ProvisionerSpec maps 1:1 to the allowlisted tool's install flags. dots owns
// these declarative values; the tool owns how they render into agent config.
type ProvisionerSpec struct {
	Scope      string   `yaml:"scope,omitempty"`
	Channel    string   `yaml:"channel,omitempty"`
	Persona    string   `yaml:"persona,omitempty"`
	SDDMode    string   `yaml:"sdd-mode,omitempty"`
	Agents     []string `yaml:"agents,omitempty"`
	Components []string `yaml:"components,omitempty"`
	Skills     []string `yaml:"skills,omitempty"`
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
	// FontMatch, when set, switches detection from a PATH lookup to a scan of
	// the workstation font directories for an installed file whose name matches
	// this case-insensitive glob (e.g. "CascadiaCodeNF*"). A font has no
	// executable on the path, so it must be probed as an installed asset.
	FontMatch string `yaml:"font_match,omitempty"`
}

// IsFont reports whether the Dependency is detected as an installed font asset
// rather than an executable on PATH. It is true exactly when FontMatch is set.
func (d Dependency) IsFont() bool {
	return strings.TrimSpace(d.FontMatch) != ""
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

	for i, prov := range m.Provisioners {
		if strings.TrimSpace(prov.Tool) == "" {
			return fmt.Errorf("provisioners[%d].tool is required", i)
		}
		if !allowedProvisionerTool(prov.Tool) {
			return fmt.Errorf("provisioners[%d].tool must be gentle-ai", i)
		}
		if len(prov.Tags) == 0 {
			return fmt.Errorf("provisioners[%d].tags is required", i)
		}
		if j, ok := indexOfEmptyTag(prov.Tags); ok {
			return fmt.Errorf("provisioners[%d].tags[%d] must not be empty", i, j)
		}
		for j, osName := range prov.OS {
			if !allowedOS(osName) {
				return fmt.Errorf("provisioners[%d].os[%d] must be one of darwin, linux", i, j)
			}
		}
		if prov.Spec.IsEmpty() {
			return fmt.Errorf("provisioners[%d].spec is required", i)
		}
		if persona := strings.TrimSpace(prov.Spec.Persona); persona != "" && !allowedPersona(persona) {
			return fmt.Errorf("provisioners[%d].spec.persona must be one of gentleman, neutral", i)
		}
		if j, ok := indexOfEmptyString(prov.Spec.Agents); ok {
			return fmt.Errorf("provisioners[%d].spec.agents[%d] must not be empty", i, j)
		}
		if j, ok := indexOfEmptyString(prov.Spec.Components); ok {
			return fmt.Errorf("provisioners[%d].spec.components[%d] must not be empty", i, j)
		}
		if j, ok := indexOfEmptyString(prov.Spec.Skills); ok {
			return fmt.Errorf("provisioners[%d].spec.skills[%d] must not be empty", i, j)
		}
		for j, dep := range prov.Dependencies {
			if strings.TrimSpace(dep.Name) == "" {
				return fmt.Errorf("provisioners[%d].dependencies[%d].name is required", i, j)
			}
			if dep.Command != "" && strings.TrimSpace(dep.Command) == "" {
				return fmt.Errorf("provisioners[%d].dependencies[%d].command must not be empty", i, j)
			}
		}
	}

	return nil
}

// IsEmpty reports whether the spec declares no flags at all. A provisioner with
// an empty spec would render a bare `gentle-ai install` with nothing to do, so
// validation rejects it.
func (s ProvisionerSpec) IsEmpty() bool {
	return strings.TrimSpace(s.Scope) == "" &&
		strings.TrimSpace(s.Channel) == "" &&
		strings.TrimSpace(s.Persona) == "" &&
		strings.TrimSpace(s.SDDMode) == "" &&
		!hasNonEmptyString(s.Agents) &&
		!hasNonEmptyString(s.Components) &&
		!hasNonEmptyString(s.Skills)
}

func indexOfEmptyTag(tags []string) (int, bool) {
	return indexOfEmptyString(tags)
}

func indexOfEmptyString(values []string) (int, bool) {
	for i, value := range values {
		if strings.TrimSpace(value) == "" {
			return i, true
		}
	}
	return -1, false
}

func hasNonEmptyString(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
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

// allowedProvisionerTool enforces the provisioner allowlist. dots is never a
// generic command runner: only gentle-ai is an accepted provisioner tool in v1.
func allowedProvisionerTool(tool string) bool {
	return strings.TrimSpace(tool) == "gentle-ai"
}

// allowedPersona enforces the persona values gentle-ai ships as flag-driven
// presets. Any other value is rejected before dots renders the install command.
func allowedPersona(persona string) bool {
	switch persona {
	case "gentleman", "neutral":
		return true
	default:
		return false
	}
}
