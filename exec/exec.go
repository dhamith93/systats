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

// run is the single place a subprocess is actually started. defaultExecTimeout
// is applied on top of whatever the caller's context already carries, so it
// stays a backstop against a wedged binary while an earlier caller deadline
// still wins.
func run(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultExecTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	withLocale(cmd)
	stdout, err := cmd.Output()
	return string(stdout), err
}

// Execute execs the command with params returns output or error msg
func Execute(command string, params ...string) string {
	return ExecuteWithContext(context.Background(), command, params...)
}

// ExecuteWithContext is Execute, bounded by ctx.
func ExecuteWithContext(ctx context.Context, command string, params ...string) string {
	stdout, err := run(ctx, command, params...)
	if err != nil {
		return err.Error()
	}
	return stdout
}

// ExecuteWithPipe execs commands with pipe returns output or error msg
func ExecuteWithPipe(command string) string {
	return ExecuteWithPipeAndContext(context.Background(), command)
}

// ExecuteWithPipeAndContext is ExecuteWithPipe, bounded by ctx.
func ExecuteWithPipeAndContext(ctx context.Context, command string) string {
	stdout, err := run(ctx, "bash", "-c", command)
	if err != nil {
		return err.Error()
	}
	return stdout
}

// ExecuteWithError execs the command with params returns output and error
func ExecuteWithError(command string, params ...string) (string, error) {
	return ExecuteWithErrorAndContext(context.Background(), command, params...)
}

// ExecuteWithErrorAndContext is ExecuteWithError, bounded by ctx.
func ExecuteWithErrorAndContext(ctx context.Context, command string, params ...string) (string, error) {
	return run(ctx, command, params...)
}

// ExecuteWithPipeAndError execs commands with pipe returns output and error
func ExecuteWithPipeAndError(command string) (string, error) {
	return ExecuteWithPipeAndErrorAndContext(context.Background(), command)
}

// ExecuteWithPipeAndErrorAndContext is ExecuteWithPipeAndError, bounded by ctx.
func ExecuteWithPipeAndErrorAndContext(ctx context.Context, command string) (string, error) {
	return run(ctx, "bash", "-c", command)
}
