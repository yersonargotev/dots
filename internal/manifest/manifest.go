package manifest

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/yersonargotev/dots/internal/tagpolicy"
	"gopkg.in/yaml.v3"
)

var (
	skillsPackageRefPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*/[A-Za-z0-9][A-Za-z0-9._-]*(/[A-Za-z0-9][A-Za-z0-9._-]*)*(@[A-Za-z0-9][A-Za-z0-9._./-]*)?$`)
	skillsDataValuePattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

type Manifest struct {
	Version      int                `yaml:"version"`
	Tags         map[string]Tag     `yaml:"tags,omitempty"`
	Profiles     map[string]Profile `yaml:"profiles"`
	Dependencies []DependencySet    `yaml:"dependencies,omitempty"`
	Entries      []Entry            `yaml:"entries"`
	Provisioners []Provisioner      `yaml:"provisioners,omitempty"`
}

type Profile struct {
	Description  string       `yaml:"description,omitempty"`
	Status       string       `yaml:"status,omitempty"`
	Tags         []string     `yaml:"tags"`
	Dependencies []Dependency `yaml:"dependencies,omitempty"`
}

// Tag describes a declared selection tag. The registry is optional for v1
// compatibility; when present, every tag referenced by the manifest must be
// declared here.
type Tag struct {
	Description string `yaml:"description,omitempty"`
	Kind        string `yaml:"kind"`
	Status      string `yaml:"status"`
	ReplacedBy  string `yaml:"replaced_by,omitempty"`
}

type Entry struct {
	Source          string            `yaml:"source"`
	SourceOverrides map[string]string `yaml:"source_overrides,omitempty"`
	Target          string            `yaml:"target"`
	// TargetRoot selects an allowlisted non-home root for relative targets.
	// Empty preserves the traditional ~/... target contract.
	TargetRoot   string       `yaml:"target_root,omitempty"`
	Strategy     string       `yaml:"strategy"`
	Ownership    string       `yaml:"ownership,omitempty"`
	Tags         []string     `yaml:"tags"`
	OS           []string     `yaml:"os,omitempty"`
	Dependencies []Dependency `yaml:"dependencies,omitempty"`
}

// DependencySet declares Dependencies selected directly by tags rather than by
// a specific Profile, Managed Entry, or Provisioner. It is for shared toolchain
// baselines such as the core development runtime set.
type DependencySet struct {
	Tags         []string     `yaml:"tags"`
	OS           []string     `yaml:"os,omitempty"`
	Dependencies []Dependency `yaml:"dependencies"`
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
// tool; the tool owns how they render into agent config. The Claude fields
// (Marketplace, Plugin, From) describe a single idempotent `claude` invocation, the MCP fields
// (MCP, Command, Env) a single MCP-server add invocation, Package drives
// one `npx --yes skills@1.5.12 add` invocation, and the codegraph dialect reuses
// Agents, Scope, and Yes to render a fixed bootstrap-and-install script. The
// dialects are mutually exclusive: a spec speaks exactly one of them.
type ProvisionerSpec struct {
	Scope  string   `yaml:"scope,omitempty"`
	Agents []string `yaml:"agents,omitempty"`
	Skills []string `yaml:"skills,omitempty"`
	Yes    bool     `yaml:"yes,omitempty"`
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
	// MCP names a Model Context Protocol server to register with an MCP-aware
	// provisioner. For Codex it renders `codex mcp add <mcp> [--env K=V]... --
	// <command...>`; for Claude it renders `claude mcp add --transport stdio
	// <mcp> -- <command...>`. It requires Command and is mutually exclusive with
	// unrelated tool dialects.
	MCP string `yaml:"mcp,omitempty"`
	// Command is the launch argv for the MCP server, rendered verbatim after the
	// `--` separator. It is required alongside MCP and only valid with it.
	Command []string `yaml:"command,omitempty"`
	// Env carries environment variables for the MCP server, rendered as repeated
	// `--env KEY=VALUE` flags in sorted-key order so the command stays
	// deterministic. It is optional and only valid alongside MCP.
	Env map[string]string `yaml:"env,omitempty"`
	// Package is the external skills.sh source reference passed to
	// `npx --yes skills@1.5.12 add <package>`, constrained to an owner/repo ref with an
	// optional path or @ref. It is only valid for the skills provisioner.
	Package string `yaml:"package,omitempty"`
	// Global installs skills into user-level agent directories instead of the
	// current project, rendering `--global` for the skills provisioner.
	Global bool `yaml:"global,omitempty"`
	// Copy asks skills.sh to copy files instead of symlinking them into the
	// target agent directories.
	Copy bool `yaml:"copy,omitempty"`
}

// Dependency is an external tool a Managed Entry needs to work correctly. dots
// checks for it and offers OS-aware installation guidance but never installs it
// automatically in v1. The package fields (Brew/Apt/Dnf/Pacman) carry the
// per-platform package identifier used to build advisory install guidance.
type Dependency struct {
	Name        string `yaml:"name"`
	Requirement string `yaml:"requirement,omitempty"`
	Command     string `yaml:"command,omitempty"`
	// DarwinApp names a macOS application bundle that can satisfy the
	// Dependency when its command is not available on PATH.
	DarwinApp string `yaml:"darwin_app,omitempty"`
	// Manual carries dependency-specific remediation for manual-only tools.
	Manual string `yaml:"manual,omitempty"`
	// ManualDebian carries Debian/Ubuntu-specific manual remediation.
	ManualDebian string `yaml:"manual_debian,omitempty"`
	Brew         string `yaml:"brew,omitempty"`
	// LinuxHomebrew allows Linux distro tiers to fall back to Homebrew when the
	// distro package is absent or unavailable. Keep this opt-in so GUI apps and
	// tools without Linuxbrew support stay manual instead of rendering false
	// installability on Ubuntu/Linux.
	LinuxHomebrew bool `yaml:"linux_homebrew,omitempty"`
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
	// FontFallbackMatches declares compatible installed-font filename globs that
	// satisfy the same dependency when the primary font file pattern is absent.
	FontFallbackMatches []string `yaml:"font_fallback_matches,omitempty"`
	// Commands declares every executable probe that must be present for this
	// Dependency to be satisfied. It is used for manager-owned toolchains where
	// both the manager and the runtime commands matter. When empty, Command or
	// Name remains the single probe for backwards compatibility.
	Commands []string `yaml:"commands,omitempty"`
	// Toolchain selects one of dots' built-in, constrained runtime bootstrap flows.
	// It is not arbitrary shell: each value maps to fixed argv-shaped commands.
	Toolchain string `yaml:"toolchain,omitempty"`
	// UserLocal explicitly opts this Dependency into one of dots' reviewed
	// home-owned providers. Go owns the allowlisted recipe; the manifest owns the
	// artifact version and checksum policy.
	UserLocal *UserLocalProvider `yaml:"user_local,omitempty"`
	// RollingUserLocal opts this Dependency into a closed rolling recipe. The
	// recipe owns its official metadata source and platform artifact policy; the
	// manifest cannot supply a version, URL, checksum, or command.
	RollingUserLocal *RollingUserLocalProvider `yaml:"rolling_user_local,omitempty"`
}

// UserLocalProvider is the manifest-owned policy for a reviewed User-Local
// Provider recipe. Checksums are keyed by dots platform (for example
// linux_amd64 or linux_arm64) and contain the expected SHA-256 hex digest.
type UserLocalProvider struct {
	Recipe    string            `yaml:"recipe"`
	Version   string            `yaml:"version"`
	Checksum  string            `yaml:"checksum,omitempty"`
	Checksums map[string]string `yaml:"checksums,omitempty"`
}

// RollingUserLocalProvider selects one reviewed high-cadence release recipe.
// Recipe is intentionally the only manifest-controlled field.
type RollingUserLocalProvider struct {
	Recipe string `yaml:"recipe"`
}

const (
	DependencyRequirementRequired = "required"
	DependencyRequirementOptional = "optional"

	DependencyToolchainNodeLTSFNM       = "node-lts-fnm"
	DependencyToolchainRustStableRustup = "rust-stable-rustup"
)

// RequirementValue returns the dependency's stable required/optional
// classification. Empty is treated as required for backwards compatibility.
func (d Dependency) RequirementValue() string {
	requirement := strings.TrimSpace(d.Requirement)
	if requirement == "" {
		return DependencyRequirementRequired
	}
	return requirement
}

// IsRequired reports whether the dependency gates installation.
func (d Dependency) IsRequired() bool {
	return d.RequirementValue() == DependencyRequirementRequired
}

// IsFont reports whether the Dependency is detected as an installed font asset
// rather than an executable on PATH. It is true when any font match pattern is set.
func (d Dependency) IsFont() bool {
	return len(d.FontMatches()) > 0
}

// FontMatches returns the primary installed-font filename glob followed by any
// compatible fallback globs, trimming whitespace and dropping blank patterns.
func (d Dependency) FontMatches() []string {
	matches := make([]string, 0, 1+len(d.FontFallbackMatches))
	seen := map[string]bool{}
	add := func(match string) {
		match = strings.TrimSpace(match)
		if match == "" || seen[match] {
			return
		}
		seen[match] = true
		matches = append(matches, match)
	}
	add(d.FontMatch)
	for _, match := range d.FontFallbackMatches {
		add(match)
	}
	return matches
}

// Probes are the command names used to detect a Dependency's presence in PATH.
// They default to Command or Name when a dependency does not declare multiple
// executable requirements.
func (d Dependency) Probes() []string {
	if len(d.Commands) > 0 {
		probes := make([]string, 0, len(d.Commands))
		seen := map[string]bool{}
		for _, command := range d.Commands {
			command = strings.TrimSpace(command)
			if command == "" || seen[command] {
				continue
			}
			seen[command] = true
			probes = append(probes, command)
		}
		return probes
	}
	return []string{d.Probe()}
}

// Probe is the primary command name used to detect a Dependency's presence in
// PATH. It defaults to Name when an entry does not declare an explicit command.
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
	return Parse(data)
}

// Parse strictly decodes and validates a current Install Manifest.
func Parse(data []byte) (*Manifest, error) {
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

// LoadPreviousFile reads the pre-update Install Manifest only for selection
// evolution. Provisioner specs are intentionally discarded before strict
// decoding because an older dots binary may have accepted a dialect that the
// current binary has retired. The returned Provisioners retain only inventory
// fields used to report removed surfaces; callers must never plan or execute
// them.
func LoadPreviousFile(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read previous manifest: %w", err)
	}

	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("parse previous manifest: %w", err)
	}
	discardPreviousProvisionerSpecs(&document)
	projected, err := yaml.Marshal(&document)
	if err != nil {
		return nil, fmt.Errorf("project previous manifest: %w", err)
	}

	var previous Manifest
	decoder := yaml.NewDecoder(bytes.NewReader(projected))
	decoder.KnownFields(true)
	if err := decoder.Decode(&previous); err != nil {
		return nil, fmt.Errorf("parse previous manifest: %w", err)
	}
	if err := previous.validateEvolutionInventory(); err != nil {
		return nil, err
	}
	return &previous, nil
}

func discardPreviousProvisionerSpecs(document *yaml.Node) {
	if document == nil || document.Kind != yaml.DocumentNode || len(document.Content) == 0 {
		return
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != "provisioners" {
			continue
		}
		for _, provisioner := range root.Content[i+1].Content {
			if provisioner.Kind != yaml.MappingNode {
				continue
			}
			for j := 0; j+1 < len(provisioner.Content); j += 2 {
				if provisioner.Content[j].Value == "spec" {
					provisioner.Content[j+1] = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
				}
			}
		}
		return
	}
}

func (m Manifest) validateEvolutionInventory() error {
	current := m
	current.Provisioners = nil
	if err := current.Validate(); err != nil {
		return err
	}
	for i, provisioner := range m.Provisioners {
		if strings.TrimSpace(provisioner.Tool) == "" {
			return fmt.Errorf("provisioners[%d].tool is required", i)
		}
		if len(provisioner.Tags) == 0 {
			return fmt.Errorf("provisioners[%d].tags is required", i)
		}
		if j, ok := indexOfEmptyTag(provisioner.Tags); ok {
			return fmt.Errorf("provisioners[%d].tags[%d] must not be empty", i, j)
		}
		for j, osName := range provisioner.OS {
			if !allowedOS(osName) {
				return fmt.Errorf("provisioners[%d].os[%d] must be one of darwin, linux", i, j)
			}
		}
		for j, dependency := range provisioner.Dependencies {
			if err := validateDependency(dependency, fmt.Sprintf("provisioners[%d].dependencies[%d]", i, j)); err != nil {
				return err
			}
		}
	}
	return nil
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
	if err := m.validateTagRegistry(); err != nil {
		return err
	}

	names := make([]string, 0, len(m.Profiles))
	for name := range m.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		profile := m.Profiles[name]
		if profile.Description != "" && strings.TrimSpace(profile.Description) == "" {
			return fmt.Errorf("profiles[%q].description must not be empty", name)
		}
		if profile.Status != "" && !tagpolicy.IsAllowedStatus(profile.Status) {
			return fmt.Errorf("profiles[%q].status must be one of current, legacy", name)
		}
		tags := profile.Tags
		if len(tags) == 0 {
			return fmt.Errorf("profiles[%q].tags is required", name)
		}
		if i, ok := indexOfEmptyTag(tags); ok {
			return fmt.Errorf("profiles[%q].tags[%d] must not be empty", name, i)
		}
		if err := m.validateDeclaredTags(tags, fmt.Sprintf("profiles[%q].tags", name)); err != nil {
			return err
		}
		for j, dep := range profile.Dependencies {
			if err := validateDependency(dep, fmt.Sprintf("profiles[%q].dependencies[%d]", name, j)); err != nil {
				return err
			}
		}
	}

	for i, set := range m.Dependencies {
		if len(set.Tags) == 0 {
			return fmt.Errorf("dependencies[%d].tags is required", i)
		}
		if j, ok := indexOfEmptyTag(set.Tags); ok {
			return fmt.Errorf("dependencies[%d].tags[%d] must not be empty", i, j)
		}
		if err := m.validateDeclaredTags(set.Tags, fmt.Sprintf("dependencies[%d].tags", i)); err != nil {
			return err
		}
		for j, osName := range set.OS {
			if !allowedOS(osName) {
				return fmt.Errorf("dependencies[%d].os[%d] must be one of darwin, linux", i, j)
			}
		}
		if len(set.Dependencies) == 0 {
			return fmt.Errorf("dependencies[%d].dependencies is required", i)
		}
		for j, dep := range set.Dependencies {
			if err := validateDependency(dep, fmt.Sprintf("dependencies[%d].dependencies[%d]", i, j)); err != nil {
				return err
			}
		}
	}

	for i, entry := range m.Entries {
		if entry.Source == "" {
			return fmt.Errorf("entries[%d].source is required", i)
		}
		if entry.Target == "" {
			return fmt.Errorf("entries[%d].target is required", i)
		}
		overrideTags := make([]string, 0, len(entry.SourceOverrides))
		for tag := range entry.SourceOverrides {
			overrideTags = append(overrideTags, tag)
		}
		sort.Strings(overrideTags)
		for _, tag := range overrideTags {
			if strings.TrimSpace(tag) == "" {
				return fmt.Errorf("entries[%d].source_overrides contains an empty tag", i)
			}
			if strings.TrimSpace(entry.SourceOverrides[tag]) == "" {
				return fmt.Errorf("entries[%d].source_overrides[%q] must not be empty", i, tag)
			}
			if err := m.validateDeclaredTags([]string{tag}, fmt.Sprintf("entries[%d].source_overrides", i)); err != nil {
				return err
			}
		}
		if !allowedStrategy(entry.Strategy) {
			return fmt.Errorf("entries[%d].strategy must be one of copy, symlink, template", i)
		}
		if !allowedTargetRoot(entry.TargetRoot) {
			return fmt.Errorf("entries[%d].target_root must be xdg-state when set", i)
		}
		if entry.TargetRoot == "xdg-state" {
			if filepath.IsAbs(entry.Target) || !filepath.IsLocal(entry.Target) || entry.Target == "." {
				return fmt.Errorf("entries[%d].target must be a confined relative path for target_root xdg-state", i)
			}
			if entry.Ownership != "seeded" {
				return fmt.Errorf("entries[%d].target_root xdg-state requires seeded ownership", i)
			}
		}
		if !allowedOwnership(entry.Ownership) {
			return fmt.Errorf("entries[%d].ownership must be one of json-subset, jsonc-subset, toml-subset, marked-block, seeded", i)
		}
		if entry.Ownership != "" && entry.Strategy != "copy" {
			return fmt.Errorf("entries[%d].ownership %s requires strategy copy", i, entry.Ownership)
		}
		if len(entry.Tags) == 0 {
			return fmt.Errorf("entries[%d].tags is required", i)
		}
		if j, ok := indexOfEmptyTag(entry.Tags); ok {
			return fmt.Errorf("entries[%d].tags[%d] must not be empty", i, j)
		}
		if err := m.validateDeclaredTags(entry.Tags, fmt.Sprintf("entries[%d].tags", i)); err != nil {
			return err
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
			return fmt.Errorf("provisioners[%d].tool must be one of claude, codegraph, codex, skills, zimfw", i)
		}
		if len(prov.Tags) == 0 {
			return fmt.Errorf("provisioners[%d].tags is required", i)
		}
		if j, ok := indexOfEmptyTag(prov.Tags); ok {
			return fmt.Errorf("provisioners[%d].tags[%d] must not be empty", i, j)
		}
		if err := m.validateDeclaredTags(prov.Tags, fmt.Sprintf("provisioners[%d].tags", i)); err != nil {
			return err
		}
		for j, osName := range prov.OS {
			if !allowedOS(osName) {
				return fmt.Errorf("provisioners[%d].os[%d] must be one of darwin, linux", i, j)
			}
		}
		if prov.Spec.IsEmpty() {
			return fmt.Errorf("provisioners[%d].spec is required", i)
		}
		switch prov.Tool {
		case "claude":
			if err := validateClaudeSpec(prov.Spec, i); err != nil {
				return err
			}
		case "codex":
			if err := validateCodexSpec(prov.Spec, i); err != nil {
				return err
			}
		case "codegraph":
			if err := validateCodeGraphSpec(prov.Spec, i); err != nil {
				return err
			}
		case "skills":
			if err := validateSkillsSpec(prov.Spec, i); err != nil {
				return err
			}
		case "zimfw":
			if err := validateZimFWSpec(prov.Spec, i); err != nil {
				return err
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

func (m Manifest) validateTagRegistry() error {
	if m.Tags == nil {
		return nil
	}
	names := make([]string, 0, len(m.Tags))
	for name := range m.Tags {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("tags contains an empty tag name")
		}
		tag := m.Tags[name]
		if tag.Description != "" && strings.TrimSpace(tag.Description) == "" {
			return fmt.Errorf("tags[%q].description must not be empty", name)
		}
		if !tagpolicy.IsAllowedKind(tag.Kind) {
			return fmt.Errorf("tags[%q].kind must be one of surface, cleanup, compatibility", name)
		}
		if expectedKind, isBehaviorTag := tagpolicy.ExpectedKind(name); isBehaviorTag && tag.Kind != expectedKind {
			return fmt.Errorf("tags[%q].kind must be %q", name, expectedKind)
		}
		if (tag.Kind == "cleanup" || tag.Kind == "compatibility") && !tagpolicy.IsBehaviorTag(name) {
			return fmt.Errorf("tags[%q].kind %q requires a supported behavior tag", name, tag.Kind)
		}
		if !tagpolicy.IsAllowedStatus(tag.Status) {
			return fmt.Errorf("tags[%q].status must be one of current, legacy", name)
		}
		if tag.ReplacedBy == "" {
			continue
		}
		if tag.Status != "legacy" {
			return fmt.Errorf("tags[%q].replaced_by requires status legacy", name)
		}
		replacement, ok := m.Tags[tag.ReplacedBy]
		if !ok {
			return fmt.Errorf("tags[%q].replaced_by %q is not declared", name, tag.ReplacedBy)
		}
		if replacement.Status != "current" {
			return fmt.Errorf("tags[%q].replaced_by %q must reference a current tag", name, tag.ReplacedBy)
		}
	}
	return nil
}

func (m Manifest) validateDeclaredTags(tags []string, path string) error {
	if m.Tags == nil {
		return nil
	}
	for i, tag := range tags {
		if _, ok := m.Tags[tag]; !ok {
			return fmt.Errorf("%s[%d] tag %q is not declared", path, i, tag)
		}
	}
	return nil
}

func validateDependency(dep Dependency, path string) error {
	if strings.TrimSpace(dep.Name) == "" {
		return fmt.Errorf("%s.name is required", path)
	}
	requirement := strings.TrimSpace(dep.Requirement)
	if requirement != "" && requirement != DependencyRequirementRequired && requirement != DependencyRequirementOptional {
		return fmt.Errorf("%s.requirement must be one of required, optional", path)
	}
	if dep.Command != "" && strings.TrimSpace(dep.Command) == "" {
		return fmt.Errorf("%s.command must not be empty", path)
	}
	if dep.DarwinApp != "" {
		app := strings.TrimSpace(dep.DarwinApp)
		if app == "" || strings.ContainsAny(app, `/\`) || !strings.HasSuffix(app, ".app") {
			return fmt.Errorf("%s.darwin_app must be an .app bundle name without a path", path)
		}
	}
	if dep.Manual != "" && strings.TrimSpace(dep.Manual) == "" {
		return fmt.Errorf("%s.manual must not be empty", path)
	}
	if dep.ManualDebian != "" && strings.TrimSpace(dep.ManualDebian) == "" {
		return fmt.Errorf("%s.manual_debian must not be empty", path)
	}
	for i, command := range dep.Commands {
		if strings.TrimSpace(command) == "" {
			return fmt.Errorf("%s.commands[%d] must not be empty", path, i)
		}
	}
	if dep.Toolchain != "" && dep.Toolchain != DependencyToolchainNodeLTSFNM && dep.Toolchain != DependencyToolchainRustStableRustup {
		return fmt.Errorf("%s.toolchain must be one of node-lts-fnm, rust-stable-rustup", path)
	}
	if dep.BrewCask != "" && strings.TrimSpace(dep.BrewCask) == "" {
		return fmt.Errorf("%s.brew_cask must not be empty", path)
	}
	for i, match := range dep.FontFallbackMatches {
		if strings.TrimSpace(match) == "" {
			return fmt.Errorf("%s.font_fallback_matches[%d] must not be empty", path, i)
		}
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
	return strings.TrimSpace(s.Scope) == "" &&
		!hasNonEmptyString(s.Agents) &&
		!hasNonEmptyString(s.Skills) &&
		!s.Yes &&
		!s.usesClaudeFields() &&
		!s.usesMCPFields() &&
		!s.usesSkillsFields()
}

// usesClaudeFields reports whether the spec sets any Claude-specific field. It
// keeps the tool dialects from silently bleeding into each other.
func (s ProvisionerSpec) usesClaudeFields() bool {
	return strings.TrimSpace(s.Marketplace) != "" ||
		strings.TrimSpace(s.Plugin) != "" ||
		strings.TrimSpace(s.From) != ""
}

// usesMCPFields reports whether the spec sets any MCP-server field. It
// keeps the MCP dialect disjoint from the other tool dialects.
func (s ProvisionerSpec) usesMCPFields() bool {
	return strings.TrimSpace(s.MCP) != "" ||
		hasNonEmptyString(s.Command) ||
		len(s.Env) > 0
}

// usesSkillsFields reports whether the spec sets any skills.sh-specific field.
func (s ProvisionerSpec) usesSkillsFields() bool {
	return strings.TrimSpace(s.Package) != "" ||
		s.Global ||
		s.Copy
}

// validateClaudeSpec enforces the claude provisioner contract: it drives exactly
// one idempotent invocation — a marketplace registration, a plugin install
// (which needs a From marketplace), or a stdio MCP server registration.
func validateClaudeSpec(s ProvisionerSpec, i int) error {
	if strings.TrimSpace(s.Scope) != "" || hasNonEmptyString(s.Agents) || hasNonEmptyString(s.Skills) || s.Yes {
		return fmt.Errorf("provisioners[%d].spec must not set scope, agents, skills, or yes for the claude tool", i)
	}
	if s.usesSkillsFields() {
		return fmt.Errorf("provisioners[%d].spec must not set skills.sh fields (package, global, copy) for the claude tool", i)
	}
	hasMarketplace := strings.TrimSpace(s.Marketplace) != ""
	hasPlugin := strings.TrimSpace(s.Plugin) != ""
	hasMCP := strings.TrimSpace(s.MCP) != ""
	shapes := 0
	for _, set := range []bool{hasMarketplace, hasPlugin, hasMCP} {
		if set {
			shapes++
		}
	}
	if shapes != 1 {
		return fmt.Errorf("provisioners[%d].spec must set exactly one of marketplace, plugin, or mcp for the claude tool", i)
	}
	if hasPlugin && strings.TrimSpace(s.From) == "" {
		return fmt.Errorf("provisioners[%d].spec.from is required when plugin is set", i)
	}
	if (hasMarketplace || hasMCP) && strings.TrimSpace(s.From) != "" {
		return fmt.Errorf("provisioners[%d].spec.from is only valid alongside plugin", i)
	}
	hasCommand := hasNonEmptyString(s.Command)
	if hasMCP {
		if !hasCommand {
			return fmt.Errorf("provisioners[%d].spec.command is required when mcp is set", i)
		}
		if len(s.Env) > 0 {
			return fmt.Errorf("provisioners[%d].spec.env is only valid for the codex tool", i)
		}
		return nil
	}
	if hasCommand {
		return fmt.Errorf("provisioners[%d].spec.command is only valid when mcp is set", i)
	}
	if len(s.Env) > 0 {
		return fmt.Errorf("provisioners[%d].spec.env is only valid when mcp is set", i)
	}
	return nil
}

// validateCodexSpec enforces the codex provisioner contract: it drives exactly
// one idempotent `codex mcp add` invocation — an MCP server name plus its launch
// command — and never mixes in the Claude or skills dialects.
func validateCodexSpec(s ProvisionerSpec, i int) error {
	if strings.TrimSpace(s.Scope) != "" || hasNonEmptyString(s.Agents) || hasNonEmptyString(s.Skills) || s.Yes {
		return fmt.Errorf("provisioners[%d].spec must not set scope, agents, skills, or yes for the codex tool", i)
	}
	if s.usesClaudeFields() {
		return fmt.Errorf("provisioners[%d].spec must not set claude fields (marketplace, plugin, from) for the codex tool", i)
	}
	if s.usesSkillsFields() {
		return fmt.Errorf("provisioners[%d].spec must not set skills.sh fields (package, global, copy) for the codex tool", i)
	}
	if strings.TrimSpace(s.MCP) == "" {
		return fmt.Errorf("provisioners[%d].spec.mcp is required for the codex tool", i)
	}
	if !hasNonEmptyString(s.Command) {
		return fmt.Errorf("provisioners[%d].spec.command is required when mcp is set", i)
	}
	for key := range s.Env {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("provisioners[%d].spec.env has an empty key", i)
		}
	}
	return nil
}

// validateCodeGraphSpec enforces the CodeGraph installer contract: it drives one
// non-interactive install through a fixed shell script so dots can bootstrap the
// codegraph binary when it is absent, then wire the selected agents to
// `codegraph serve --mcp`.
func validateCodeGraphSpec(s ProvisionerSpec, i int) error {
	if s.usesClaudeFields() {
		return fmt.Errorf("provisioners[%d].spec must not set claude fields (marketplace, plugin, from) for the codegraph tool", i)
	}
	if s.usesMCPFields() {
		return fmt.Errorf("provisioners[%d].spec must not set MCP fields (mcp, command, env) for the codegraph tool", i)
	}
	if s.usesSkillsFields() {
		return fmt.Errorf("provisioners[%d].spec must not set skills.sh fields (package, global, copy) for the codegraph tool", i)
	}
	if hasNonEmptyString(s.Skills) {
		return fmt.Errorf("provisioners[%d].spec must not set skills for the codegraph tool", i)
	}
	if !hasNonEmptyString(s.Agents) {
		return fmt.Errorf("provisioners[%d].spec.agents is required for the codegraph tool", i)
	}
	if !s.Yes {
		return fmt.Errorf("provisioners[%d].spec.yes must be true for the codegraph tool", i)
	}
	if j, ok := indexOfEmptyString(s.Agents); ok {
		return fmt.Errorf("provisioners[%d].spec.agents[%d] must not be empty", i, j)
	}
	if err := validateSkillsDataValues(s.Agents, fmt.Sprintf("provisioners[%d].spec.agents", i)); err != nil {
		return err
	}
	for j, agent := range s.Agents {
		if !allowedCodeGraphAgent(strings.TrimSpace(agent)) {
			return fmt.Errorf("provisioners[%d].spec.agents[%d] must be one of antigravity, claude, codex, opencode for the codegraph tool", i, j)
		}
	}
	scope := strings.TrimSpace(s.Scope)
	if scope != "" && scope != "global" && scope != "local" {
		return fmt.Errorf("provisioners[%d].spec.scope must be one of global, local for the codegraph tool", i)
	}
	return nil
}

func allowedCodeGraphAgent(agent string) bool {
	switch agent {
	case "antigravity", "claude", "codex", "opencode":
		return true
	default:
		return false
	}
}

// validateSkillsSpec enforces the skills.sh provisioner contract: it drives one
// exact `npx --yes skills@1.5.12 add <package>` invocation with optional target agents and
// selected skill names. It does not accept unrelated scalar, Claude, or MCP fields.
func validateSkillsSpec(s ProvisionerSpec, i int) error {
	if s.usesClaudeFields() {
		return fmt.Errorf("provisioners[%d].spec must not set claude fields (marketplace, plugin, from) for the skills tool", i)
	}
	if s.usesMCPFields() {
		return fmt.Errorf("provisioners[%d].spec must not set MCP fields (mcp, command, env) for the skills tool", i)
	}
	if strings.TrimSpace(s.Scope) != "" || s.Yes {
		return fmt.Errorf("provisioners[%d].spec must not set scope or yes for the skills tool", i)
	}
	if strings.TrimSpace(s.Package) == "" {
		return fmt.Errorf("provisioners[%d].spec.package is required for the skills tool", i)
	}
	if err := validateSkillsPackageRef(s.Package, fmt.Sprintf("provisioners[%d].spec.package", i)); err != nil {
		return err
	}
	if !s.Global {
		return fmt.Errorf("provisioners[%d].spec.global must be true for the skills tool", i)
	}
	if j, ok := indexOfEmptyString(s.Agents); ok {
		return fmt.Errorf("provisioners[%d].spec.agents[%d] must not be empty", i, j)
	}
	if j, ok := indexOfEmptyString(s.Skills); ok {
		return fmt.Errorf("provisioners[%d].spec.skills[%d] must not be empty", i, j)
	}
	if err := validateSkillsDataValues(s.Agents, fmt.Sprintf("provisioners[%d].spec.agents", i)); err != nil {
		return err
	}
	if err := validateSkillsDataValues(s.Skills, fmt.Sprintf("provisioners[%d].spec.skills", i)); err != nil {
		return err
	}
	return nil
}

// validateZimFWSpec enforces the Zim runtime bootstrap contract: dots owns a
// single non-interactive install/init invocation for ~/.zim and accepts no
// user-shaped command fields.
func validateZimFWSpec(s ProvisionerSpec, i int) error {
	if s.usesClaudeFields() {
		return fmt.Errorf("provisioners[%d].spec must not set claude fields (marketplace, plugin, from) for the zimfw tool", i)
	}
	if s.usesMCPFields() {
		return fmt.Errorf("provisioners[%d].spec must not set MCP fields (mcp, command, env) for the zimfw tool", i)
	}
	if s.usesSkillsFields() {
		return fmt.Errorf("provisioners[%d].spec must not set skills.sh fields (package, global, copy) for the zimfw tool", i)
	}
	if strings.TrimSpace(s.Scope) != "" || hasNonEmptyString(s.Agents) || hasNonEmptyString(s.Skills) {
		return fmt.Errorf("provisioners[%d].spec must not set scope, agents, or skills for the zimfw tool", i)
	}
	if !s.Yes {
		return fmt.Errorf("provisioners[%d].spec.yes must be true for the zimfw tool", i)
	}
	return nil
}

func validateSkillsPackageRef(value, path string) error {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "-") {
		return fmt.Errorf("%s must be a package reference, not a CLI flag", path)
	}
	if containsControl(value) {
		return fmt.Errorf("%s must not contain control characters", path)
	}
	if strings.ContainsAny(value, " \t") || !skillsPackageRefPattern.MatchString(value) {
		return fmt.Errorf("%s must be an owner/repo package reference with optional path or @ref", path)
	}
	return nil
}

func validateSkillsDataValues(values []string, path string) error {
	for i, value := range values {
		itemPath := fmt.Sprintf("%s[%d]", path, i)
		if containsControl(value) {
			return fmt.Errorf("%s must not contain control characters", itemPath)
		}
		value = strings.TrimSpace(value)
		if strings.HasPrefix(value, "-") {
			return fmt.Errorf("%s must be data, not a CLI flag", itemPath)
		}
		if !skillsDataValuePattern.MatchString(value) {
			return fmt.Errorf("%s must contain only letters, digits, dots, underscores, and hyphens", itemPath)
		}
	}
	return nil
}

func containsControl(value string) bool {
	return strings.ContainsFunc(value, unicode.IsControl)
}

// Selection describes the ordered Profile and Tag union selected by a command.
// Profiles preserves the user-supplied Profile order after de-duplication; Tags
// preserves the ordered union of all selected Profile tags plus explicit --tag
// values. Profile is a compatibility label for existing metadata and prose.
type Selection struct {
	Profile  string
	Profiles []string
	Tags     []string
}

// SelectedProfileNames returns the repeatable profile list when present, or the
// legacy single-profile option used by tests and internal callers.
func SelectedProfileNames(profile string, profiles []string) []string {
	if len(profiles) > 0 {
		return profiles
	}
	if profile == "" {
		return nil
	}
	return []string{profile}
}

// ResolveSelection validates selected Profile names and returns their ordered,
// de-duplicated tag union. Repository manifests without a legacy default Profile
// require an explicit Profile so install never falls back to an implicit baseline.
func ResolveSelection(m Manifest, profileNames []string, extraTags []string) (Selection, error) {
	return resolveSelection(m, profileNames, extraTags, true)
}

// ResolveReadOnlySelection validates a read-only command selection without
// inferring a legacy default Profile. Explicit Tags may form the complete
// selection when no Profile was supplied.
func ResolveReadOnlySelection(m Manifest, profileNames []string, extraTags []string) (Selection, error) {
	return resolveSelection(m, profileNames, extraTags, false)
}

func resolveSelection(m Manifest, profileNames []string, extraTags []string, allowDefault bool) (Selection, error) {
	profiles := make([]string, 0, len(profileNames))
	profileSeen := make(map[string]bool, len(profileNames))
	tags := make([]string, 0, len(extraTags))
	tagSeen := map[string]bool{}
	addTag := func(tag string) {
		if tagSeen[tag] {
			return
		}
		tagSeen[tag] = true
		tags = append(tags, tag)
	}
	for _, name := range profileNames {
		if name == "" || profileSeen[name] {
			continue
		}
		profile, ok := m.Profiles[name]
		if !ok {
			return Selection{}, fmt.Errorf("profile %q not found", name)
		}
		profileSeen[name] = true
		profiles = append(profiles, name)
		for _, tag := range profile.Tags {
			addTag(tag)
		}
	}
	if len(profiles) == 0 && allowDefault {
		if profile, ok := m.Profiles["default"]; ok {
			profiles = append(profiles, "default")
			for _, tag := range profile.Tags {
				addTag(tag)
			}
		} else {
			return Selection{}, fmt.Errorf("at least one --profile is required; choose one or repeat --profile to compose profiles (for example: --profile core, --profile workstation, or --profile agents --profile web)")
		}
	}
	for _, tag := range extraTags {
		addTag(tag)
	}
	return Selection{Profile: strings.Join(profiles, ","), Profiles: profiles, Tags: tags}, nil
}

// SelectionTags returns the effective tag set for a profile plus any
// explicit tags requested by the caller. The profile tags stay first so selected
// entries and provisioners preserve the manifest's role-oriented baseline while
// --tag can opt into one-off capabilities without creating feature profiles.
func SelectionTags(profile Profile, extraTags []string) []string {
	tags := append([]string(nil), profile.Tags...)
	seen := make(map[string]bool, len(tags)+len(extraTags))
	for _, tag := range tags {
		seen[tag] = true
	}
	for _, tag := range extraTags {
		if seen[tag] {
			continue
		}
		seen[tag] = true
		tags = append(tags, tag)
	}
	return tags
}

func EntrySource(entry Entry, selectionTags []string) string {
	for i := len(selectionTags) - 1; i >= 0; i-- {
		if source, ok := entry.SourceOverrides[selectionTags[i]]; ok {
			return source
		}
	}
	return entry.Source
}

// SharesTag reports whether an item's tags intersect a profile's tags. It is
// half the profile-scoping rule entries and provisioners share: an item belongs
// to a profile when at least one of its tags matches one of the profile's tags.
// Centralizing it here keeps plan, status, deps, doctor, and provision filtering
// through one rule instead of each carrying its own copy.
func SharesTag(itemTags, profileTags []string) bool {
	for _, it := range itemTags {
		for _, pt := range profileTags {
			if it == pt {
				return true
			}
		}
	}
	return false
}

// MatchesOS reports whether an item's OS filter admits the current OS. An empty
// filter matches every OS. The comparison is case-insensitive so a manifest that
// declared an OS in mixed case still matches runtime.GOOS, which is always
// lowercase; validation already constrains OS values to lowercase darwin/linux,
// so this is defensive rather than load-bearing. It is the other half of the
// profile-scoping rule SharesTag begins.
func MatchesOS(itemOS []string, currentOS string) bool {
	if len(itemOS) == 0 {
		return true
	}
	for _, osName := range itemOS {
		if strings.EqualFold(osName, currentOS) {
			return true
		}
	}
	return false
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

func allowedOwnership(ownership string) bool {
	switch ownership {
	case "", "json-subset", "jsonc-subset", "toml-subset", "marked-block", "seeded":
		return true
	default:
		return false
	}
}

func allowedTargetRoot(root string) bool {
	return root == "" || root == "xdg-state"
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
// generic command runner: claude, codex, codegraph, skills, and zimfw
// are the only accepted provisioner tools, each driven through a fixed set of
// subcommands.
func allowedProvisionerTool(tool string) bool {
	switch strings.TrimSpace(tool) {
	case "claude", "codex", "codegraph", "skills", "zimfw":
		return true
	default:
		return false
	}
}
