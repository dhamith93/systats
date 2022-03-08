package systats

import (
	"errors"

	"github.com/dhamith93/systats/internal/fileops"
)

func readFile(path string) (string, error) {
	if !fileops.IsFile(path) {
		return "", errors.New(path + " file not found")
	}

	return fileops.ReadFile(path), nil
}
