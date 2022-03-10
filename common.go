package systats

import (
	"errors"
	"os/exec"

	"github.com/dhamith93/systats/internal/fileops"
)

func readFile(path string) (string, error) {
	if !fileops.IsFile(path) {
		return "", errors.New(path + " file not found")
	}

	return fileops.ReadFile(path), nil
}

func Execute(command string, isUsingPipes bool, params ...string) string {
	if isUsingPipes {
		cmd := exec.Command("bash", "-c", command)
		stdout, err := cmd.Output()
		if err != nil {
			return err.Error()
		}
		return string(stdout)
	}

	cmd := exec.Command(command, params...)
	stdout, err := cmd.Output()
	if err != nil {
		return err.Error()
	}
	return string(stdout)
}
