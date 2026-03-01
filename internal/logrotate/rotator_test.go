package logrotate_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lantern/internal/logrotate"
)

func TestRotatingFile_WritesContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	r := logrotate.New(path, 1024*1024) // 1 MB limit
	if _, err := r.Write([]byte("hello\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "hello") {
		t.Errorf("expected file to contain 'hello', got: %q", data)
	}
}

func TestRotatingFile_RotatesAtLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	// Set a tiny limit so we rotate after the first write.
	r := logrotate.New(path, 10)

	if _, err := r.Write([]byte("this line is definitely longer than 10 bytes\n")); err != nil {
		t.Fatalf("first Write: %v", err)
	}
	// Second write should trigger rotation of the first file.
	if _, err := r.Write([]byte("second line\n")); err != nil {
		t.Fatalf("second Write: %v", err)
	}

	rotated := path + ".1"
	if _, err := os.Stat(rotated); os.IsNotExist(err) {
		t.Errorf("expected rotated file %q to exist", rotated)
	}
}

func TestRotatingFile_OverwritesOldRotated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	// Pre-create a .1 file.
	old := path + ".1"
	if err := os.WriteFile(old, []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}

	r := logrotate.New(path, 5) // very small limit
	r.Write([]byte("aaaaaaaaaa\n"))
	r.Write([]byte("bbbbbbbbbb\n")) // triggers rotation, overwrites .1

	data, _ := os.ReadFile(old)
	if strings.Contains(string(data), "old") {
		t.Error("expected .1 to be overwritten, still contains 'old'")
	}
}
