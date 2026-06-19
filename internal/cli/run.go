package cli

import (
	"errors"
	"fmt"
	"io"
)

// Process exit codes that form the agent-facing contract. They let a caller
// branch on the outcome of a read-only diagnostic command without parsing its
// human-readable output.
const (
	// ExitOK means the command ran and the workstation is aligned with the
	// Source of Truth.
	ExitOK = 0
	// ExitError means the command failed to run (bad manifest, I/O failure, or
	// misuse). It is reserved for real failures so callers never confuse a
	// broken run with a divergent-but-healthy one.
	ExitError = 1
	// ExitFindings means the command ran successfully but surfaced a non-error
	// divergence the caller should act on: Drift, an unresolved Conflict, a
	// missing Dependency, or a doctor concern.
	ExitFindings = 2
)

// FindingsError signals that a read-only diagnostic command ran successfully
// but surfaced findings. It carries no user-facing message because the command
// has already rendered its report; Run translates it into ExitFindings.
type FindingsError struct{}

func (*FindingsError) Error() string { return "findings" }

// Run builds the root command, executes it with the given arguments, and
// translates the outcome into a semantic process exit code (ExitOK,
// ExitFindings, or ExitError). It is the single owner of error presentation:
// the root command silences cobra's own error and usage printing so a structured
// FindingsError never reaches the user as an "Error:" line.
func Run(args []string, stdout, stderr io.Writer) int {
	root := NewRootCommand()
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)

	// ExecuteC returns the command that actually ran so the error envelope can be
	// labeled and the output mode read from its inherited flags.
	executed, err := root.ExecuteC()
	if err == nil {
		return ExitOK
	}

	var findings *FindingsError
	if errors.As(err, &findings) {
		// The diagnostic envelope (status "findings") was already written; only
		// the exit code remains.
		return ExitFindings
	}

	if wantsJSON(executed) {
		emitError(stdout, executed, err)
	} else {
		fmt.Fprintln(stderr, "Error:", err)
	}
	return ExitError
}
