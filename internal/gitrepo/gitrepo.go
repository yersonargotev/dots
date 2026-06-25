// Package gitrepo is a thin wrapper around the system git binary used to update
// the Installed Repository before re-running the install flow. It deliberately
// avoids a heavy in-process Git library: dots already shells out to git in its
// bootstrapper, and a fast-forward-only update needs only a handful of plumbing
// commands.
package gitrepo

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ErrNotFastForward reports that the Installed Repository cannot be advanced by
// a clean fast-forward because the local branch has diverged from its upstream.
// dots refuses to merge or rebase automatically so it never rewrites local work.
var ErrNotFastForward = errors.New("installed repository cannot be fast-forwarded")

// Update describes the fast-forward an update did apply (FastForward) or would
// apply (Preview), so callers can report exactly what changed.
type Update struct {
	OldRev           string   `json:"old_rev"`
	NewRev           string   `json:"new_rev"`
	Incoming         []string `json:"incoming"`
	PreservedChanges string   `json:"preserved_changes,omitempty"`
	AttachedBranch   string   `json:"attached_branch,omitempty"`
}

type upstreamTarget struct {
	Ref             string
	Rev             string
	AttachBranch    string
	NeedsAttachment bool
}

// Changed reports whether the update moves (or would move) HEAD.
func (u Update) Changed() bool {
	return u.OldRev != u.NewRev
}

// IsRepo reports whether dir is inside a Git work tree.
func IsRepo(dir string) bool {
	out, err := run(dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

// IsClean reports whether dir has no uncommitted changes. Any porcelain output,
// including untracked files, is treated as dirty so an update never silently
// overwrites local state.
func IsClean(dir string) (bool, error) {
	out, err := run(dir, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "", nil
}

// PreserveLocalChanges moves any uncommitted Installed Repository changes into
// Git's stash and leaves the work tree clean for a safe fast-forward. The stash
// is intentionally not re-applied automatically: the Installed Repository is the
// Source of Truth clone, so local edits are preserved for inspection without
// blocking customers from receiving the latest reviewed manifest.
func PreserveLocalChanges(dir string) (string, bool, error) {
	clean, err := IsClean(dir)
	if err != nil {
		return "", false, err
	}
	if clean {
		return "", false, nil
	}
	if _, err := run(dir, "stash", "push", "--include-untracked", "-m", "dots update preserved local Installed Repository changes"); err != nil {
		return "", false, fmt.Errorf("preserve local changes for %s: %w", dir, err)
	}
	return "stash@{0}", true, nil
}

// Preview fetches remote refs and reports the fast-forward an update would apply
// without modifying the working tree. It returns ErrNotFastForward when the
// local branch has diverged from upstream.
func Preview(dir string) (Update, error) {
	old, err := head(dir)
	if err != nil {
		return Update{}, err
	}
	if err := fetch(dir); err != nil {
		return Update{}, err
	}
	upstream, err := resolveUpstream(dir)
	if err != nil {
		return Update{}, err
	}
	if old == upstream.Rev {
		return Update{OldRev: old, NewRev: old, AttachedBranch: upstream.AttachBranch}, nil
	}
	if !canFastForwardTo(dir, upstream.Ref) {
		return Update{}, ErrNotFastForward
	}
	incoming, err := incomingCommitsFrom(dir, upstream.Ref)
	if err != nil {
		return Update{}, err
	}
	return Update{OldRev: old, NewRev: upstream.Rev, Incoming: incoming, AttachedBranch: upstream.AttachBranch}, nil
}

// FastForward fetches and advances dir to its upstream via merge --ff-only. It
// returns ErrNotFastForward when the local branch cannot be fast-forwarded,
// leaving the working tree untouched.
func FastForward(dir string) (Update, error) {
	old, err := head(dir)
	if err != nil {
		return Update{}, err
	}
	if err := fetch(dir); err != nil {
		return Update{}, err
	}
	upstream, err := resolveUpstream(dir)
	if err != nil {
		return Update{}, err
	}
	if old == upstream.Rev {
		return Update{OldRev: old, NewRev: old, AttachedBranch: upstream.AttachBranch}, nil
	}
	if upstream.NeedsAttachment && !canAttachBranch(dir, upstream.AttachBranch, upstream.Ref) {
		return Update{}, ErrNotFastForward
	}
	// Capture the incoming commits before merging so the report reflects exactly
	// what the fast-forward applied.
	incoming, err := incomingCommitsFrom(dir, upstream.Ref)
	if err != nil {
		return Update{}, err
	}
	if upstream.NeedsAttachment {
		if err := attachBranch(dir, upstream.AttachBranch); err != nil {
			return Update{}, err
		}
	}
	if _, err := run(dir, "merge", "--ff-only", upstream.Ref); err != nil {
		return Update{}, ErrNotFastForward
	}
	newRev, err := head(dir)
	if err != nil {
		return Update{}, err
	}
	return Update{OldRev: old, NewRev: newRev, Incoming: incoming, AttachedBranch: upstream.AttachBranch}, nil
}

// FastForwardPreservingLocalChanges verifies that dir can fast-forward before
// moving local changes out of the work tree. This preserves the fast-forward-only
// safety invariant: when the branch has diverged, dots returns ErrNotFastForward
// without mutating the user's Installed Repository state.
func FastForwardPreservingLocalChanges(dir string) (Update, error) {
	old, err := head(dir)
	if err != nil {
		return Update{}, err
	}
	if err := fetch(dir); err != nil {
		return Update{}, err
	}
	upstream, err := resolveUpstream(dir)
	if err != nil {
		return Update{}, err
	}
	if old != upstream.Rev && !canFastForwardTo(dir, upstream.Ref) {
		return Update{}, ErrNotFastForward
	}

	var incoming []string
	if old != upstream.Rev {
		incoming, err = incomingCommitsFrom(dir, upstream.Ref)
		if err != nil {
			return Update{}, err
		}
	}
	if upstream.NeedsAttachment && !canAttachBranch(dir, upstream.AttachBranch, upstream.Ref) {
		return Update{}, ErrNotFastForward
	}

	preserved, stashed, err := PreserveLocalChanges(dir)
	if err != nil {
		return Update{}, err
	}
	upd := Update{OldRev: old, NewRev: old, Incoming: incoming, AttachedBranch: upstream.AttachBranch}
	if stashed {
		upd.PreservedChanges = preserved
	}
	if upstream.NeedsAttachment {
		if err := attachBranch(dir, upstream.AttachBranch); err != nil {
			return Update{}, err
		}
	}
	if old == upstream.Rev {
		return upd, nil
	}

	if _, err := run(dir, "merge", "--ff-only", upstream.Ref); err != nil {
		return Update{}, ErrNotFastForward
	}
	newRev, err := head(dir)
	if err != nil {
		return Update{}, err
	}
	upd.NewRev = newRev
	return upd, nil
}

func head(dir string) (string, error) {
	out, err := run(dir, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func resolveUpstream(dir string) (upstreamTarget, error) {
	ref, err := run(dir, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	if err == nil {
		ref = strings.TrimSpace(ref)
		rev, err := rev(dir, ref)
		if err != nil {
			return upstreamTarget{}, err
		}
		return upstreamTarget{Ref: ref, Rev: rev}, nil
	}

	branch, branchErr := run(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if branchErr != nil || strings.TrimSpace(branch) != "HEAD" {
		return upstreamTarget{}, fmt.Errorf("resolve upstream for %s: %w", dir, err)
	}

	ref, attachBranch, fallbackErr := defaultRemoteBranch(dir)
	if fallbackErr != nil {
		if fetchErr := fetchOriginBranches(dir); fetchErr != nil {
			return upstreamTarget{}, fmt.Errorf("resolve upstream for detached Installed Repository %s: %w", dir, fetchErr)
		}
		ref, attachBranch, fallbackErr = defaultRemoteBranch(dir)
		if fallbackErr != nil {
			return upstreamTarget{}, fmt.Errorf("resolve upstream for detached Installed Repository %s: %w", dir, fallbackErr)
		}
	}
	rev, err := rev(dir, ref)
	if err != nil {
		return upstreamTarget{}, err
	}
	return upstreamTarget{Ref: ref, Rev: rev, AttachBranch: attachBranch, NeedsAttachment: true}, nil
}

func rev(dir, ref string) (string, error) {
	out, err := run(dir, "rev-parse", "--short", ref)
	if err != nil {
		return "", fmt.Errorf("resolve %s for %s: %w", ref, dir, err)
	}
	return strings.TrimSpace(out), nil
}

func defaultRemoteBranch(dir string) (ref, branch string, err error) {
	out, err := run(dir, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")
	if err == nil {
		ref = strings.TrimSpace(out)
		return ref, strings.TrimPrefix(ref, "origin/"), nil
	}
	for _, candidate := range []string{"origin/main", "origin/master"} {
		if _, err := rev(dir, candidate); err == nil {
			return candidate, strings.TrimPrefix(candidate, "origin/"), nil
		}
	}
	return "", "", errors.New("origin default branch is unavailable")
}

func attachBranch(dir, branch string) error {
	if strings.TrimSpace(branch) == "" || branch == "HEAD" {
		return errors.New("default branch is unavailable")
	}
	if branchExists(dir, branch) {
		if _, err := run(dir, "switch", branch); err != nil {
			return fmt.Errorf("attach Installed Repository to %s: %w", branch, err)
		}
		return nil
	}
	if _, err := run(dir, "switch", "--create", branch, "origin/"+branch); err != nil {
		return fmt.Errorf("attach Installed Repository to %s: %w", branch, err)
	}
	if _, err := run(dir, "branch", "--set-upstream-to", "origin/"+branch, branch); err != nil {
		return fmt.Errorf("track Installed Repository branch %s: %w", branch, err)
	}
	return nil
}

func branchExists(dir, branch string) bool {
	_, err := run(dir, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

func canAttachBranch(dir, branch, upstreamRef string) bool {
	if strings.TrimSpace(branch) == "" || branch == "HEAD" {
		return false
	}
	if !branchExists(dir, branch) {
		return true
	}
	return canFastForwardRefTo(dir, branch, upstreamRef)
}

func fetch(dir string) error {
	if _, err := run(dir, "fetch", "--quiet"); err != nil {
		return fmt.Errorf("fetch updates for %s: %w", dir, err)
	}
	return nil
}

func fetchOriginBranches(dir string) error {
	if _, err := run(dir, "config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*"); err != nil {
		return fmt.Errorf("configure origin branch fetch for %s: %w", dir, err)
	}
	if _, err := run(dir, "fetch", "--quiet", "origin", "+refs/heads/*:refs/remotes/origin/*"); err != nil {
		return fmt.Errorf("fetch origin branches for %s: %w", dir, err)
	}
	return nil
}

func canFastForwardTo(dir, ref string) bool {
	// HEAD must be an ancestor of the update target for a fast-forward to be possible.
	return canFastForwardRefTo(dir, "HEAD", ref)
}

func canFastForwardRefTo(dir, from, to string) bool {
	_, err := run(dir, "merge-base", "--is-ancestor", from, to)
	return err == nil
}

func incomingCommitsFrom(dir, ref string) ([]string, error) {
	out, err := run(dir, "log", "--pretty=%h %s", "HEAD.."+ref)
	if err != nil {
		return nil, fmt.Errorf("list incoming commits for %s: %w", dir, err)
	}
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return nil, nil
	}
	return strings.Split(trimmed, "\n"), nil
}

// run executes a git subcommand in dir and returns its standard output. Git
// prompts are disabled so update fails deterministically instead of hanging when
// credentials or remote access need user input.
func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return "", err
		}
		return "", fmt.Errorf("%s", msg)
	}
	return stdout.String(), nil
}
