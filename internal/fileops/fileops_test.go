package fileops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "content.txt")
	if err := os.WriteFile(path, []byte("hello\n"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if got := ReadFile(path); got != "hello\n" {
		t.Errorf("ReadFile() = %q, want %q", got, "hello\n")
	}
}

// ReadFile reports an unreadable file as empty rather than erroring -
// callers that need to tell those apart use ReadFileWithError.
func TestReadFileMissingReturnsEmpty(t *testing.T) {
	if got := ReadFile(filepath.Join(t.TempDir(), "nope.txt")); got != "" {
		t.Errorf("ReadFile() on a missing path = %q, want empty", got)
	}
}

func TestReadFileWithError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "content.txt")
	if err := os.WriteFile(path, []byte("data"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	got, err := ReadFileWithError(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "data" {
		t.Errorf("ReadFileWithError() = %q, want %q", got, "data")
	}

	if _, err := ReadFileWithError(filepath.Join(dir, "nope.txt")); err == nil {
		t.Errorf("ReadFileWithError() on a missing path returned nil error")
	}
}

// A directory is not a file. /proc and /sys are full of directories where
// a caller might expect a file, so this distinction matters.
func TestIsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "content.txt")
	if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if !IsFile(path) {
		t.Errorf("IsFile(regular file) = false, want true")
	}
	if IsFile(dir) {
		t.Errorf("IsFile(directory) = true, want false")
	}
	if IsFile(filepath.Join(dir, "nope.txt")) {
		t.Errorf("IsFile(missing path) = true, want false")
	}
}

// ReadFileWithError on a directory must fail rather than returning the
// empty string ReadFile would give it.
func TestReadFileWithErrorRejectsDirectory(t *testing.T) {
	if _, err := ReadFileWithError(t.TempDir()); err == nil {
		t.Errorf("ReadFileWithError(directory) returned nil error")
	}
}

func TestFindFileWithNameLike(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"hostname", "os-release", "passwd"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	got, err := FindFileWithNameLike(dir, "-release")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(got, "os-release") {
		t.Errorf("FindFileWithNameLike() = %q, want a path ending in os-release", got)
	}

	if _, err := FindFileWithNameLike(dir, "nomatch"); err == nil {
		t.Errorf("FindFileWithNameLike() with no match returned nil error")
	}
	if _, err := FindFileWithNameLike(filepath.Join(dir, "nodir"), "x"); err == nil {
		t.Errorf("FindFileWithNameLike() on a missing directory returned nil error")
	}
}
