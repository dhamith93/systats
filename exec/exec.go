package exec

import (
	"os/exec"
	"strings"
)

// Execute execs the command with params returns output or error msg
func Execute(command string, params ...string) string {
	cmd := exec.Command(command, params...)
	stdout, err := cmd.Output()
	if err != nil {
		return err.Error()
	}
	return string(stdout)
}

// ExecuteWithPipe execs commands with pipe returns output or error msg
func ExecuteWithPipe(command string) string {
	cmd := exec.Command("bash", "-c", command)
	stdout, err := cmd.Output()
	if err != nil {
		return err.Error()
	}
	return string(stdout)
}

// ExecuteWithError execs the command with params returns output and error
func ExecuteWithError(command string, params ...string) (string, error) {
	cmd := exec.Command(command, params...)
	stdout, err := cmd.Output()
	if err != nil {
		return string(stdout), err
	}
	return string(stdout), nil
}

// ExecuteWithPipeAndError execs commands with pipe returns output and error
func ExecuteWithPipeAndError(command string, params ...string) (string, error) {
	cmd := exec.Command("bash", "-c", command)
	stdout, err := cmd.Output()
	if err != nil {
		return string(stdout), err
	}
	return string(stdout), nil
}

// GetExecPath returns the execpath of binary
func GetExecPath(cmd string) string {
	result := Execute("whereis", cmd)
	result = strings.TrimSpace(result)
	resultArr := strings.Fields(result)
	if len(resultArr) == 1 {
		return ""
	}
	return resultArr[1]
}
