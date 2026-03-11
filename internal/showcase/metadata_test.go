package showcase

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCalculateRepoScore_IncreasesWithTagCount(t *testing.T) {
	t.Parallel()

	withoutTags := calculateRepoScore(5000, 14, 0)
	withTags := calculateRepoScore(5000, 14, 10)

	if withTags <= withoutTags {
		t.Fatalf("expected tags to increase score, got without=%f with=%f", withoutTags, withTags)
	}
}

func TestCalculateRepoScore_DecreasesWithAge(t *testing.T) {
	t.Parallel()

	recent := calculateRepoScore(5000, 7, 3)
	old := calculateRepoScore(5000, 70, 3)

	if recent <= old {
		t.Fatalf("expected newer activity to score higher, got recent=%f old=%f", recent, old)
	}
}

func TestGetLatestTag_ReturnsTotalTagCount(t *testing.T) {
	t.Parallel()

	repoPath := t.TempDir()
	runGit(t, repoPath, "init")
	runGit(t, repoPath, "config", "user.name", "Test User")
	runGit(t, repoPath, "config", "user.email", "test@example.com")

	writeAndCommit := func(name, content, message string) {
		path := filepath.Join(repoPath, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write file %s: %v", name, err)
		}
		runGit(t, repoPath, "add", name)
		runGit(t, repoPath, "commit", "-m", message)
	}

	writeAndCommit("README.md", "first", "first")
	runGit(t, repoPath, "tag", "notes")
	runGit(t, repoPath, "tag", "v1.0.0")

	writeAndCommit("README.md", "second", "second")
	runGit(t, repoPath, "tag", "v1.1.0")

	latestTag, _, hasReleases, tagCount, err := getLatestTag(repoPath)
	if err != nil {
		t.Fatalf("getLatestTag() error = %v", err)
	}
	if latestTag != "v1.1.0" {
		t.Fatalf("latestTag = %q, want %q", latestTag, "v1.1.0")
	}
	if !hasReleases {
		t.Fatal("expected hasReleases to be true")
	}
	if tagCount != 3 {
		t.Fatalf("tagCount = %d, want %d", tagCount, 3)
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
