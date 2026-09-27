package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/state"
)

type carbonfoxVersionRequirement struct {
	tool, name, minimum string
	pattern             *regexp.Regexp
	version             []int
}

var carbonfoxSemver = regexp.MustCompile(`(?:^|[^0-9])([0-9]+)\.([0-9]+)\.([0-9]+)(?:[^0-9]|$)`)
var carbonfoxWarpVersion = regexp.MustCompile(`(?:^|[^0-9])(0)\.([0-9]{4})\.([0-9]{2})\.([0-9]{2})\.([0-9]{2})\.([0-9]{2})\.stable_([0-9]+)(?:[^0-9]|$)`)

// These checks are limited to native loaders that otherwise silently ignore an
// unsupported custom theme. --skip-deps skips provisioning, not this proof.
func carbonfoxRequirements(p plan.Plan) []carbonfoxVersionRequirement {
	known := map[string]carbonfoxVersionRequirement{
		"configs/claude/settings-carbonfox.json": {"claude", "Claude Code", "2.1.220", carbonfoxSemver, []int{2, 1, 220}},
		"configs/tuicr/config-carbonfox.toml":    {"tuicr", "tuicr", "0.16.1", carbonfoxSemver, []int{0, 16, 1}},
		"configs/warp/settings-carbonfox.toml":   {"warp-terminal", "Warp", "v0.2026.06.03.09.49.stable_00", carbonfoxWarpVersion, []int{0, 2026, 6, 3, 9, 49, 0}},
	}
	var result []carbonfoxVersionRequirement
	seen := map[string]bool{}
	for _, action := range p.Actions {
		for _, source := range append([]string{action.Source}, action.Sources...) {
			if requirement, ok := known[source]; ok && !seen[requirement.tool] {
				seen[requirement.tool] = true
				result = append(result, requirement)
			}
		}
	}
	return result
}

func checkCarbonfoxVersions(requirements []carbonfoxVersionRequirement, probe func(string) (string, error)) error {
	for _, requirement := range requirements {
		value, err := probe(requirement.tool)
		if err != nil {
			return fmt.Errorf("Carbonfox requires %s >= %s; install or upgrade the application and retry: %w", requirement.name, requirement.minimum, err)
		}
		match := requirement.pattern.FindStringSubmatch(strings.TrimSpace(value))
		if len(match) != len(requirement.version)+1 {
			return fmt.Errorf("cannot verify %s native Carbonfox support; install %s or newer and retry", requirement.name, requirement.minimum)
		}
		comparison := 0
		for index, minimum := range requirement.version {
			part, err := strconv.Atoi(match[index+1])
			if err != nil {
				return fmt.Errorf("parse %s version: %w", requirement.name, err)
			}
			if comparison == 0 && part < minimum {
				comparison = -1
			} else if comparison == 0 && part > minimum {
				comparison = 1
			}
		}
		if comparison < 0 {
			return fmt.Errorf("Carbonfox requires %s >= %s; upgrade the application before applying this selection", requirement.name, requirement.minimum)
		}
	}
	return nil
}

func validateCarbonfoxNativeSupport(ctx context.Context, p plan.Plan, home, goos string, baseEnv []string) (resultErr error) {
	requirements := carbonfoxRequirements(p)
	if len(requirements) == 0 {
		return nil
	}
	if baseEnv == nil {
		baseEnv = os.Environ()
	}
	lookupEnv := envForProvisioner(baseEnv, home)
	probeHome, err := os.MkdirTemp("", "dots-carbonfox-probe-")
	if err != nil {
		return fmt.Errorf("create native theme probe directory: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(probeHome); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("remove native theme probe directory: %w", err))
		}
	}()
	env := carbonfoxProbeEnvironment(lookupEnv, probeHome)
	return checkCarbonfoxVersions(requirements, func(tool string) (string, error) {
		if tool == "warp-terminal" && goos == "darwin" {
			for _, directory := range appDirectories(goos, home) {
				plist := filepath.Join(directory, "Warp.app", "Contents", "Info.plist")
				if _, err := os.Stat(plist); errors.Is(err, os.ErrNotExist) {
					continue
				} else if err != nil {
					return "", fmt.Errorf("inspect Warp application version: %w", err)
				}
				cmd := exec.CommandContext(ctx, "/usr/bin/plutil", "-extract", "CFBundleShortVersionString", "raw", "-o", "-", plist)
				cmd.Env, cmd.Dir = env, probeHome
				output, err := cmd.Output()
				if err != nil {
					return "", fmt.Errorf("read Warp application version: %w", err)
				}
				return string(output), nil
			}
			return "", fmt.Errorf("Warp.app is missing from the selected home Applications directory and /Applications")
		}
		executable, ok := lookPathInEnvironment(tool, lookupEnv)
		if !ok {
			return "", fmt.Errorf("%s is missing from the selected environment PATH", tool)
		}
		cmd := exec.CommandContext(ctx, executable, "--version")
		cmd.Env, cmd.Dir = env, probeHome
		output, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("probe %s version: %w", tool, err)
		}
		return string(output), nil
	})
}

func carbonfoxProbeEnvironment(base []string, home string) []string {
	env := make([]string, 0, len(base)+8)
	for _, value := range base {
		if strings.HasPrefix(value, "HOME=") || strings.HasPrefix(value, "XDG_") || strings.HasPrefix(value, "CLAUDE_") || strings.HasPrefix(value, "DISABLE_AUTOUPDATER=") {
			continue
		}
		env = append(env, value)
	}
	return append(env, "HOME="+home,
		"XDG_CONFIG_HOME="+filepath.Join(home, ".config"),
		"XDG_DATA_HOME="+filepath.Join(home, ".local/share"),
		"XDG_CACHE_HOME="+filepath.Join(home, ".cache"),
		"XDG_STATE_HOME="+filepath.Join(home, ".local/state"),
		"CLAUDE_CONFIG_DIR="+filepath.Join(home, ".claude"),
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1", "DISABLE_AUTOUPDATER=1")
}

// Refresh commands retain removed Managed Entries. Preference transitions must
// use install's reconciliation and explicit Conflict Resolution instead.
func guardCarbonfoxRefreshPreference(previous *state.InstalledSelection, tags []string, dryRun bool) error {
	if dryRun || previous == nil {
		return nil
	}
	if slices.Contains(previous.ResolvedTags, "theme-carbonfox") == slices.Contains(tags, "theme-carbonfox") {
		return nil
	}
	return errors.New("changing the Carbonfox preference requires dots install with the complete desired selection; review the plan and use normal selection-change acknowledgement and Replace conflict decisions, then retry update or upgrade")
}
