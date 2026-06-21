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
	upstream, err := upstreamRev(dir)
	if err != nil {
		return Update{}, err
	}
	if old == upstream {
		return Update{OldRev: old, NewRev: old}, nil
	}
	if !canFastForward(dir) {
		return Update{}, ErrNotFastForward
	}
	incoming, err := incomingCommits(dir)
	if err != nil {
		return Update{}, err
	}
	return Update{OldRev: old, NewRev: upstream, Incoming: incoming}, nil
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
	upstream, err := upstreamRev(dir)
	if err != nil {
		return Update{}, err
	}
	if old == upstream {
		return Update{OldRev: old, NewRev: old}, nil
	}
	// Capture the incoming commits before merging so the report reflects exactly
	// what the fast-forward applied.
	incoming, err := incomingCommits(dir)
	if err != nil {
		return Update{}, err
	}
	if _, err := run(dir, "merge", "--ff-only", "@{u}"); err != nil {
		return Update{}, ErrNotFastForward
	}
	newRev, err := head(dir)
	if err != nil {
		return Update{}, err
	}
	return Update{OldRev: old, NewRev: newRev, Incoming: incoming}, nil
}

func head(dir string) (string, error) {
	out, err := run(dir, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func upstreamRev(dir string) (string, error) {
	out, err := run(dir, "rev-parse", "--short", "@{u}")
	if err != nil {
		return "", fmt.Errorf("resolve upstream for %s: %w", dir, err)
	}
	return strings.TrimSpace(out), nil
}

func fetch(dir string) error {
	if _, err := run(dir, "fetch", "--quiet"); err != nil {
		return fmt.Errorf("fetch updates for %s: %w", dir, err)
	}
	return nil
}

func canFastForward(dir string) bool {
	// HEAD must be an ancestor of upstream for a fast-forward to be possible.
	_, err := run(dir, "merge-base", "--is-ancestor", "HEAD", "@{u}")
	return err == nil
}

func incomingCommits(dir string) ([]string, error) {
	out, err := run(dir, "log", "--pretty=%h %s", "HEAD..@{u}")
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
