package systats

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/dhamith93/systats/exec"
)

var serviceActiveRe = regexp.MustCompile(`(Active: active)`)

// isServiceRunning reports whether a service is active, and whether the
// check itself succeeded.
//
// Both tools here exit non-zero for an inactive service as well as for a
// genuine failure, so the exit code alone can't tell those apart - but a
// tool that printed a verdict has answered the question regardless of how
// it exited. Output is therefore what decides, and an error is returned
// only when neither tool said anything at all (no systemctl, no service,
// or a cancelled context). That is what lets a caller distinguish a
// stopped service from a check that never ran.
func isServiceRunning(ctx context.Context, service string) (bool, error) {
	output, err := exec.ExecuteWithErrorAndContext(ctx, "systemctl", "is-active", service)
	if trimmed := strings.TrimSpace(output); trimmed != "" {
		// "active", "inactive", "failed", "unknown", "activating", ...
		return trimmed == "active", nil
	}
	if err == nil {
		// Ran cleanly but said nothing - treat as not active rather than
		// as a failed check.
		return false, nil
	}

	fallback, fallbackErr := exec.ExecuteWithErrorAndContext(ctx, "service", service, "status")
	if strings.TrimSpace(fallback) != "" {
		return serviceActiveRe.MatchString(fallback), nil
	}
	if fallbackErr == nil {
		return false, nil
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		return false, ctxErr
	}
	// errors.Join would be tidier, but go.mod targets Go 1.18.
	return false, fmt.Errorf("systats: could not determine status of %q: systemctl: %v; service: %v", service, err, fallbackErr)
}
