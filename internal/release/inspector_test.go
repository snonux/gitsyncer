package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGetDiffBetweenTags_EmptyFromTag exercises the fromTag == "" branch,
// which resolves the repository's root commit and diffs from there up to
// toTag. This also covers the "leading cmd assignment" code path that was
// previously flagged as containing a dead exec.Command assignment ahead of
// the if/else (task s01) — the current implementation declares cmd via
// `var cmd *exec.Cmd` and assigns it exactly once, inside the branch that
// applies, so there is no dead assignment to remove.
func TestGetDiffBetweenTags_EmptyFromTag(t *testing.T) {
	t.Parallel()

	repoPath := t.TempDir()
	runGit(t, repoPath, "init")
	runGit(t, repoPath, "config", "user.name", "Test User")
	runGit(t, repoPath, "config", "user.email", "test@example.com")

	readmePath := filepath.Join(repoPath, "README.md")
	if err := os.WriteFile(readmePath, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit(t, repoPath, "add", "README.md")
	runGit(t, repoPath, "commit", "-m", "initial commit")
	runGit(t, repoPath, "tag", "v1.0.0")

	inspector := NewGitInspector()
	diff, err := inspector.GetDiffBetweenTags(repoPath, "", "v1.0.0")
	if err != nil {
		t.Fatalf("GetDiffBetweenTags() error = %v", err)
	}

	if !strings.Contains(diff, "README.md") {
		t.Fatalf("GetDiffBetweenTags() = %q, want output mentioning README.md", diff)
	}
}

// TestGetDiffBetweenTags_NonEmptyFromTag exercises the fromTag != "" branch,
// which diffs directly between the two tags.
func TestGetDiffBetweenTags_NonEmptyFromTag(t *testing.T) {
	t.Parallel()

	repoPath := t.TempDir()
	runGit(t, repoPath, "init")
	runGit(t, repoPath, "config", "user.name", "Test User")
	runGit(t, repoPath, "config", "user.email", "test@example.com")

	readmePath := filepath.Join(repoPath, "README.md")
	if err := os.WriteFile(readmePath, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit(t, repoPath, "add", "README.md")
	runGit(t, repoPath, "commit", "-m", "initial commit")
	runGit(t, repoPath, "tag", "v1.0.0")

	mainPath := filepath.Join(repoPath, "main.go")
	if err := os.WriteFile(mainPath, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit(t, repoPath, "add", "main.go")
	runGit(t, repoPath, "commit", "-m", "add main.go")
	runGit(t, repoPath, "tag", "v1.1.0")

	inspector := NewGitInspector()
	diff, err := inspector.GetDiffBetweenTags(repoPath, "v1.0.0", "v1.1.0")
	if err != nil {
		t.Fatalf("GetDiffBetweenTags() error = %v", err)
	}

	if !strings.Contains(diff, "main.go") {
		t.Fatalf("GetDiffBetweenTags() = %q, want output mentioning main.go", diff)
	}
}
