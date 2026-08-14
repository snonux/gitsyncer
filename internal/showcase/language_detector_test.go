package showcase

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCountLanguageLines_SkipsVendorAndHiddenDirs exercises the
// filepath.WalkDir-based directory skip rule in countLanguageLines: files
// under vendor/ and hidden directories (e.g. .git) must not contribute to
// the line counts, while ordinary source and doc files are counted.
func TestCountLanguageLines_SkipsVendorAndHiddenDirs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	writeTestFile(t, filepath.Join(dir, "main.go"), "package main\n\nfunc main() {}\n")
	writeTestFile(t, filepath.Join(dir, "README.md"), "# Title\n\nSome docs.\n")
	writeTestFile(t, filepath.Join(dir, "vendor", "ignored.go"), "package vendor\n\nfunc Ignored() {}\n")
	writeTestFile(t, filepath.Join(dir, ".git", "ignored2.go"), "package git\n\nfunc Ignored() {}\n")

	languageLines, documentationLines, err := countLanguageLines(dir)
	if err != nil {
		t.Fatalf("countLanguageLines() error = %v", err)
	}

	if got, want := languageLines["Go"], 3; got != want {
		t.Fatalf("languageLines[Go] = %d, want %d (vendor/.git files must be skipped)", got, want)
	}
	if got, want := documentationLines["Markdown"], 3; got != want {
		t.Fatalf("documentationLines[Markdown] = %d, want %d", got, want)
	}
}

// writeTestFile writes content to path, creating any missing parent
// directories first. Shared by the showcase package's walk-related tests.
func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}
