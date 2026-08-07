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
	if resolved, ok := lookPathInEnvironment(executable, r.environment()); ok {
		executable = resolved
	}
	cmd := exec.CommandContext(r.ctx, executable, args...)
	cmd.Stdin = r.stdin
	cmd.Stdout = r.stdout
	cmd.Stderr = r.stderr

	cmd.Env = r.environment()
	return cmd.Run()
}

func (r provisionExecRunner) Lookup(command string) bool {
	_, ok := lookPathInEnvironment(command, r.environment())
	return ok
}

func (r provisionExecRunner) environment() []string {
	base := r.baseEnv
	if base == nil {
		base = os.Environ()
	}
	return envForProvisioner(base, r.home)
}

// envForProvisioner returns base with a sandboxed HOME, a user-local npm prefix,
// and ~/.local/bin first on PATH. Dropping inherited HOME/NPM_CONFIG_PREFIX/PATH
// entries avoids accidental writes to the operator's real home or sudo-backed npm
// globals during non-interactive provisioner runs.
func envForProvisioner(base []string, home string) []string {
	localBin := home + "/.local/bin"
	out := make([]string, 0, len(base)+3)
	path := ""
	for _, kv := range base {
		switch {
		case strings.HasPrefix(kv, "HOME="):
			continue
		case strings.HasPrefix(kv, "NPM_CONFIG_PREFIX="):
			continue
		case strings.HasPrefix(kv, "PATH="):
			path = strings.TrimPrefix(kv, "PATH=")
			continue
		}
		out = append(out, kv)
	}
	if path == "" {
		path = localBin
	} else {
		path = localBin + string(os.PathListSeparator) + path
	}
	return append(out, "HOME="+home, "NPM_CONFIG_PREFIX="+home+"/.local", "PATH="+path)
}
