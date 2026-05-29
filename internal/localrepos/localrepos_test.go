package localrepos

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestListLocalRepos_ReturnsGitPathsSorted(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(workDir, "z-repo", ".git"), 0755); err != nil {
		t.Fatalf("failed to create z-repo: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workDir, "a-repo", ".git"), 0755); err != nil {
		t.Fatalf("failed to create a-repo: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workDir, "not-a-repo"), 0755); err != nil {
		t.Fatalf("failed to create non-repo dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "README.txt"), []byte("ignore"), 0644); err != nil {
		t.Fatalf("failed to create file entry: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(workDir, "git-file-repo"), 0755); err != nil {
		t.Fatalf("failed to create git-file-repo dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "git-file-repo", ".git"), []byte("gitdir: /tmp/worktree"), 0644); err != nil {
		t.Fatalf("failed to create .git file: %v", err)
	}

	got, err := ListLocalRepos(workDir)
	if err != nil {
		t.Fatalf("ListLocalRepos returned error: %v", err)
	}

	want := []string{"a-repo", "git-file-repo", "z-repo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListLocalRepos() = %#v, want %#v", got, want)
	}
}

func TestListLocalReposWithGitDir_ExcludesGitFileRepos(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(workDir, "repo-dir", ".git"), 0755); err != nil {
		t.Fatalf("failed to create repo-dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workDir, "repo-file"), 0755); err != nil {
		t.Fatalf("failed to create repo-file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "repo-file", ".git"), []byte("gitdir: /tmp/worktree"), 0644); err != nil {
		t.Fatalf("failed to create repo-file .git file: %v", err)
	}

	got, err := ListLocalReposWithGitDir(workDir)
	if err != nil {
		t.Fatalf("ListLocalReposWithGitDir returned error: %v", err)
	}

	want := []string{"repo-dir"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListLocalReposWithGitDir() = %#v, want %#v", got, want)
	}
}

func TestListLocalRepos_ReturnsReadDirError(t *testing.T) {
	t.Parallel()

	_, err := ListLocalRepos(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("expected error for missing work dir")
	}
}
