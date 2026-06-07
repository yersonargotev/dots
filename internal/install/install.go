package install

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yersonargotev/dots/internal/plan"
)

// Options carries resolved inputs needed to apply an Install Plan.
type Options struct {
	SourceRoot string
	Home       string
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
	return nil
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

	resolvedSources := make([]string, len(p.Actions))
	for i, action := range p.Actions {
		if err := plan.ValidateResolvedTarget(action.Target, home); err != nil {
			return nil, err
		}
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
	homeReal, err := filepath.EvalSymlinks(home)
	if err != nil {
		return fmt.Errorf("resolve real home %s: %w", home, err)
	}
	homeReal = filepath.Clean(homeReal)

	targetAbs, err := cleanAbs(target)
	if err != nil {
		return fmt.Errorf("resolve target %s: %w", target, err)
	}
	parent := filepath.Dir(targetAbs)
	rel, err := filepath.Rel(home, parent)
	if err != nil {
		return fmt.Errorf("resolve target parent %s relative to home %s: %w", parent, home, err)
	}
	if rel == "." {
		return nil
	}
	if rel == ".." || filepath.IsAbs(rel) || len(rel) >= 3 && rel[:3] == ".."+string(filepath.Separator) {
		return fmt.Errorf("unsafe target parent %q: resolved parent escapes home %q", parent, home)
	}

	current := home
	for _, part := range splitPath(rel) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("stat target parent %s: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink == 0 && !info.IsDir() {
			return fmt.Errorf("target parent %s is not a directory", current)
		}
		realParent, err := filepath.EvalSymlinks(current)
		if err != nil {
			return fmt.Errorf("resolve target parent %s: %w", current, err)
		}
		if !plan.InsideRoot(realParent, homeReal) {
			return fmt.Errorf("unsafe target parent %q: symlink resolves outside home %q", current, home)
		}
	}
	return nil
}

func splitPath(path string) []string {
	parts := []string{}
	separator := string(filepath.Separator)
	for _, part := range strings.Split(path, separator) {
		if part != "" && part != "." {
			parts = append(parts, part)
		}
	}
	return parts
}

func validateSource(strategy, source, sourceRoot string) error {
	rootReal, err := filepath.EvalSymlinks(sourceRoot)
	if err != nil {
		return fmt.Errorf("resolve real source root %s: %w", sourceRoot, err)
	}
	rootReal = filepath.Clean(rootReal)

	sourceReal, err := filepath.EvalSymlinks(source)
	if err != nil {
		return fmt.Errorf("resolve source %s: %w", source, err)
	}
	sourceReal = filepath.Clean(sourceReal)
	if !plan.InsideRoot(sourceReal, rootReal) {
		return fmt.Errorf("unsafe source %q: symlink resolves outside source root %q", source, sourceRoot)
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
