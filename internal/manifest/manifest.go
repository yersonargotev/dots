package manifest

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

var (
	skillsPackageRefPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*/[A-Za-z0-9][A-Za-z0-9._-]*(/[A-Za-z0-9][A-Za-z0-9._-]*)*(@[A-Za-z0-9][A-Za-z0-9._./-]*)?$`)
	skillsDataValuePattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

type Manifest struct {
	Version      int                `yaml:"version"`
	Profiles     map[string]Profile `yaml:"profiles"`
	Entries      []Entry            `yaml:"entries"`
	Provisioners []Provisioner      `yaml:"provisioners,omitempty"`
}

type Profile struct {
	Tags         []string     `yaml:"tags"`
	Dependencies []Dependency `yaml:"dependencies,omitempty"`
}

type Entry struct {
	Source       string       `yaml:"source"`
	Target       string       `yaml:"target"`
	Strategy     string       `yaml:"strategy"`
	Ownership    string       `yaml:"ownership,omitempty"`
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
// map 1:1 to gentle-ai's install/uninstall flags. The Claude fields (Marketplace, Plugin,
// From) describe a single idempotent `claude` invocation, the codex MCP fields
// (MCP, Command, Env) a single `codex mcp add` invocation, Package drives
// one `npx --yes skills@1.5.12 add` invocation, and the codegraph dialect reuses
// Agents, Scope, and Yes to render a fixed bootstrap-and-install script. The
// dialects are mutually exclusive: a spec speaks exactly one of them.
type ProvisionerSpec struct {
	Action     string   `yaml:"action,omitempty"`
	Scope      string   `yaml:"scope,omitempty"`
	Channel    string   `yaml:"channel,omitempty"`
	Persona    string   `yaml:"persona,omitempty"`
	Preset     string   `yaml:"preset,omitempty"`
	SDDMode    string   `yaml:"sdd-mode,omitempty"`
	Agents     []string `yaml:"agents,omitempty"`
	Components []string `yaml:"components,omitempty"`
	Skills     []string `yaml:"skills,omitempty"`
	Yes        bool     `yaml:"yes,omitempty"`
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
	// MCP names a Model Context Protocol server to register with the codex tool,
	// rendering `codex mcp add <mcp> [--env K=V]... -- <command...>`. It requires
	// Command and is mutually exclusive with the gentle-ai and claude fields.
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
	// FontFallbackMatches declares compatible installed-font filename globs that
	// satisfy the same dependency when the primary font file pattern is absent.
	FontFallbackMatches []string `yaml:"font_fallback_matches,omitempty"`
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
		profile := m.Profiles[name]
		tags := profile.Tags
		if len(tags) == 0 {
			return fmt.Errorf("profiles[%q].tags is required", name)
		}
		if i, ok := indexOfEmptyTag(tags); ok {
			return fmt.Errorf("profiles[%q].tags[%d] must not be empty", name, i)
		}
		for j, dep := range profile.Dependencies {
			if err := validateDependency(dep, fmt.Sprintf("profiles[%q].dependencies[%d]", name, j)); err != nil {
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
		if !allowedStrategy(entry.Strategy) {
			return fmt.Errorf("entries[%d].strategy must be one of copy, symlink, template", i)
		}
		if !allowedOwnership(entry.Ownership) {
			return fmt.Errorf("entries[%d].ownership must be one of json-subset, toml-subset", i)
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
			return fmt.Errorf("provisioners[%d].tool must be one of claude, codegraph, codex, gentle-ai, skills", i)
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
		default:
			if prov.Spec.usesClaudeFields() {
				return fmt.Errorf("provisioners[%d].spec must not set claude fields (marketplace, plugin, from) for the gentle-ai tool", i)
			}
			if prov.Spec.usesMCPFields() {
				return fmt.Errorf("provisioners[%d].spec must not set codex MCP fields (mcp, command, env) for the gentle-ai tool", i)
			}
			if prov.Spec.usesSkillsFields() {
				return fmt.Errorf("provisioners[%d].spec must not set skills.sh fields (package, global, copy) for the gentle-ai tool", i)
			}
			action := strings.TrimSpace(prov.Spec.Action)
			if !allowedGentleAIAction(action) {
				return fmt.Errorf("provisioners[%d].spec.action must be one of install, uninstall", i)
			}
			if prov.Spec.Yes && action != "uninstall" {
				return fmt.Errorf("provisioners[%d].spec.yes is only valid when action is uninstall", i)
			}
			if action == "uninstall" && prov.Spec.usesGentleAIInstallOnlyFlags() {
				return fmt.Errorf("provisioners[%d].spec uninstall action must not set install-only fields (scope, channel, persona, preset, sdd-mode, skills)", i)
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
	return !s.usesGentleAIFlags() && !s.usesClaudeFields() && !s.usesMCPFields() && !s.usesSkillsFields()
}

// usesClaudeFields reports whether the spec sets any Claude-specific field. It
// keeps the tool dialects from silently bleeding into each other.
func (s ProvisionerSpec) usesClaudeFields() bool {
	return strings.TrimSpace(s.Marketplace) != "" ||
		strings.TrimSpace(s.Plugin) != "" ||
		strings.TrimSpace(s.From) != ""
}

// usesMCPFields reports whether the spec sets any codex MCP-server field. It
// keeps the codex dialect disjoint from the gentle-ai and claude dialects.
func (s ProvisionerSpec) usesMCPFields() bool {
	return strings.TrimSpace(s.MCP) != "" ||
		hasNonEmptyString(s.Command) ||
		len(s.Env) > 0
}

// usesSkillsFields reports whether the spec sets any skills.sh field. Agents
// and Skills are intentionally shared names between gentle-ai and skills.sh, so
// they are not counted here; validation decides which tool owns those lists.
func (s ProvisionerSpec) usesSkillsFields() bool {
	return strings.TrimSpace(s.Package) != "" ||
		s.Global ||
		s.Copy
}

// usesGentleAIFlags reports whether the spec sets any gentle-ai install flag.
func (s ProvisionerSpec) usesGentleAIFlags() bool {
	return strings.TrimSpace(s.Action) != "" ||
		strings.TrimSpace(s.Scope) != "" ||
		strings.TrimSpace(s.Channel) != "" ||
		strings.TrimSpace(s.Persona) != "" ||
		strings.TrimSpace(s.Preset) != "" ||
		strings.TrimSpace(s.SDDMode) != "" ||
		hasNonEmptyString(s.Agents) ||
		hasNonEmptyString(s.Components) ||
		hasNonEmptyString(s.Skills) ||
		s.Yes
}

func (s ProvisionerSpec) usesGentleAIInstallOnlyFlags() bool {
	return strings.TrimSpace(s.Scope) != "" ||
		strings.TrimSpace(s.Channel) != "" ||
		strings.TrimSpace(s.Persona) != "" ||
		strings.TrimSpace(s.Preset) != "" ||
		strings.TrimSpace(s.SDDMode) != "" ||
		hasNonEmptyString(s.Skills)
}

func allowedGentleAIAction(action string) bool {
	return action == "" || action == "install" || action == "uninstall"
}

// validateClaudeSpec enforces the claude provisioner contract: it drives exactly
// one idempotent invocation — either a marketplace registration or a plugin
// install (which needs a From marketplace) — and never mixes in gentle-ai flags.
func validateClaudeSpec(s ProvisionerSpec, i int) error {
	if s.usesGentleAIFlags() {
		return fmt.Errorf("provisioners[%d].spec must not set gentle-ai install flags for the claude tool", i)
	}
	if s.usesMCPFields() {
		return fmt.Errorf("provisioners[%d].spec must not set codex MCP fields (mcp, command, env) for the claude tool", i)
	}
	if s.usesSkillsFields() {
		return fmt.Errorf("provisioners[%d].spec must not set skills.sh fields (package, global, copy) for the claude tool", i)
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

// validateCodexSpec enforces the codex provisioner contract: it drives exactly
// one idempotent `codex mcp add` invocation — an MCP server name plus its launch
// command — and never mixes in the gentle-ai or claude dialects.
func validateCodexSpec(s ProvisionerSpec, i int) error {
	if s.usesGentleAIFlags() {
		return fmt.Errorf("provisioners[%d].spec must not set gentle-ai install flags for the codex tool", i)
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
		return fmt.Errorf("provisioners[%d].spec must not set codex MCP fields (mcp, command, env) for the codegraph tool", i)
	}
	if s.usesSkillsFields() {
		return fmt.Errorf("provisioners[%d].spec must not set skills.sh fields (package, global, copy) for the codegraph tool", i)
	}
	if strings.TrimSpace(s.Action) != "" ||
		strings.TrimSpace(s.Channel) != "" ||
		strings.TrimSpace(s.Persona) != "" ||
		strings.TrimSpace(s.Preset) != "" ||
		strings.TrimSpace(s.SDDMode) != "" ||
		hasNonEmptyString(s.Components) ||
		hasNonEmptyString(s.Skills) {
		return fmt.Errorf("provisioners[%d].spec must not set gentle-ai fields (action, channel, persona, preset, sdd-mode, components, skills) for the codegraph tool", i)
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
// selected skill names. It does not accept gentle-ai scalar/action fields,
// Claude plugin fields, or Codex MCP fields.
func validateSkillsSpec(s ProvisionerSpec, i int) error {
	if s.usesClaudeFields() {
		return fmt.Errorf("provisioners[%d].spec must not set claude fields (marketplace, plugin, from) for the skills tool", i)
	}
	if s.usesMCPFields() {
		return fmt.Errorf("provisioners[%d].spec must not set codex MCP fields (mcp, command, env) for the skills tool", i)
	}
	if strings.TrimSpace(s.Action) != "" ||
		strings.TrimSpace(s.Scope) != "" ||
		strings.TrimSpace(s.Channel) != "" ||
		strings.TrimSpace(s.Persona) != "" ||
		strings.TrimSpace(s.Preset) != "" ||
		strings.TrimSpace(s.SDDMode) != "" ||
		hasNonEmptyString(s.Components) ||
		s.Yes {
		return fmt.Errorf("provisioners[%d].spec must not set gentle-ai fields (action, scope, channel, persona, preset, sdd-mode, components, yes) for the skills tool", i)
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

// SharesTag reports whether an item's tags intersect a profile's tags. It is
// half the profile-scoping rule entries and provisioners share: an item belongs
// to a profile when at least one of its tags matches one of the profile's tags.
// Centralizing it here keeps plan, status, deps, doctor, and provision filtering
// through one rule instead of each carrying its own copy.
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
	case "", "json-subset", "toml-subset":
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
// generic command runner: gentle-ai, claude, codex, codegraph, and skills are the
// only accepted provisioner tools, each driven through a fixed set of subcommands.
func allowedProvisionerTool(tool string) bool {
	switch strings.TrimSpace(tool) {
	case "gentle-ai", "claude", "codex", "codegraph", "skills":
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
