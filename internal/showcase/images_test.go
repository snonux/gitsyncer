package showcase

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCopyFile_CopiesContent verifies the basic success path still works
// after copyFile was changed to use a named return so that a destination
// Close() error can be surfaced without changing the happy-path result.
func TestCopyFile_CopiesContent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := filepath.Join(dir, "source.txt")
	dst := filepath.Join(dir, "dest.txt")
	want := "hello showcase\n"

	if err := os.WriteFile(src, []byte(want), 0644); err != nil {
		t.Fatalf("failed to write source file: %v", err)
	}

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile() returned error: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("failed to read destination file: %v", err)
	}
	if string(got) != want {
		t.Fatalf("copyFile() wrote %q, want %q", string(got), want)
	}
}

// TestCopyFile_MissingSourceReturnsError verifies that a missing source file
// still produces an error (and never a nil error masked by the deferred
// destination-close handling) and does not leave a destination file behind.
func TestCopyFile_MissingSourceReturnsError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := filepath.Join(dir, "does-not-exist.txt")
	dst := filepath.Join(dir, "dest.txt")

	err := copyFile(src, dst)
	if err == nil {
		t.Fatal("copyFile() with missing source returned nil error, want non-nil")
	}

	if _, statErr := os.Stat(dst); statErr == nil {
		t.Fatal("copyFile() with missing source created a destination file, want none")
	}
}
