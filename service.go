package systats

import (
	"regexp"
	"strings"

	"github.com/dhamith93/systats/exec"
)

func isServiceRunning(service string) bool {
	output, err := exec.ExecuteWithError("systemctl", "is-active", service)
	if err != nil {
		output = exec.Execute("service", service, "status")
		r, _ := regexp.Compile("(Active: active)")
		return r.Match([]byte(output))
	}
	return strings.TrimSpace(output) == "active"
}
