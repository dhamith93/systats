package exec

import (
	"os/exec"
	"strings"
)

func Execute(command string, params ...string) string {
	cmd := exec.Command(command, params...)
	stdout, err := cmd.Output()
	if err != nil {
		return err.Error()
	}
	return string(stdout)
}

func ExecuteWithPipe(command string) string {
	cmd := exec.Command("bash", "-c", command)
	stdout, err := cmd.Output()
	if err != nil {
		return err.Error()
	}
	return string(stdout)
}

func ExecuteWithError(command string, params ...string) (string, error) {
	cmd := exec.Command(command, params...)
	stdout, err := cmd.Output()
	if err != nil {
		return string(stdout), err
	}
	return string(stdout), nil
}

func ExecuteWithPipeAndError(command string, params ...string) (string, error) {
	cmd := exec.Command("bash", "-c", command)
	stdout, err := cmd.Output()
	if err != nil {
		return string(stdout), err
	}
	return string(stdout), nil
}

func GetExecPath(cmd string) string {
	result := Execute("whereis", cmd)
	result = strings.TrimSpace(result)
	resultArr := strings.Fields(result)
	if len(resultArr) == 1 {
		return ""
	}
	return resultArr[1]
}
