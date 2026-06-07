// Package doctor composes dots' read-only diagnostics into one report. It is a
// guardrail: especially for Secret Scan, findings catch obvious risks but never
// prove the repository is safe.
package doctor

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/yersonargotev/dots/internal/deps"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/state"
	"github.com/yersonargotev/dots/internal/status"
)

// Options carries the resolved inputs needed to build read-only diagnostics.
type Options struct {
	Profile    string
	OS         string
	SourceRoot string
	Home       string
}

// Report is the consolidated doctor diagnostic output for a profile.
type Report struct {
	Profile       string
	OS            string
	Platform      Platform
	Dependencies  deps.CheckReport
	Configuration status.Report
	SecretScan    SecretReport
}

// Platform reports whether the current platform is supported by dots v1.
type Platform struct {
	Supported bool
	OS        string
}

// SecretReport is the repository-managed configuration Secret Scan result.
type SecretReport struct {
	Findings []SecretFinding
}

// SecretFinding identifies one obvious credential/private-key pattern in a
// managed source file.
type SecretFinding struct {
	Source  string
	Line    int
	Pattern string
}

type secretPattern struct {
	name string
	re   *regexp.Regexp
}

var secretPatterns = []secretPattern{
	{name: "private-key", re: regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----`)},
	{name: "aws-access-key-id", re: regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`)},
	{name: "credential-assignment", re: regexp.MustCompile(`(?i)\b(?:password|passwd|secret|token|api[_-]?key|access[_-]?key)\b\s*[:=]\s*['"]?[^'"\s#]+`)},
}

// Build runs all doctor diagnostics without mutating the filesystem.
func Build(m manifest.Manifest, meta state.Metadata, opts Options, look deps.Lookup) (Report, error) {
	report := Report{
		Profile:  opts.Profile,
		OS:       opts.OS,
		Platform: Platform{Supported: supportedOS(opts.OS), OS: opts.OS},
	}

	depReport, err := deps.Check(m, deps.Options{Profile: opts.Profile, OS: opts.OS}, look)
	if err != nil {
		return Report{}, err
	}
	report.Dependencies = depReport

	statusReport, err := status.Build(m, meta, status.Options{
		Profile:    opts.Profile,
		OS:         opts.OS,
		SourceRoot: opts.SourceRoot,
		Home:       opts.Home,
	})
	if err != nil {
		return Report{}, err
	}
	report.Configuration = statusReport

	secretReport, err := ScanSecrets(m, opts)
	if err != nil {
		return Report{}, err
	}
	report.SecretScan = secretReport

	return report, nil
}

// ScanSecrets scans selected repository-managed source files for obvious secret
// patterns. It is intentionally conservative scope-wise and advisory in meaning.
func ScanSecrets(m manifest.Manifest, opts Options) (SecretReport, error) {
	profile, ok := m.Profiles[opts.Profile]
	if !ok {
		return SecretReport{}, fmt.Errorf("profile %q not found", opts.Profile)
	}

	var report SecretReport
	seen := map[string]bool{}
	for _, entry := range m.Entries {
		if !sharesTag(entry.Tags, profile.Tags) || !matchesOS(entry.OS, opts.OS) || seen[entry.Source] {
			continue
		}
		seen[entry.Source] = true

		sourceAbs, err := plan.ResolveSource(entry.Source, opts.SourceRoot)
		if err != nil {
			return SecretReport{}, err
		}
		if err := plan.ValidateResolvedSource(sourceAbs, opts.SourceRoot); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return SecretReport{}, err
		}

		findings, err := scanSource(entry.Source, sourceAbs)
		if err != nil {
			return SecretReport{}, err
		}
		report.Findings = append(report.Findings, findings...)
	}
	return report, nil
}

func scanSource(source, path string) ([]SecretFinding, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read secret scan source %s: %w", path, err)
	}
	defer file.Close()

	var findings []SecretFinding
	scanner := bufio.NewScanner(file)
	for line := 1; scanner.Scan(); line++ {
		text := scanner.Text()
		for _, pattern := range secretPatterns {
			if pattern.re.MatchString(text) && !looksLikePlaceholder(text) {
				findings = append(findings, SecretFinding{Source: source, Line: line, Pattern: pattern.name})
				break
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan source %s: %w", path, err)
	}
	return findings, nil
}

func supportedOS(osName string) bool {
	return osName == "darwin" || osName == "linux"
}

func looksLikePlaceholder(text string) bool {
	lower := strings.ToLower(text)
	for _, marker := range []string{"example", "placeholder", "replace_me", "changeme", "your_", "dummy"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func sharesTag(entryTags, profileTags []string) bool {
	for _, et := range entryTags {
		for _, pt := range profileTags {
			if et == pt {
				return true
			}
		}
	}
	return false
}

func matchesOS(entryOS []string, current string) bool {
	if len(entryOS) == 0 {
		return true
	}
	for _, osName := range entryOS {
		if strings.EqualFold(osName, current) {
			return true
		}
	}
	return false
}
