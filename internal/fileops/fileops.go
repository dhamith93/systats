package fileops

import (
	"errors"
	"io/ioutil"
	"os"
	"strings"

	"github.com/dhamith93/systats/internal/logger"
)

// ReadFile read from given file
func ReadFile(path string) string {
	s, err := ioutil.ReadFile(path)
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

// WriteFile write to given file
func WriteFile(path string, input string) {
	s := []byte(input)
	err := ioutil.WriteFile(path, s, 0644)
	if err != nil {
		logger.Log("Error", err.Error())
	}
}

// IsFile check if file exists
func IsFile(path string) bool {
	_, err := os.Open(path)
	return err == nil
}

func FindFileWithNameLike(dir string, name string) (string, error) {
	files, err := ioutil.ReadDir(dir)
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
