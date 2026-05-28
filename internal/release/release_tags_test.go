package release

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func TestGetLocalTags_FiltersToStrictVersionTags(t *testing.T) {
	t.Parallel()

	repoPath := t.TempDir()
	runGit(t, repoPath, "init")
	runGit(t, repoPath, "config", "user.name", "Test User")
	runGit(t, repoPath, "config", "user.email", "test@example.com")

	readmePath := filepath.Join(repoPath, "README.md")
	if err := os.WriteFile(readmePath, []byte("test"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit(t, repoPath, "add", "README.md")
	runGit(t, repoPath, "commit", "-m", "initial commit")

	runGit(t, repoPath, "tag", "v2")
	runGit(t, repoPath, "tag", "1.2")
	runGit(t, repoPath, "tag", "v1.0.0")
	runGit(t, repoPath, "tag", "latest")
	runGit(t, repoPath, "tag", "1-beta")

	manager := NewManager("")
	tags, err := manager.GetLocalTags(repoPath)
	if err != nil {
		t.Fatalf("GetLocalTags() error = %v", err)
	}

	want := []string{"v1.0.0", "1.2", "v2"}
	if !reflect.DeepEqual(tags, want) {
		t.Fatalf("GetLocalTags() = %#v, want %#v", tags, want)
	}
}

func runGit(t *testing.T, repoPath string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", append([]string{"-C", repoPath}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(output))
	}

	return string(output)
}
