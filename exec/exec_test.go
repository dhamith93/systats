package exec

import "testing"

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
