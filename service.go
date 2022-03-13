package systats

import (
	"regexp"
	"strings"
)

func isServiceRunning(service string) bool {
	output, err := ExecuteWithError("systemctl", "is-active", service)
	if err != nil {
		output = Execute("service", service, "status")
		r, _ := regexp.Compile("(Active: active)")
		return r.Match([]byte(output))
	}
	return strings.TrimSpace(output) == "active"
}
