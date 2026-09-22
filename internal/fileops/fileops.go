package fileops

import (
	"errors"
	"os"
	"strings"
)

// ReadFile read from given file
func ReadFile(path string) string {
	s, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(s)
}

// ReadFileWithError read file and return string and error
func ReadFileWithError(path string) (string, error) {
	if !IsFile(path) {
		return "", errors.New(path + " file not found")
	}

	return ReadFile(path), nil
}

// IsFile check if path exists and is a regular file (not a directory)
func IsFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func FindFileWithNameLike(dir string, name string) (string, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}

	for _, file := range files {
		if strings.Contains(file.Name(), name) {
			return dir + "/" + file.Name(), nil
		}
	}

	return "", errors.New("file " + name + " not found in " + dir)
}
