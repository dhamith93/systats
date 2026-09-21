package exec

import (
	"testing"
	"time"
)

func TestExecuteSuccess(t *testing.T) {
	got := Execute("echo", "hello")
	if got != "hello\n" {
		t.Errorf("Execute(echo, hello) = %q, want %q", got, "hello\n")
	}
}

// TestExecuteFailureReturnsErrorTextAsOutput documents the current (odd)
// contract: on failure, Execute returns err.Error() as if it were the
// command's stdout, rather than signaling failure separately. Callers
// that don't know this can end up parsing an error message as data.
func TestExecuteFailureReturnsErrorTextAsOutput(t *testing.T) {
	got := Execute("__systats_nonexistent_binary__")
	if got == "" {
		t.Errorf("Execute() of a missing binary returned empty string, want the error text")
	}
}

func TestExecuteWithPipe(t *testing.T) {
	got := ExecuteWithPipe("echo hi | tr a-z A-Z")
	if got != "HI\n" {
		t.Errorf("ExecuteWithPipe(echo hi | tr a-z A-Z) = %q, want %q", got, "HI\n")
	}
}

func TestExecuteWithErrorSuccess(t *testing.T) {
	got, err := ExecuteWithError("echo", "hello")
	if err != nil {
		t.Errorf("ExecuteWithError(echo, hello) returned error %v, want nil", err)
	}
	if got != "hello\n" {
		t.Errorf("ExecuteWithError(echo, hello) = %q, want %q", got, "hello\n")
	}
}

func TestExecuteWithErrorFailure(t *testing.T) {
	_, err := ExecuteWithError("false")
	if err == nil {
		t.Errorf("ExecuteWithError(false) returned nil error, want non-nil")
	}
}

func TestExecuteWithPipeAndErrorSuccess(t *testing.T) {
	got, err := ExecuteWithPipeAndError("echo hi | tr a-z A-Z")
	if err != nil {
		t.Errorf("ExecuteWithPipeAndError(...) returned error %v, want nil", err)
	}
	if got != "HI\n" {
		t.Errorf("ExecuteWithPipeAndError(...) = %q, want %q", got, "HI\n")
	}
}

func TestExecuteWithPipeAndErrorFailure(t *testing.T) {
	_, err := ExecuteWithPipeAndError("false")
	if err == nil {
		t.Errorf("ExecuteWithPipeAndError(false) returned nil error, want non-nil")
	}
}

// TestExecuteForcesCLocale verifies subprocesses run under LC_ALL=C, so
// their output stays parseable regardless of the host's configured
// locale (e.g. service.go's "Active: active" match against systemctl
// output).
func TestExecuteForcesCLocale(t *testing.T) {
	got := Execute("printenv", "LC_ALL")
	if got != "C\n" {
		t.Errorf("Execute(printenv, LC_ALL) = %q, want %q", got, "C\n")
	}
}

// TestExecuteRespectsTimeout verifies a subprocess is actually killed
// once defaultExecTimeout elapses, rather than blocking the caller
// forever. Shrinks the package-level timeout for the duration of the
// test so this doesn't need to wait out the real 5s default.
func TestExecuteRespectsTimeout(t *testing.T) {
	original := defaultExecTimeout
	defaultExecTimeout = 100 * time.Millisecond
	defer func() { defaultExecTimeout = original }()

	start := time.Now()
	Execute("sleep", "10")
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Errorf("Execute(sleep, 10) took %v with a 100ms timeout, want well under 2s", elapsed)
	}
}
