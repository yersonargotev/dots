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

// ProvisionerSpec carries the declarative values dots owns for an allowlisted
// tool; the tool owns how they render into agent config. The scalar/list flags
// map 1:1 to gentle-ai's install flags. The Claude fields (Marketplace, Plugin,
// From) describe a single idempotent `claude` invocation instead — they are
// mutually exclusive with each other and with the gentle-ai flags.
type ProvisionerSpec struct {
	Scope      string   `yaml:"scope,omitempty"`
	Channel    string   `yaml:"channel,omitempty"`
	Persona    string   `yaml:"persona,omitempty"`
	SDDMode    string   `yaml:"sdd-mode,omitempty"`
	Agents     []string `yaml:"agents,omitempty"`
	Components []string `yaml:"components,omitempty"`
	Skills     []string `yaml:"skills,omitempty"`
	// Marketplace registers a Claude Code plugin marketplace from a source
	// (GitHub repo, URL, or path), rendering `claude plugin marketplace add
	// <source>`. The marketplace name is derived by Claude from the source.
	Marketplace string `yaml:"marketplace,omitempty"`
	// Plugin installs a plugin from an already-registered marketplace, rendering
	// `claude plugin install <plugin>@<from> --scope user`. It requires From.
	Plugin string `yaml:"plugin,omitempty"`
	// From names the marketplace a Plugin is installed from. It is only valid
	// alongside Plugin.
	From string `yaml:"from,omitempty"`
}

// Dependency is an external tool a Managed Entry needs to work correctly. dots
// checks for it and offers OS-aware installation guidance but never installs it
// automatically in v1. The package fields (Brew/Apt/Dnf/Pacman) carry the
// per-platform package identifier used to build advisory install guidance.
type Dependency struct {
	Name    string `yaml:"name"`
	Command string `yaml:"command,omitempty"`
	Brew    string `yaml:"brew,omitempty"`
	// BrewCask declares a Homebrew cask package. It renders as
	// `brew install --cask <token>` and is separate from Brew so casks are not
	// hidden behind Homebrew's implicit formula/cask resolution.
	BrewCask string `yaml:"brew_cask,omitempty"`
	Apt      string `yaml:"apt,omitempty"`
	Dnf      string `yaml:"dnf,omitempty"`
	Pacman   string `yaml:"pacman,omitempty"`
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
			if err := validateDependency(dep, fmt.Sprintf("entries[%d].dependencies[%d]", i, j)); err != nil {
				return err
			}
		}
	}

	for i, prov := range m.Provisioners {
		if strings.TrimSpace(prov.Tool) == "" {
			return fmt.Errorf("provisioners[%d].tool is required", i)
		}
		if !allowedProvisionerTool(prov.Tool) {
			return fmt.Errorf("provisioners[%d].tool must be one of claude, gentle-ai", i)
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
		if prov.Tool == "claude" {
			if err := validateClaudeSpec(prov.Spec, i); err != nil {
				return err
			}
		} else {
			if prov.Spec.usesClaudeFields() {
				return fmt.Errorf("provisioners[%d].spec must not set claude fields (marketplace, plugin, from) for the gentle-ai tool", i)
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
		}
		for j, dep := range prov.Dependencies {
			if err := validateDependency(dep, fmt.Sprintf("provisioners[%d].dependencies[%d]", i, j)); err != nil {
				return err
			}
		}
	}

	return nil
}

func validateDependency(dep Dependency, path string) error {
	if strings.TrimSpace(dep.Name) == "" {
		return fmt.Errorf("%s.name is required", path)
	}
	if dep.Command != "" && strings.TrimSpace(dep.Command) == "" {
		return fmt.Errorf("%s.command must not be empty", path)
	}
	if dep.BrewCask != "" && strings.TrimSpace(dep.BrewCask) == "" {
		return fmt.Errorf("%s.brew_cask must not be empty", path)
	}
	if strings.TrimSpace(dep.Brew) != "" && strings.TrimSpace(dep.BrewCask) != "" {
		return fmt.Errorf("%s must not set both brew and brew_cask", path)
	}
	return nil
}

// IsEmpty reports whether the spec declares no values at all. A provisioner with
// an empty spec would render a bare command with nothing to do, so validation
// rejects it.
func (s ProvisionerSpec) IsEmpty() bool {
	return !s.usesGentleAIFlags() && !s.usesClaudeFields()
}

// usesClaudeFields reports whether the spec sets any Claude-specific field. It
// keeps the two tool dialects from silently bleeding into each other.
func (s ProvisionerSpec) usesClaudeFields() bool {
	return strings.TrimSpace(s.Marketplace) != "" ||
		strings.TrimSpace(s.Plugin) != "" ||
		strings.TrimSpace(s.From) != ""
}

// usesGentleAIFlags reports whether the spec sets any gentle-ai install flag.
func (s ProvisionerSpec) usesGentleAIFlags() bool {
	return strings.TrimSpace(s.Scope) != "" ||
		strings.TrimSpace(s.Channel) != "" ||
		strings.TrimSpace(s.Persona) != "" ||
		strings.TrimSpace(s.SDDMode) != "" ||
		hasNonEmptyString(s.Agents) ||
		hasNonEmptyString(s.Components) ||
		hasNonEmptyString(s.Skills)
}

// validateClaudeSpec enforces the claude provisioner contract: it drives exactly
// one idempotent invocation — either a marketplace registration or a plugin
// install (which needs a From marketplace) — and never mixes in gentle-ai flags.
func validateClaudeSpec(s ProvisionerSpec, i int) error {
	if s.usesGentleAIFlags() {
		return fmt.Errorf("provisioners[%d].spec must not set gentle-ai install flags for the claude tool", i)
	}
	hasMarketplace := strings.TrimSpace(s.Marketplace) != ""
	hasPlugin := strings.TrimSpace(s.Plugin) != ""
	if hasMarketplace == hasPlugin {
		return fmt.Errorf("provisioners[%d].spec must set exactly one of marketplace or plugin for the claude tool", i)
	}
	if hasPlugin && strings.TrimSpace(s.From) == "" {
		return fmt.Errorf("provisioners[%d].spec.from is required when plugin is set", i)
	}
	if hasMarketplace && strings.TrimSpace(s.From) != "" {
		return fmt.Errorf("provisioners[%d].spec.from is only valid alongside plugin", i)
	}
	return nil
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
// generic command runner: gentle-ai and claude are the only accepted provisioner
// tools, each driven through a fixed set of subcommands.
func allowedProvisionerTool(tool string) bool {
	switch strings.TrimSpace(tool) {
	case "gentle-ai", "claude":
		return true
	default:
		return false
	}
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
