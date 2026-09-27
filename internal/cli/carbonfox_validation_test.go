package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/state"
)

func TestCarbonfoxNativeVersionBoundaries(t *testing.T) {
	for _, tt := range []struct {
		source, value string
		valid         bool
	}{
		{"configs/claude/settings-carbonfox.json", "2.1.220 (Claude Code)", true},
		{"configs/claude/settings-carbonfox.json", "2.1.219 (Claude Code)", false},
		{"configs/claude/settings-carbonfox.json", "3.0.0 (Claude Code)", true},
		{"configs/claude/settings-carbonfox.json", "unknown", false},
		{"configs/tuicr/config-carbonfox.toml", "tuicr 0.16.1", true},
		{"configs/tuicr/config-carbonfox.toml", "tuicr 0.16.0", false},
		{"configs/warp/settings-carbonfox.toml", "v0.2026.06.03.09.49.stable_00", true},
		{"configs/warp/settings-carbonfox.toml", "v0.2026.06.03.09.48.stable_00", false},
		{"configs/warp/settings-carbonfox.toml", "v0.2026.09.22.08.03.stable_01", true},
		{"configs/warp/settings-carbonfox.toml", "v0.2026.09.22.08.03.preview_01", false},
	} {
		t.Run(tt.source+tt.value, func(t *testing.T) {
			requirements := carbonfoxRequirements(plan.Plan{Actions: []plan.Action{{Source: tt.source}}})
			err := checkCarbonfoxVersions(requirements, func(string) (string, error) { return tt.value, nil })
			if (err == nil) != tt.valid {
				t.Fatalf("value %q: error = %v, valid = %v", tt.value, err, tt.valid)
			}
			if err != nil && !strings.Contains(err.Error(), "Carbonfox") {
				t.Fatalf("missing actionable theme diagnosis: %v", err)
			}
		})
	}
}

func TestCarbonfoxNativeProbesAreLimitedToSelectedSources(t *testing.T) {
	p := plan.Plan{Actions: []plan.Action{{Source: "configs/claude/settings.json"}, {Source: "configs/dots/theme-carbonfox"}}}
	if requirements := carbonfoxRequirements(p); len(requirements) != 0 {
		t.Fatalf("baseline/preference-alone requirements = %#v", requirements)
	}
	p.Actions = append(p.Actions, plan.Action{Source: "configs/claude/settings-carbonfox.json", Sources: []string{"configs/claude/settings-carbonfox.json"}})
	requirements := carbonfoxRequirements(p)
	if len(requirements) != 1 {
		t.Fatalf("duplicate native probes: %#v", requirements)
	}
	err := checkCarbonfoxVersions(requirements, func(string) (string, error) { return "", errors.New("binary absent") })
	if err == nil || !strings.Contains(err.Error(), "install or upgrade") || !strings.Contains(err.Error(), "binary absent") {
		t.Fatalf("missing actionable absent-native diagnosis: %v", err)
	}
}

func TestCarbonfoxVersionProbeIsolatesConfigurationAndUsesSelectedPATH(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
test "$1" = --version || exit 8
test "$CLAUDE_CONFIG_DIR" = "$HOME/.claude" || exit 9
test "$XDG_CONFIG_HOME" = "$HOME/.config" || exit 10
test "$CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC" = 1 || exit 11
test "$DISABLE_AUTOUPDATER" = 1 || exit 12
case "$HOME" in */dots-carbonfox-probe-*) ;; *) exit 13 ;; esac
printf '2.1.220 (Claude Code)\n'
`
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	p := plan.Plan{Actions: []plan.Action{{Source: "configs/claude/settings-carbonfox.json"}}}
	if err := validateCarbonfoxNativeSupport(context.Background(), p, home, "linux", []string{"PATH=/missing", "HOME=/outside", "CLAUDE_CONFIG_DIR=/outside", "XDG_CONFIG_HOME=/outside"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("version probe touched selected application config: %v", err)
	}
	if err := validateCarbonfoxNativeSupport(context.Background(), p, t.TempDir(), "linux", []string{"PATH=/missing"}); err == nil {
		t.Fatal("missing executable unexpectedly passed")
	}
}

func TestCarbonfoxWarpVersionUsesSelectedHomeApplication(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("native macOS plist reader")
	}
	home := t.TempDir()
	contents := filepath.Join(home, "Applications", "Warp.app", "Contents")
	if err := os.MkdirAll(contents, 0o700); err != nil {
		t.Fatal(err)
	}
	p := plan.Plan{Actions: []plan.Action{{Source: "configs/warp/settings-carbonfox.toml"}}}
	for _, tt := range []struct {
		version string
		valid   bool
	}{
		{"v0.2026.06.03.09.49.stable_00", true},
		{"v0.2026.05.20.08.00.stable_00", false},
		{"unknown", false},
	} {
		content := `<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict><key>CFBundleShortVersionString</key><string>` + tt.version + `</string></dict></plist>`
		if err := os.WriteFile(filepath.Join(contents, "Info.plist"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		err := validateCarbonfoxNativeSupport(context.Background(), p, home, "darwin", []string{"PATH=/missing"})
		if (err == nil) != tt.valid {
			t.Fatalf("version %s: error=%v", tt.version, err)
		}
	}
}

func TestCarbonfoxRefreshPreferenceRoutesChangesToInstall(t *testing.T) {
	for _, previous := range [][]string{{"ghostty"}, {"ghostty", "theme-carbonfox"}} {
		for _, desired := range [][]string{{"ghostty"}, {"ghostty", "theme-carbonfox"}} {
			recorded := &state.InstalledSelection{ResolvedTags: previous}
			err := guardCarbonfoxRefreshPreference(recorded, desired, false)
			changed := len(previous) != len(desired)
			if (err != nil) != changed {
				t.Fatalf("%v -> %v: %v", previous, desired, err)
			}
			if err != nil && !strings.Contains(err.Error(), "dots install with the complete desired selection") {
				t.Fatal(err)
			}
			if err := guardCarbonfoxRefreshPreference(recorded, desired, true); err != nil {
				t.Fatalf("dry-run must retain preview: %v", err)
			}
		}
	}
	if err := guardCarbonfoxRefreshPreference(nil, []string{"theme-carbonfox"}, false); err != nil {
		t.Fatal(err)
	}
}
