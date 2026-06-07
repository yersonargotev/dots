package install

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/state"
)

// Options carries resolved inputs needed to apply an Install Plan.
type Options struct {
	SourceRoot string
	Home       string
	// StateRoot is the directory where Installation Metadata is recorded so
	// dots status can later detect Drift. When empty, metadata is not written.
	StateRoot string
}

// Apply performs safe filesystem changes described by an Install Plan.
func Apply(p plan.Plan, opts Options) error {
	resolvedSources, err := validatePlan(p, opts)
	if err != nil {
		return err
	}

	for i, action := range p.Actions {
		switch action.Status {
		case plan.StatusUnchanged:
			continue
		case plan.StatusCreate:
			if err := applyCreate(action, resolvedSources[i]); err != nil {
				return err
			}
		}
	}

	return recordMetadata(p, resolvedSources, opts)
}

// recordMetadata upserts Installation Metadata for every managed target the plan
// installed or confirmed unchanged, so dots status has an authoritative record
// of what dots owns and what each target's Source of Truth hashed to.
func recordMetadata(p plan.Plan, resolvedSources []string, opts Options) error {
	if opts.StateRoot == "" {
		return nil
	}

	path := state.Path(opts.StateRoot)
	meta, err := state.Load(path)
	if err != nil {
		return err
	}
	meta.Version = 1

	now := time.Now().UTC().Format(time.RFC3339)
	for i, action := range p.Actions {
		if action.Status != plan.StatusCreate && action.Status != plan.StatusUnchanged {
			continue
		}
		hash, err := state.HashFile(resolvedSources[i])
		if err != nil {
			return err
		}
		upsertRecord(&meta, state.Record{
			Target:      action.Target,
			Source:      action.Source,
			Strategy:    action.Strategy,
			Hash:        hash,
			InstalledAt: now,
		})
	}

	return state.Save(path, meta)
}

func upsertRecord(meta *state.Metadata, rec state.Record) {
	for i := range meta.Entries {
		if meta.Entries[i].Target == rec.Target {
			meta.Entries[i] = rec
			return
		}
	}
	meta.Entries = append(meta.Entries, rec)
}

func validatePlan(p plan.Plan, opts Options) ([]string, error) {
	if opts.Home == "" {
		return nil, fmt.Errorf("install home is required")
	}
	if opts.SourceRoot == "" {
		return nil, fmt.Errorf("install source root is required")
	}
	home, err := cleanAbs(opts.Home)
	if err != nil {
		return nil, fmt.Errorf("resolve home: %w", err)
	}
	sourceRoot, err := cleanAbs(opts.SourceRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve source root: %w", err)
	}
	if err := validateStateRoot(opts.StateRoot, home); err != nil {
		return nil, err
	}

	seenTargets := map[string]struct{}{}
	resolvedSources := make([]string, len(p.Actions))
	for i, action := range p.Actions {
		if err := plan.ValidateResolvedTarget(action.Target, home); err != nil {
			return nil, err
		}
		targetKey, err := cleanAbs(action.Target)
		if err != nil {
			return nil, fmt.Errorf("resolve target %s: %w", action.Target, err)
		}
		if _, ok := seenTargets[targetKey]; ok {
			return nil, fmt.Errorf("install plan contains duplicate target %s", targetKey)
		}
		seenTargets[targetKey] = struct{}{}
		source, err := plan.ResolveSource(action.Source, sourceRoot)
		if err != nil {
			return nil, err
		}
		if action.ResolvedSource != "" && action.ResolvedSource != source {
			return nil, fmt.Errorf("install plan source %q resolved to %q, want %q", action.Source, action.ResolvedSource, source)
		}
		resolvedSources[i] = source

		switch action.Status {
		case plan.StatusCreate:
			if !supportedStrategy(action.Strategy) {
				return nil, fmt.Errorf("install strategy %q is not supported for %s", action.Strategy, action.Target)
			}
			if err := validateCreate(action, source, sourceRoot, home); err != nil {
				return nil, err
			}
		case plan.StatusUnchanged:
			continue
		case plan.StatusConflict, plan.StatusMissingSource:
			return nil, fmt.Errorf("install plan contains %s for %s", action.Status, action.Target)
		default:
			return nil, fmt.Errorf("install plan contains unsupported status %q for %s", action.Status, action.Target)
		}
	}
	return resolvedSources, nil
}

func validateStateRoot(stateRoot, home string) error {
	if stateRoot == "" {
		return nil
	}
	stateAbs, err := cleanAbs(stateRoot)
	if err != nil {
		return fmt.Errorf("resolve state root %s: %w", stateRoot, err)
	}
	homeAbs, err := cleanAbs(home)
	if err != nil {
		return fmt.Errorf("resolve home: %w", err)
	}
	// Explicit state roots outside home are trusted caller-controlled storage.
	// State roots inside home, including the CLI default, must not escape home
	// through symlinks because that would write default metadata outside the
	// sandbox selected by --home.
	if !plan.InsideRoot(stateAbs, homeAbs) {
		return nil
	}
	return validateStatePathInsideHome(stateAbs, homeAbs)
}

func validateStatePathInsideHome(stateAbs, homeAbs string) error {
	if err := plan.ValidatePathInsideHomeNoSymlinkEscape(stateAbs, homeAbs, "state root"); err != nil {
		return err
	}
	return plan.ValidateFilePathInsideHomeNoSymlinkEscape(state.Path(stateAbs), homeAbs, "installation metadata")
}

func cleanAbs(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func validateCreate(action plan.Action, source, sourceRoot, home string) error {
	if err := validateTargetStillAbsent(action.Target); err != nil {
		return err
	}
	if err := validateTargetParentInsideHome(action.Target, home); err != nil {
		return err
	}
	if err := validateSource(action.Strategy, source, sourceRoot); err != nil {
		return err
	}
	return nil
}

func validateTargetStillAbsent(target string) error {
	if _, err := os.Lstat(target); err == nil {
		return fmt.Errorf("install plan is stale: create target %s already exists", target)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat target %s: %w", target, err)
	}
	return nil
}

func validateTargetParentInsideHome(target, home string) error {
	return plan.ValidateTargetParentInsideHome(target, home)
}

func validateSource(strategy, source, sourceRoot string) error {
	if err := plan.ValidateResolvedSource(source, sourceRoot); err != nil {
		return err
	}

	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("stat source %s: %w", source, err)
	}
	if strategy == "copy" && !info.Mode().IsRegular() {
		return fmt.Errorf("source %s is not a regular file", source)
	}
	return nil
}

func supportedStrategy(strategy string) bool {
	switch strategy {
	case "symlink", "copy":
		return true
	default:
		return false
	}
}

func applyCreate(action plan.Action, source string) error {
	if err := os.MkdirAll(filepath.Dir(action.Target), 0o755); err != nil {
		return fmt.Errorf("create parent directory for %s: %w", action.Target, err)
	}

	switch action.Strategy {
	case "symlink":
		if err := os.Symlink(source, action.Target); err != nil {
			return fmt.Errorf("create symlink %s: %w", action.Target, err)
		}
		return nil
	case "copy":
		if err := copyRegularFile(source, action.Target); err != nil {
			return fmt.Errorf("copy %s to %s: %w", source, action.Target, err)
		}
		return nil
	default:
		return fmt.Errorf("install strategy %q is not supported for %s", action.Strategy, action.Target)
	}
}

func copyRegularFile(source, target string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source is not a regular file")
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(target)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(target)
		return err
	}
	return os.Chmod(target, info.Mode().Perm())
}
