package exec

import (
	"context"
	"os"
	"os/exec"
	"time"
)

// defaultExecTimeout bounds how long a subprocess is allowed to run. It's
// a var (not a const) so tests can shrink it to verify the timeout is
// actually wired up without waiting out the real default.
var defaultExecTimeout = 5 * time.Second

// withLocale forces a subprocess to run under the C locale, so its output
// is in a consistent, parseable form regardless of the host's configured
// language (e.g. systemctl/service status text).
func withLocale(cmd *exec.Cmd) {
	cmd.Env = append(os.Environ(), "LC_ALL=C")
}

// Execute execs the command with params returns output or error msg
func Execute(command string, params ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), defaultExecTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, command, params...)
	withLocale(cmd)
	stdout, err := cmd.Output()
	if err != nil {
		return err.Error()
	}
	return string(stdout)
}

// ExecuteWithPipe execs commands with pipe returns output or error msg
func ExecuteWithPipe(command string) string {
	ctx, cancel := context.WithTimeout(context.Background(), defaultExecTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	withLocale(cmd)
	stdout, err := cmd.Output()
	if err != nil {
		return err.Error()
	}
	return string(stdout)
}

// ExecuteWithError execs the command with params returns output and error
func ExecuteWithError(command string, params ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultExecTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, command, params...)
	withLocale(cmd)
	stdout, err := cmd.Output()
	if err != nil {
		return string(stdout), err
	}
	return string(stdout), nil
}

// ExecuteWithPipeAndError execs commands with pipe returns output and error
func ExecuteWithPipeAndError(command string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultExecTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	withLocale(cmd)
	stdout, err := cmd.Output()
	if err != nil {
		return string(stdout), err
	}
	return string(stdout), nil
}
