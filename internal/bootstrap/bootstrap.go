package bootstrap

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const DefaultRepositoryURL = "https://github.com/yersonargotev/dots.git"

type Options struct {
	SourceRoot    string
	RepositoryURL string
	RepositoryRef string
}

type Result struct {
	SourceRoot string `json:"source_root"`
	Cloned     bool   `json:"cloned"`
}

func Ensure(opts Options) (Result, error) {
	if strings.TrimSpace(opts.SourceRoot) == "" {
		return Result{}, errors.New("source root is required")
	}
	if strings.TrimSpace(opts.RepositoryURL) == "" {
		opts.RepositoryURL = DefaultRepositoryURL
	}

	result := Result{SourceRoot: opts.SourceRoot}
	if validSourceRoot(opts.SourceRoot) {
		return result, nil
	}

	info, err := os.Stat(opts.SourceRoot)
	switch {
	case err == nil && !info.IsDir():
		return Result{}, fmt.Errorf("Installed Repository path exists but is not a directory: %s", opts.SourceRoot)
	case err == nil:
		empty, err := dirEmpty(opts.SourceRoot)
		if err != nil {
			return Result{}, err
		}
		if !empty {
			return Result{}, fmt.Errorf("Installed Repository path exists but does not contain a valid dots.yaml: %s. Move it aside or pass --source-root", opts.SourceRoot)
		}
		if err := os.Remove(opts.SourceRoot); err != nil {
			return Result{}, fmt.Errorf("remove empty Installed Repository directory: %w", err)
		}
	case !os.IsNotExist(err):
		return Result{}, fmt.Errorf("inspect Installed Repository: %w", err)
	}

	if _, err := exec.LookPath("git"); err != nil {
		return Result{}, fmt.Errorf("git is required to clone the Source of Truth into %s", opts.SourceRoot)
	}
	if err := clone(opts); err != nil {
		return Result{}, err
	}
	result.Cloned = true
	return result, nil
}

// RequireCurrentRef verifies a valid Installed Repository already matches the
// requested release ref without mutating it. It lets dry-run commands fail before
// reading a stale manifest while preserving dry-run's no-write contract.
func RequireCurrentRef(opts Options) error {
	ref := strings.TrimSpace(opts.RepositoryRef)
	if ref == "" || !validSourceRoot(opts.SourceRoot) {
		return nil
	}
	matches, err := repositoryRefMatches(opts.SourceRoot, ref, fmt.Sprintf("cannot verify it is at %s", ref))
	if err != nil {
		return err
	}
	if !matches {
		return fmt.Errorf("Installed Repository at %s is not at %s; rerun without --dry-run to update it, or pass --source-root", opts.SourceRoot, ref)
	}
	return nil
}

func repositoryRefMatches(sourceRoot, ref, missingGitReason string) (bool, error) {
	if _, err := os.Stat(filepath.Join(sourceRoot, ".git")); err != nil {
		if os.IsNotExist(err) {
			return false, fmt.Errorf("Installed Repository at %s is not a git checkout; %s. Move it aside or pass --source-root", sourceRoot, missingGitReason)
		}
		return false, fmt.Errorf("inspect Installed Repository git metadata: %w", err)
	}
	return repositoryAtRef(sourceRoot, ref)
}

func repositoryAtRef(sourceRoot, ref string) (bool, error) {
	head, err := gitOutput(sourceRoot, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return false, fmt.Errorf("inspect Installed Repository HEAD: %w", err)
	}
	target, err := gitOutput(sourceRoot, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return false, nil
	}
	return strings.TrimSpace(head) == strings.TrimSpace(target), nil
}

func gitOutput(sourceRoot string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", sourceRoot}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s failed: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func validSourceRoot(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "dots.yaml"))
	return err == nil && !info.IsDir() && info.Size() > 0
}

func dirEmpty(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, fmt.Errorf("read Installed Repository directory: %w", err)
	}
	return len(entries) == 0, nil
}

func clone(opts Options) error {
	parent := filepath.Dir(opts.SourceRoot)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create Installed Repository parent: %w", err)
	}
	tmp, err := os.MkdirTemp(parent, ".dots-clone.*")
	if err != nil {
		return fmt.Errorf("create temporary clone directory: %w", err)
	}
	defer os.RemoveAll(tmp)

	args := []string{"clone", "--depth", "1"}
	if strings.TrimSpace(opts.RepositoryRef) != "" {
		args = append(args, "--branch", opts.RepositoryRef)
	}
	args = append(args, opts.RepositoryURL, tmp)
	cmd := exec.Command("git", args...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("clone Source of Truth: %w\n%s", err, strings.TrimSpace(string(output)))
	}
	if !validSourceRoot(tmp) {
		return errors.New("cloned Source of Truth does not contain a valid dots.yaml at repository root")
	}
	if err := os.Rename(tmp, opts.SourceRoot); err != nil {
		return fmt.Errorf("install cloned Source of Truth: %w", err)
	}
	return nil
}
