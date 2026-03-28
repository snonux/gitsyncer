package showcase

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCalculateRepoScore_IncreasesWithTagCount(t *testing.T) {
	t.Parallel()

	withoutTags := calculateRepoScore(5000, 14, 0, true)
	withTags := calculateRepoScore(5000, 14, 10, true)

	if withTags <= withoutTags {
		t.Fatalf("expected tags to increase score, got without=%f with=%f", withoutTags, withTags)
	}
}

func TestCalculateRepoScore_DecreasesWithAge(t *testing.T) {
	t.Parallel()

	recent := calculateRepoScore(5000, 7, 3, true)
	old := calculateRepoScore(5000, 70, 3, true)

	if recent <= old {
		t.Fatalf("expected newer activity to score higher, got recent=%f old=%f", recent, old)
	}
}

func TestCalculateRepoScore_DecreasesWithoutRelease(t *testing.T) {
	t.Parallel()

	released := calculateRepoScore(5000, 14, 3, true)
	unreleased := calculateRepoScore(5000, 14, 3, false)

	if unreleased >= released {
		t.Fatalf("expected unreleased repo to score lower, got released=%f unreleased=%f", released, unreleased)
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

func TestExtractRepoMetadata_UsesCurrentBranchState(t *testing.T) {
	t.Parallel()

	repoPath := t.TempDir()
	runGit(t, repoPath, "init", "--initial-branch=main")
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

	writeAndCommit("main.go", "package main\n\nfunc main() {\n}\n", "main branch")
	runGit(t, repoPath, "tag", "v1.0.0")

	runGit(t, repoPath, "checkout", "-b", "content-gemtext")
	writeAndCommit("content.go", "package main\n\nfunc render() string {\n\treturn \"gemtext\"\n}\n", "content branch")
	runGit(t, repoPath, "tag", "v2.0.0")

	runGit(t, repoPath, "checkout", "main")

	mainMetadata, err := extractRepoMetadata(repoPath)
	if err != nil {
		t.Fatalf("extractRepoMetadata(main) error = %v", err)
	}
	if mainMetadata.CommitCount != 1 {
		t.Fatalf("main branch CommitCount = %d, want %d", mainMetadata.CommitCount, 1)
	}
	if mainMetadata.LinesOfCode != 4 {
		t.Fatalf("main branch LinesOfCode = %d, want %d", mainMetadata.LinesOfCode, 4)
	}
	if mainMetadata.LatestTag != "v1.0.0" {
		t.Fatalf("main branch LatestTag = %q, want %q", mainMetadata.LatestTag, "v1.0.0")
	}
	if mainMetadata.TagCount != 1 {
		t.Fatalf("main branch TagCount = %d, want %d", mainMetadata.TagCount, 1)
	}

	runGit(t, repoPath, "checkout", "content-gemtext")

	contentMetadata, err := extractRepoMetadata(repoPath)
	if err != nil {
		t.Fatalf("extractRepoMetadata(content-gemtext) error = %v", err)
	}
	if contentMetadata.CommitCount != 2 {
		t.Fatalf("content-gemtext CommitCount = %d, want %d", contentMetadata.CommitCount, 2)
	}
	if contentMetadata.LinesOfCode != 9 {
		t.Fatalf("content-gemtext LinesOfCode = %d, want %d", contentMetadata.LinesOfCode, 9)
	}
	if contentMetadata.LatestTag != "v2.0.0" {
		t.Fatalf("content-gemtext LatestTag = %q, want %q", contentMetadata.LatestTag, "v2.0.0")
	}
	if contentMetadata.TagCount != 2 {
		t.Fatalf("content-gemtext TagCount = %d, want %d", contentMetadata.TagCount, 2)
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
