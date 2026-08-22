package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSelectionFlagCompletionCoversEveryPublicCommand(t *testing.T) {
	manifestPath := writeSelectionCompletionManifest(t, t.TempDir())
	commands := [][]string{
		{"install"},
		{"plan"},
		{"status"},
		{"doctor"},
		{"deps", "check"},
		{"deps", "plan"},
		{"deps", "install"},
		{"update"},
		{"upgrade"},
	}

	for _, command := range commands {
		name := strings.Join(command, " ")
		t.Run(name+" profile", func(t *testing.T) {
			args := append(append([]string{}, command...), "--file", manifestPath, "--profile", "w")
			out, errOut := runCompletion(t, args...)
			assertCompletion(t, out, errOut, []string{"workstation\tComplete workstation preset"}, []string{"legacy"})
		})
		t.Run(name+" tag", func(t *testing.T) {
			args := append(append([]string{}, command...), "--file", manifestPath, "--tag", "z")
			out, errOut := runCompletion(t, args...)
			assertCompletion(t, out, errOut, []string{"zed\tZed editor", "zsh\tZ shell"}, []string{"legacy"})
		})
	}
}

func TestSelectionFlagCompletionUsesEffectiveSourceRoot(t *testing.T) {
	sourceRoot := t.TempDir()
	writeSelectionCompletionManifest(t, sourceRoot)

	out, errOut := runCompletion(t, "plan", "--source-root", sourceRoot, "--tag", "z")
	assertCompletion(t, out, errOut, []string{"zed\tZed editor", "zsh\tZ shell"}, []string{"core", "legacy"})
}

func TestSelectionFlagCompletionFailsQuietlyWithoutMutation(t *testing.T) {
	home := t.TempDir()
	sourceRoot := filepath.Join(home, "missing-repository")

	out, errOut := runCompletion(t, "install", "--home", home, "--source-root", sourceRoot, "--tag", "")
	if strings.TrimSpace(out) != ":4" {
		t.Fatalf("completion output = %q, want only NoFileComp directive", out)
	}
	if strings.Contains(errOut, "manifest") || strings.Contains(errOut, "repository") {
		t.Fatalf("completion printed ordinary error: %s", errOut)
	}
	if _, err := os.Stat(sourceRoot); !os.IsNotExist(err) {
		t.Fatalf("completion mutated source root: stat error = %v", err)
	}
}

func TestSelectionFlagCompletionFailsQuietlyForInvalidManifest(t *testing.T) {
	manifestPath := filepath.Join(t.TempDir(), "invalid.yaml")
	if err := os.WriteFile(manifestPath, []byte("version: ["), 0o600); err != nil {
		t.Fatalf("write invalid manifest: %v", err)
	}

	out, errOut := runCompletion(t, "plan", "--file", manifestPath, "--tag", "")
	if strings.TrimSpace(out) != ":4" {
		t.Fatalf("completion output = %q, want only NoFileComp directive", out)
	}
	if strings.Contains(errOut, "manifest") || strings.Contains(errOut, "yaml") {
		t.Fatalf("completion printed ordinary error: %s", errOut)
	}
}

func TestSelectionHelpIsConsistentAcrossPublicCommands(t *testing.T) {
	commands := [][]string{
		{"install"},
		{"plan"},
		{"status"},
		{"doctor"},
		{"deps", "check"},
		{"deps", "plan"},
		{"deps", "install"},
		{"update"},
		{"upgrade"},
	}

	root := NewRootCommand()
	for _, path := range commands {
		cmd, _, err := root.Find(path)
		if err != nil {
			t.Fatalf("find %q: %v", strings.Join(path, " "), err)
		}
		if flag := cmd.Flag("profile"); flag == nil || flag.Usage != selectionProfileHelp {
			t.Errorf("%s --profile help = %#v", strings.Join(path, " "), flag)
		}
		if flag := cmd.Flag("tag"); flag == nil || flag.Usage != selectionTagHelp {
			t.Errorf("%s --tag help = %#v", strings.Join(path, " "), flag)
		}
	}
}

func TestSelectionCommandDescriptionsDoNotRequireAProfile(t *testing.T) {
	root := NewRootCommand()
	commands := [][]string{{"plan"}, {"deps"}, {"deps", "plan"}}
	for _, path := range commands {
		cmd, _, err := root.Find(path)
		if err != nil {
			t.Fatalf("find %q: %v", strings.Join(path, " "), err)
		}
		if !strings.Contains(cmd.Long, "complete Profile/Tag selection") {
			t.Errorf("%s Long description does not describe complete Profile/Tag selection: %q", strings.Join(path, " "), cmd.Long)
		}
		if strings.Contains(strings.ToLower(cmd.Long), "a profile") {
			t.Errorf("%s Long description still requires a Profile: %q", strings.Join(path, " "), cmd.Long)
		}
	}
}

func runCompletion(t *testing.T, args ...string) (string, string) {
	t.Helper()
	root := NewRootCommand()
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(append([]string{"__complete"}, args...))
	if err := root.Execute(); err != nil {
		t.Fatalf("completion Execute() error = %v\nstderr:\n%s", err, errOut.String())
	}
	return out.String(), errOut.String()
}

func assertCompletion(t *testing.T, out, errOut string, want, avoid []string) {
	t.Helper()
	if !strings.HasSuffix(strings.TrimSpace(out), ":4") {
		t.Fatalf("completion output missing NoFileComp directive:\n%s", out)
	}
	for _, value := range want {
		if !strings.Contains(out, value+"\n") {
			t.Errorf("completion output missing %q:\n%s", value, out)
		}
	}
	for _, value := range avoid {
		if strings.Contains(out, value) {
			t.Errorf("completion output unexpectedly contains %q:\n%s", value, out)
		}
	}
	if strings.Contains(errOut, "Error:") {
		t.Errorf("completion printed ordinary error:\n%s", errOut)
	}
}

func writeSelectionCompletionManifest(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "dots.yaml")
	contents := `version: 1
tags:
  zsh:
    description: Z shell
    kind: surface
    status: current
  legacy:
    description: Legacy alias
    kind: compatibility
    status: legacy
    replaced_by: [zsh]
  zed:
    description: Zed editor
    kind: surface
    status: current
profiles:
  workstation:
    description: Complete workstation preset
    tags: [zsh, zed]
  legacy:
    description: Legacy preset
    status: legacy
    tags: [legacy]
entries:
  - source: zshrc
    target: ~/.zshrc
    strategy: copy
    tags: [zsh]
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write completion manifest: %v", err)
	}
	return path
}
