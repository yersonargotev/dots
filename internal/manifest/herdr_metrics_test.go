package manifest_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/selectedsurface"
)

const (
	herdrMetricsTag    = "herdr-metrics"
	herdrStatusTarget  = "~/.config/herdr/status.sh"
	herdrStatusSource  = "configs/herdr/status.sh"
	herdrStatusPattern = `^CPU [0-9]+% GPU [0-9]+% RAM [0-9]+\.[0-9]GiB$`
)

func loadRepositoryManifest(t *testing.T) manifest.Manifest {
	t.Helper()
	m, err := manifest.LoadFile(filepath.Join(repositoryRoot(t), "dots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return *m
}

func TestRepositoryHerdrMetricsTagIsCurrentOptInSurface(t *testing.T) {
	m := loadRepositoryManifest(t)
	tag, ok := m.Tags[herdrMetricsTag]
	if !ok || tag.Kind != "surface" || tag.Status != "current" || len(tag.ReplacedBy) != 0 {
		t.Fatalf("Tag %q = %#v (declared %t), want current surface Tag", herdrMetricsTag, tag, ok)
	}
	for name, profile := range m.Profiles {
		resolved, _, err := manifest.NormalizeTags(m, profile.Tags)
		if err != nil {
			t.Fatalf("normalize Profile %q: %v", name, err)
		}
		if slices.Contains(resolved, herdrMetricsTag) {
			t.Errorf("Profile %q selects opt-in Tag %q", name, herdrMetricsTag)
		}
	}
}

func TestHerdrMetricsTagSelectsOptionalMacmonAndStatusScriptOnDarwin(t *testing.T) {
	m := loadRepositoryManifest(t)
	for _, arch := range []string{"arm64", "amd64"} {
		t.Run(arch, func(t *testing.T) {
			surface := selectedsurface.EvaluateForPlatform(m, []string{herdrMetricsTag}, "darwin", arch)
			if len(surface.Dependencies) != 1 {
				t.Fatalf("Dependencies = %#v, want only macmon", surface.Dependencies)
			}
			macmon := surface.Dependencies[0]
			if macmon.Name != "macmon" || macmon.Command != "macmon" || macmon.Brew != "macmon" || macmon.RequirementValue() != manifest.DependencyRequirementOptional {
				t.Fatalf("macmon Dependency = %#v, want optional Homebrew macmon", macmon)
			}
			if len(surface.Entries) != 1 {
				t.Fatalf("Entries = %#v, want only the status script", surface.Entries)
			}
			entry := surface.Entries[0]
			if entry.Entry.Target != herdrStatusTarget || entry.Source != herdrStatusSource || entry.Entry.Strategy != "copy" {
				t.Fatalf("status Managed Entry = %#v, want %s -> %s by copy", entry, herdrStatusSource, herdrStatusTarget)
			}
			if len(surface.Provisioners) != 0 {
				t.Fatalf("Provisioners = %#v, want none", surface.Provisioners)
			}
		})
	}
}

func TestHerdrMetricsTagSelectsNothingOnLinux(t *testing.T) {
	m := loadRepositoryManifest(t)
	surface := selectedsurface.EvaluateForPlatform(m, []string{herdrMetricsTag}, "linux", "amd64")
	if len(surface.Entries) != 0 || len(surface.Dependencies) != 0 || len(surface.Provisioners) != 0 {
		t.Fatalf("Linux %s surface = entries %#v, dependencies %#v, provisioners %#v; want empty", herdrMetricsTag, surface.Entries, surface.Dependencies, surface.Provisioners)
	}
}

func TestHerdrWithoutMetricsTagOmitsMacmonAndStatusScript(t *testing.T) {
	m := loadRepositoryManifest(t)
	selections := map[string][]string{"herdr": {"herdr"}}
	for _, name := range []string{"core", "workstation"} {
		resolved, _, err := manifest.NormalizeTags(m, m.Profiles[name].Tags)
		if err != nil {
			t.Fatal(err)
		}
		selections["profile "+name] = resolved
	}
	for name, tags := range selections {
		t.Run(name, func(t *testing.T) {
			surface := selectedsurface.EvaluateForPlatform(m, tags, "darwin", "arm64")
			if selectedDependencyNamed(surface, "macmon") {
				t.Fatalf("Selected Surface for %v plans macmon without %s", tags, herdrMetricsTag)
			}
			for _, entry := range surface.Entries {
				if entry.Entry.Target == herdrStatusTarget {
					t.Fatalf("Selected Surface for %v installs %s without %s", tags, herdrStatusTarget, herdrMetricsTag)
				}
			}
		})
	}

	withMetrics := selectedsurface.EvaluateForPlatform(m, []string{"herdr", herdrMetricsTag}, "darwin", "arm64")
	if !selectedDependencyNamed(withMetrics, "macmon") || !selectedDependencyNamed(withMetrics, "herdr") {
		t.Fatalf("herdr + %s Dependencies = %#v, want herdr and macmon", herdrMetricsTag, withMetrics.Dependencies)
	}
}

// macmonFixture mirrors the documented `macmon pipe` JSON shape.
const macmonFixture = `{
  "timestamp": "2025-02-24T20:38:15.427569+00:00",
  "temp": { "cpu_temp_avg": 43.73614, "gpu_temp_avg": 36.95167 },
  "memory": {
    "ram_total": 25769803776,
    "ram_usage": 20985479168,
    "swap_total": 4294967296,
    "swap_usage": 2602434560
  },
  "cpu_scaled_ratio": 0.036854,
  "cpu_active_ratio": 0.092,
  "ecpu_active_ratio": 0.18,
  "pcpu_active_ratio": 0.04,
  "ecpu_cores": [{ "die_id": 0, "core_id": 0, "freq_mhz": 1600, "scaled_ratio": 0.14, "active_ratio": 0.24 }],
  "gpu_freq_mhz": 461,
  "gpu_scaled_ratio": 0.021497859,
  "gpu_active_ratio": 0.5,
  "cpu_power": 0.20486385
}`

func TestHerdrStatusScriptFormatsMacmonSample(t *testing.T) {
	for _, tc := range []struct {
		name, payload, want string
	}{
		{"pretty sample", macmonFixture, "CPU 9% GPU 50% RAM 19.5GiB"},
		{"compact sample", strings.Join(strings.Fields(macmonFixture), ""), "CPU 9% GPU 50% RAM 19.5GiB"},
		{"idle", `{"memory":{"ram_usage":0},"cpu_active_ratio":0,"gpu_active_ratio":0.0}`, "CPU 0% GPU 0% RAM 0.0GiB"},
		{"saturated", `{"memory":{"ram_usage":68719476736},"cpu_active_ratio":1,"gpu_active_ratio":1.0}`, "CPU 100% GPU 100% RAM 64.0GiB"},
		{"exponent", `{"memory":{"ram_usage":1.073741824e9},"cpu_active_ratio":2.5e-1,"gpu_active_ratio":1e-3}`, "CPU 25% GPU 0% RAM 1.0GiB"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := runHerdrStatusScript(t, macmonStub(t, tc.payload, 0))
			if result.err != nil {
				t.Fatalf("status script error = %v, stdout %q, stderr %q", result.err, result.stdout, result.stderr)
			}
			if result.stdout != tc.want+"\n" {
				t.Fatalf("stdout = %q, want one line %q", result.stdout, tc.want)
			}
			if strings.ContainsRune(result.stdout, '\x1b') {
				t.Fatalf("stdout contains escape sequences: %q", result.stdout)
			}
		})
	}
}

func TestHerdrStatusScriptFailsQuietlyWithoutUsableSample(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload string
		exit    int
		missing bool
	}{
		{name: "missing macmon", missing: true},
		{name: "macmon failure", payload: macmonFixture, exit: 1},
		{name: "empty output"},
		{name: "not JSON", payload: "macmon: unsupported hardware"},
		{name: "missing CPU", payload: strings.Replace(macmonFixture, `"cpu_active_ratio"`, `"cpu_other"`, 1)},
		{name: "missing GPU", payload: strings.Replace(macmonFixture, `"gpu_active_ratio"`, `"gpu_other"`, 1)},
		{name: "missing RAM", payload: strings.Replace(macmonFixture, `"ram_usage"`, `"ram_other"`, 1)},
		{name: "string ratio", payload: strings.Replace(macmonFixture, `"cpu_active_ratio": 0.092`, `"cpu_active_ratio": "0.092"`, 1)},
		{name: "negative ratio", payload: strings.Replace(macmonFixture, `"gpu_active_ratio": 0.5`, `"gpu_active_ratio": -0.5`, 1)},
		{name: "null RAM", payload: strings.Replace(macmonFixture, `"ram_usage": 20985479168`, `"ram_usage": null`, 1)},
		{name: "truncated number", payload: `{"memory":{"ram_usage":1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bin := t.TempDir()
			if !tc.missing {
				bin = macmonStub(t, tc.payload, tc.exit)
			}
			result := runHerdrStatusScript(t, bin)
			var exitErr *exec.ExitError
			if !errors.As(result.err, &exitErr) || exitErr.ExitCode() == 0 {
				t.Fatalf("status script error = %v, want non-zero exit", result.err)
			}
			if result.stdout != "" {
				t.Fatalf("stdout = %q, want empty so Herdr clears the status entry", result.stdout)
			}
		})
	}
}

func TestHerdrStatusScriptLiveMacmonSample(t *testing.T) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skipf("macmon requires Apple Silicon macOS; running on %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	macmon, err := exec.LookPath("macmon")
	if err != nil {
		t.Skip("macmon is not installed")
	}
	result := runHerdrStatusScript(t, filepath.Dir(macmon))
	if result.err != nil {
		t.Fatalf("live status script error = %v, stderr %q", result.err, result.stderr)
	}
	if line := strings.TrimSuffix(result.stdout, "\n"); !regexp.MustCompile(herdrStatusPattern).MatchString(line) {
		t.Fatalf("live status = %q, want %s", result.stdout, herdrStatusPattern)
	}
}

type statusScriptResult struct {
	stdout, stderr string
	err            error
}

func macmonStub(t *testing.T, payload string, exit int) string {
	t.Helper()
	bin := t.TempDir()
	fixture := filepath.Join(t.TempDir(), "payload.json")
	if err := os.WriteFile(fixture, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n[ \"$*\" = 'pipe -s 1' ] || exit 64\ncat '" + fixture + "'\nexit " + strconv.Itoa(exit) + "\n"
	if err := os.WriteFile(filepath.Join(bin, "macmon"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return bin
}

func runHerdrStatusScript(t *testing.T, bin string) statusScriptResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", filepath.Join(repositoryRoot(t), herdrStatusSource))
	cmd.Dir = t.TempDir()
	cmd.Env = []string{"PATH=" + bin + ":/usr/bin:/bin", "HOME=" + t.TempDir()}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return statusScriptResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
}
