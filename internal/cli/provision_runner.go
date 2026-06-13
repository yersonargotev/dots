package cli

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
)

// provisionExecRunner executes an allowlisted provisioner command with HOME
// threaded from the install's --home value, so a sandboxed install lands every
// tool-managed file under the temporary home and never the real one. It
// implements deps.Runner, reusing that boundary for execution; HOME threading is
// this concrete runner's responsibility, not the provision orchestration's.
type provisionExecRunner struct {
	ctx    context.Context
	home   string
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	// baseEnv is the environment to thread HOME into. It is os.Environ() in
	// production and is injectable so sandbox tests never touch the real HOME.
	baseEnv []string
}

func (r provisionExecRunner) Run(executable string, args []string) error {
	cmd := exec.CommandContext(r.ctx, executable, args...)
	cmd.Stdin = r.stdin
	cmd.Stdout = r.stdout
	cmd.Stderr = r.stderr

	base := r.baseEnv
	if base == nil {
		base = os.Environ()
	}
	cmd.Env = envWithHome(base, r.home)
	return cmd.Run()
}

// envWithHome returns base with every HOME entry removed and a single HOME=home
// appended. Dropping inherited HOME entries is required because a child's libc
// getenv commonly returns the first match, so merely appending HOME would let an
// inherited real HOME win over the sandbox value.
func envWithHome(base []string, home string) []string {
	out := make([]string, 0, len(base)+1)
	for _, kv := range base {
		if strings.HasPrefix(kv, "HOME=") {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "HOME="+home)
}
