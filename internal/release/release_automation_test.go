package release_test

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

var supportedReleasePlatforms = []string{
	"darwin/amd64",
	"darwin/arm64",
	"linux/amd64",
	"linux/arm64",
}

func TestReleaseWorkflowPublishesBootstrapperConsumableArtifacts(t *testing.T) {
	root := repoRoot(t)
	workflow := readWorkflow(t, filepath.Join(root, ".github", "workflows", "release.yml"))

	triggers := mapAt(t, workflow, "on")
	push := mapAt(t, triggers, "push")
	tags := stringSliceAt(t, push, "tags")
	if !contains(tags, "v0.*") {
		t.Fatalf("release workflow should run for v0.x tags; got tags %v", tags)
	}
	if _, ok := triggers["workflow_dispatch"]; !ok {
		t.Fatalf("release workflow should support manual dispatch for the first v0.x release")
	}

	workflowText, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(workflowText)
	for _, want := range []string{
		"actions/checkout@v5",
		"actions/setup-go@v6",
		"scripts/build-release-artifacts.sh",
		"gh release upload",
		"checksums.txt",
		"scripts/generate-homebrew-formula.sh",
		"HOMEBREW_TAP_TOKEN",
		"yersonargotev/homebrew-tap",
		"Formula/dots.rb",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("release workflow should contain %q so GitHub Releases and the Homebrew tap stay in sync", want)
		}
	}
}

func TestReleaseWorkflowCreatesReleaseWithGeneratedNotes(t *testing.T) {
	root := repoRoot(t)
	workflow := readWorkflow(t, filepath.Join(root, ".github", "workflows", "release.yml"))
	steps := workflowSteps(t, workflow)

	createReleaseIndex := workflowStepIndex(t, steps, "GitHub Release creation with generated notes", func(step map[string]any) bool {
		return stepName(step) == "Create GitHub Release if needed"
	})
	run := stepRun(steps[createReleaseIndex])

	if !strings.Contains(run, "gh release create") {
		t.Fatalf("release creation step should create the GitHub Release; run script:\n%s", run)
	}
	if !strings.Contains(run, "--generate-notes") {
		t.Fatalf("release creation should ask GitHub to generate per-tag notes; run script:\n%s", run)
	}
	if strings.Contains(run, "--notes") {
		t.Fatalf("release creation should not pass static release notes; run script:\n%s", run)
	}
	if strings.Contains(run, "First usable v0.x release target") {
		t.Fatalf("release creation should not keep the old first-release notes body; run script:\n%s", run)
	}
}

func TestReleaseWorkflowProvesTapAccessBeforePublishingReleaseAssets(t *testing.T) {
	root := repoRoot(t)
	workflow := readWorkflow(t, filepath.Join(root, ".github", "workflows", "release.yml"))
	steps := workflowSteps(t, workflow)

	buildIndex := workflowStepIndex(t, steps, "build release artifacts and checksums", func(step map[string]any) bool {
		run := stepRun(step)
		return strings.Contains(run, "scripts/build-release-artifacts.sh") &&
			strings.Contains(run, "--out-dir dist")
	})
	requireTapTokenIndex := workflowStepIndex(t, steps, "required Homebrew tap token guard", func(step map[string]any) bool {
		run := stepRun(step)
		return strings.Contains(fmt.Sprint(stepEnv(step)["HOMEBREW_TAP_TOKEN"]), "HOMEBREW_TAP_TOKEN") &&
			strings.Contains(run, "HOMEBREW_TAP_TOKEN is required") &&
			strings.Contains(run, "yersonargotev/homebrew-tap")
	})
	tapCheckoutIndex := workflowStepIndex(t, steps, "token-backed Homebrew tap checkout", func(step map[string]any) bool {
		with := stepWith(step)
		return stepUses(step) == "actions/checkout@v5" &&
			with["repository"] == "yersonargotev/homebrew-tap" &&
			with["path"] == "homebrew-tap" &&
			strings.Contains(fmt.Sprint(with["token"]), "HOMEBREW_TAP_TOKEN")
	})
	formulaIndex := workflowStepIndex(t, steps, "Homebrew formula generation from release checksums", func(step map[string]any) bool {
		run := stepRun(step)
		return strings.Contains(run, "scripts/generate-homebrew-formula.sh") &&
			strings.Contains(run, "--checksums dist/checksums.txt") &&
			strings.Contains(run, "--out homebrew-tap/Formula/dots.rb")
	})
	prepareTapIndex := workflowStepIndex(t, steps, "local Homebrew tap formula commit preparation", func(step map[string]any) bool {
		run := stepRun(step)
		return stepWorkingDirectory(step) == "homebrew-tap" &&
			strings.Contains(run, `git config user.name "github-actions[bot]"`) &&
			strings.Contains(run, `git config user.email "github-actions[bot]@users.noreply.github.com"`) &&
			strings.Contains(run, "git add Formula/dots.rb") &&
			strings.Contains(run, "git diff --cached --quiet") &&
			strings.Contains(run, `echo "changed=false" >> "$GITHUB_OUTPUT"`) &&
			strings.Contains(run, `echo "changed=true" >> "$GITHUB_OUTPUT"`) &&
			strings.Contains(run, `git commit -m "feat: update dots formula to ${RELEASE_TAG}"`)
	})
	tapPushAccessProofIndex := workflowStepIndex(t, steps, "non-mutating Homebrew tap push permission proof against prepared local state", func(step map[string]any) bool {
		run := stepRun(step)
		return stepWorkingDirectory(step) == "homebrew-tap" &&
			strings.Contains(run, "git push --dry-run origin HEAD:main") &&
			!strings.Contains(run, "git commit") &&
			!strings.Contains(run, "git add Formula/dots.rb")
	})
	createReleaseIndex := workflowStepIndex(t, steps, "GitHub Release creation", func(step map[string]any) bool {
		run := stepRun(step)
		return strings.Contains(run, "gh release create") &&
			strings.Contains(fmt.Sprint(stepEnv(step)["GH_TOKEN"]), "github.token")
	})
	uploadIndex := workflowStepIndex(t, steps, "GitHub Release asset upload", func(step map[string]any) bool {
		run := stepRun(step)
		return strings.Contains(run, "gh release upload") &&
			strings.Contains(run, "dist/*") &&
			strings.Contains(run, "--clobber")
	})
	pushTapIndex := workflowStepIndex(t, steps, "final Homebrew tap formula push", func(step map[string]any) bool {
		run := stepRun(step)
		return stepWorkingDirectory(step) == "homebrew-tap" &&
			strings.Contains(fmt.Sprint(stepEnv(step)["TAP_UPDATE_CHANGED"]), "prepare_tap.outputs.changed") &&
			strings.Contains(run, `[[ "$TAP_UPDATE_CHANGED" != "true" ]]`) &&
			strings.Contains(run, "git push origin HEAD:main") &&
			!strings.Contains(run, "git push --dry-run") &&
			!strings.Contains(run, "git commit")
	})

	assertStepBefore(t, steps, buildIndex, formulaIndex, "formula generation must consume freshly built artifacts and dist/checksums.txt")
	assertStepBefore(t, steps, requireTapTokenIndex, tapCheckoutIndex, "the workflow must reject a missing HOMEBREW_TAP_TOKEN before falling back to anonymous tap checkout")
	assertStepBefore(t, steps, requireTapTokenIndex, createReleaseIndex, "a missing HOMEBREW_TAP_TOKEN must fail before creating a GitHub Release")
	assertStepBefore(t, steps, requireTapTokenIndex, uploadIndex, "a missing HOMEBREW_TAP_TOKEN must fail before re-uploading release assets")
	assertStepBefore(t, steps, tapCheckoutIndex, formulaIndex, "the tap checkout must exist before writing Formula/dots.rb into it")
	assertStepBefore(t, steps, formulaIndex, prepareTapIndex, "the generated formula must be staged before preparing a local tap commit")
	assertStepBefore(t, steps, prepareTapIndex, tapPushAccessProofIndex, "the workflow must dry-run push the already-prepared local tap state, not the untouched checkout")
	assertStepBefore(t, steps, tapPushAccessProofIndex, createReleaseIndex, "token-backed tap push permission must be proven before creating a GitHub Release")
	assertStepBefore(t, steps, tapPushAccessProofIndex, uploadIndex, "token-backed tap push permission must be proven before re-uploading release assets")
	assertStepBefore(t, steps, uploadIndex, pushTapIndex, "the tap update must not be published until release assets exist")
	assertStepBefore(t, steps, tapPushAccessProofIndex, pushTapIndex, "token-backed tap push permission must be proven before the mutating tap push")
	assertStepBefore(t, steps, prepareTapIndex, pushTapIndex, "the final tap push must publish the already-prepared commit instead of creating a new commit after assets upload")
}

func TestBuildReleaseArtifactsCreatesChecksummedSupportedPlatforms(t *testing.T) {
	if testing.Short() {
		t.Skip("cross-compiles release artifacts")
	}

	root := repoRoot(t)
	outDir := t.TempDir()
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "build-release-artifacts.sh"), "--version", "v0.99.0", "--out-dir", outDir)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "GOCACHE="+t.TempDir(), "GOMODCACHE="+goEnv(t, "GOMODCACHE"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build release artifacts: %v\n%s", err, output)
	}

	wantAssets := make([]string, 0, len(supportedReleasePlatforms))
	for _, platform := range supportedReleasePlatforms {
		parts := strings.Split(platform, "/")
		wantAssets = append(wantAssets, fmt.Sprintf("dots_v0.99.0_%s_%s", parts[0], parts[1]))
	}
	sort.Strings(wantAssets)

	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatal(err)
	}
	var gotAssets []string
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "checksums.txt" {
			continue
		}
		gotAssets = append(gotAssets, entry.Name())
	}
	sort.Strings(gotAssets)
	if strings.Join(gotAssets, "\n") != strings.Join(wantAssets, "\n") {
		t.Fatalf("release assets mismatch\nwant:\n%s\ngot:\n%s", strings.Join(wantAssets, "\n"), strings.Join(gotAssets, "\n"))
	}

	checksums := readChecksums(t, filepath.Join(outDir, "checksums.txt"))
	for _, asset := range wantAssets {
		gotChecksum, ok := checksums[asset]
		if !ok {
			t.Fatalf("checksums.txt missing checksum for %s", asset)
		}
		if gotChecksum != sha256File(t, filepath.Join(outDir, asset)) {
			t.Fatalf("checksum for %s does not match artifact bytes", asset)
		}
	}
	if len(checksums) != len(wantAssets) {
		t.Fatalf("checksums.txt should contain exactly release artifacts; got %d entries", len(checksums))
	}
}

func TestBuildReleaseArtifactsValidatesReleaseVersionLikeWorkflow(t *testing.T) {
	root := repoRoot(t)
	script := filepath.Join(root, "scripts", "build-release-artifacts.sh")

	t.Run("accepts v0 release tags", func(t *testing.T) {
		for _, version := range []string{"v0.1.0", "v0.2", "v0.1.0-rc.1"} {
			t.Run(version, func(t *testing.T) {
				fakeBin, logPath := fakeGoBuild(t)
				outDir := t.TempDir()
				cmd := exec.Command("bash", script, "--version", version, "--out-dir", outDir)
				cmd.Dir = root
				cmd.Env = append(os.Environ(), "PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

				output, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("build release artifacts should accept %s: %v\n%s", version, err, output)
				}
				if _, err := os.Stat(logPath); err != nil {
					t.Fatalf("expected accepted version %s to reach go build: %v", version, err)
				}
				log, err := os.ReadFile(logPath)
				if err != nil {
					t.Fatalf("read go build log: %v", err)
				}
				wantLdflag := "-X github.com/yersonargotev/dots/internal/version.Value=" + version
				if !strings.Contains(string(log), wantLdflag) {
					t.Fatalf("release build should inject version with ldflags %q\nlog:\n%s", wantLdflag, log)
				}
			})
		}
	})

	t.Run("rejects non-v0 and malformed versions before building", func(t *testing.T) {
		for _, version := range []string{"v1.0.0", "0.1.0", "main", ""} {
			t.Run(fmt.Sprintf("%q", version), func(t *testing.T) {
				fakeBin, logPath := fakeGoBuild(t)
				outDir := t.TempDir()
				cmd := exec.Command("bash", script, "--version", version, "--out-dir", outDir)
				cmd.Dir = root
				cmd.Env = append(os.Environ(), "PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

				output, err := cmd.CombinedOutput()
				if err == nil {
					t.Fatalf("build release artifacts should reject version %q\n%s", version, output)
				}
				if !strings.Contains(string(output), "Release version must be a v0.x tag") {
					t.Fatalf("rejection should explain v0.x requirement, got:\n%s", output)
				}
				if _, err := os.Stat(logPath); !os.IsNotExist(err) {
					t.Fatalf("invalid version %q should fail before go build; stat error: %v", version, err)
				}
			})
		}
	})
}

func TestGenerateHomebrewFormulaUsesChecksummedReleaseArtifacts(t *testing.T) {
	root := repoRoot(t)
	checksumsPath := filepath.Join(t.TempDir(), "checksums.txt")
	checksums := strings.Join([]string{
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  dots_v0.99.0_darwin_amd64",
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb  dots_v0.99.0_darwin_arm64",
		"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc  dots_v0.99.0_linux_amd64",
		"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd  dots_v0.99.0_linux_arm64",
	}, "\n") + "\n"
	if err := os.WriteFile(checksumsPath, []byte(checksums), 0o644); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(t.TempDir(), "Formula", "dots.rb")

	cmd := exec.Command(
		"bash",
		filepath.Join(root, "scripts", "generate-homebrew-formula.sh"),
		"--version", "v0.99.0",
		"--checksums", checksumsPath,
		"--out", outputPath,
		"--repo", "yersonargotev/dots",
		"--homepage", "https://github.com/yersonargotev/dots",
		"--desc", "Safe dotfiles installer",
	)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generate formula: %v\n%s", err, output)
	}

	formula, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(formula)
	for _, want := range []string{
		"class Dots < Formula",
		`desc "Safe dotfiles installer"`,
		`homepage "https://github.com/yersonargotev/dots"`,
		`version "0.99.0"`,
		`url "https://github.com/yersonargotev/dots/releases/download/v0.99.0/dots_v0.99.0_darwin_amd64", using: :nounzip`,
		`sha256 "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`,
		`url "https://github.com/yersonargotev/dots/releases/download/v0.99.0/dots_v0.99.0_darwin_arm64", using: :nounzip`,
		`sha256 "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"`,
		`url "https://github.com/yersonargotev/dots/releases/download/v0.99.0/dots_v0.99.0_linux_amd64", using: :nounzip`,
		`sha256 "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"`,
		`url "https://github.com/yersonargotev/dots/releases/download/v0.99.0/dots_v0.99.0_linux_arm64", using: :nounzip`,
		`sha256 "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"`,
		`bin.install downloaded_binary => "dots"`,
		`system "#{bin}/dots", "--version"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("formula should contain %q\nformula:\n%s", want, text)
		}
	}
	if got := strings.Count(text, "using: :nounzip"); got != len(supportedReleasePlatforms) {
		t.Fatalf("formula should mark every raw executable URL as using: :nounzip; got %d occurrences in:\n%s", got, text)
	}
}

func TestGenerateHomebrewFormulaFailsClearlyWhenChecksumEntryIsMissing(t *testing.T) {
	root := repoRoot(t)
	checksumsPath := filepath.Join(t.TempDir(), "checksums.txt")
	checksums := strings.Join([]string{
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  dots_v0.99.0_darwin_amd64",
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb  dots_v0.99.0_darwin_arm64",
		"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc  dots_v0.99.0_linux_amd64",
	}, "\n") + "\n"
	if err := os.WriteFile(checksumsPath, []byte(checksums), 0o644); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(t.TempDir(), "Formula", "dots.rb")

	cmd := exec.Command(
		"bash",
		filepath.Join(root, "scripts", "generate-homebrew-formula.sh"),
		"--version", "v0.99.0",
		"--checksums", checksumsPath,
		"--out", outputPath,
	)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("generate formula should fail when a checksum entry is missing\n%s", output)
	}
	if !strings.Contains(string(output), "missing checksum entry for dots_v0.99.0_linux_arm64") {
		t.Fatalf("failure should name the missing artifact, got:\n%s", output)
	}
	if _, err := os.Stat(outputPath); !os.IsNotExist(err) {
		t.Fatalf("formula should not be written with incomplete checksums; stat error: %v", err)
	}
}

func TestGenerateHomebrewFormulaFailsClearlyWhenChecksumManifestIsNotExact(t *testing.T) {
	root := repoRoot(t)

	baseChecksums := []string{
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  dots_v0.99.0_darwin_amd64",
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb  dots_v0.99.0_darwin_arm64",
		"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc  dots_v0.99.0_linux_amd64",
		"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd  dots_v0.99.0_linux_arm64",
	}

	tests := []struct {
		name      string
		extraLine string
		wantError string
	}{
		{
			name:      "rejects unexpected release artifact",
			extraLine: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee  dots_v0.99.0_linux_386",
			wantError: "unexpected checksum entry for dots_v0.99.0_linux_386",
		},
		{
			name:      "rejects duplicate expected artifact",
			extraLine: "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff  dots_v0.99.0_darwin_amd64",
			wantError: "duplicate checksum entry for dots_v0.99.0_darwin_amd64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checksumsPath := filepath.Join(t.TempDir(), "checksums.txt")
			checksums := strings.Join(append(baseChecksums, tt.extraLine), "\n") + "\n"
			if err := os.WriteFile(checksumsPath, []byte(checksums), 0o644); err != nil {
				t.Fatal(err)
			}
			outputPath := filepath.Join(t.TempDir(), "Formula", "dots.rb")

			cmd := exec.Command(
				"bash",
				filepath.Join(root, "scripts", "generate-homebrew-formula.sh"),
				"--version", "v0.99.0",
				"--checksums", checksumsPath,
				"--out", outputPath,
			)
			cmd.Dir = root
			output, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("generate formula should fail when the checksum manifest is not exact\n%s", output)
			}
			if !strings.Contains(string(output), tt.wantError) {
				t.Fatalf("failure should explain the manifest mismatch, got:\n%s", output)
			}
			if _, err := os.Stat(outputPath); !os.IsNotExist(err) {
				t.Fatalf("formula should not be written with invalid checksum manifest; stat error: %v", err)
			}
		})
	}
}

func fakeGoBuild(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "go-build.log")
	goPath := filepath.Join(dir, "go")
	script := fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail
echo "$*" >> %q
out=""
while [[ $# -gt 0 ]]; do
  if [[ "$1" == "-o" ]]; then
    out="${2:-}"
    break
  fi
  shift
done
if [[ -n "$out" ]]; then
  mkdir -p "$(dirname "$out")"
  printf 'fake binary\n' > "$out"
fi
`, logPath)
	if err := os.WriteFile(goPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir, logPath
}

func goEnv(t *testing.T, key string) string {
	t.Helper()
	cmd := exec.Command("go", "env", key)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("go env %s: %v", key, err)
	}
	return strings.TrimSpace(string(output))
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func readWorkflow(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var workflow map[string]any
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatalf("parse workflow YAML: %v", err)
	}
	return workflow
}

func workflowSteps(t *testing.T, workflow map[string]any) []map[string]any {
	t.Helper()
	jobs := mapAt(t, workflow, "jobs")
	release := mapAt(t, jobs, "release")
	value, ok := release["steps"]
	if !ok {
		t.Fatal("release job should define steps")
	}
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("release steps should be a list, got %T", value)
	}

	steps := make([]map[string]any, 0, len(items))
	for index, item := range items {
		step, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("release step %d should be a map, got %T", index, item)
		}
		steps = append(steps, step)
	}
	return steps
}

func workflowStepIndex(t *testing.T, steps []map[string]any, description string, match func(map[string]any) bool) int {
	t.Helper()
	for index, step := range steps {
		if match(step) {
			return index
		}
	}
	t.Fatalf("release workflow missing step for %s", description)
	return -1
}

func assertStepBefore(t *testing.T, steps []map[string]any, before, after int, reason string) {
	t.Helper()
	if before >= after {
		t.Fatalf("%s: step %q should appear before %q", reason, stepName(steps[before]), stepName(steps[after]))
	}
}

func stepName(step map[string]any) string {
	if name, ok := step["name"].(string); ok {
		return name
	}
	return "<unnamed>"
}

func stepRun(step map[string]any) string {
	if run, ok := step["run"].(string); ok {
		return run
	}
	return ""
}

func stepUses(step map[string]any) string {
	if uses, ok := step["uses"].(string); ok {
		return uses
	}
	return ""
}

func stepWith(step map[string]any) map[string]any {
	with, ok := step["with"].(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return with
}

func stepEnv(step map[string]any) map[string]any {
	env, ok := step["env"].(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return env
}

func stepWorkingDirectory(step map[string]any) string {
	if dir, ok := step["working-directory"].(string); ok {
		return dir
	}
	return ""
}

func mapAt(t *testing.T, m map[string]any, key string) map[string]any {
	t.Helper()
	value, ok := m[key]
	if !ok {
		t.Fatalf("missing %q", key)
	}
	nested, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%q should be a map, got %T", key, value)
	}
	return nested
}

func stringSliceAt(t *testing.T, m map[string]any, key string) []string {
	t.Helper()
	value, ok := m[key]
	if !ok {
		t.Fatalf("missing %q", key)
	}
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("%q should be a list, got %T", key, value)
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			t.Fatalf("%q item should be a string, got %T", key, item)
		}
		out = append(out, text)
	}
	return out
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func readChecksums(t *testing.T, path string) map[string]string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	checksums := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			t.Fatalf("checksum line should be '<sha256>  <asset>', got %q", scanner.Text())
		}
		if len(fields[0]) != sha256.Size*2 {
			t.Fatalf("checksum for %s should be SHA-256 hex, got %q", fields[1], fields[0])
		}
		if strings.Contains(fields[1], string(os.PathSeparator)) {
			t.Fatalf("checksum entry should use asset filename only, got %q", fields[1])
		}
		checksums[fields[1]] = fields[0]
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return checksums
}

func sha256File(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
