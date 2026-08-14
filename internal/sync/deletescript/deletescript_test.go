package deletescript

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type failingWriter struct{}

func (f failingWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestWriteBranchDeletionBlock_ReturnsWriteError(t *testing.T) {
	err := writeBranchDeletionBlock(
		failingWriter{},
		[]BranchInfo{{Name: "feature/broken", LastCommit: time.Date(2024, time.January, 3, 0, 0, 0, 0, time.UTC)}},
		"regular",
		"🔸 Deleting branch: ",
	)
	if err == nil {
		t.Fatal("expected write error")
	}
	if !strings.Contains(err.Error(), "failed to write review mode condition") {
		t.Fatalf("expected write context in error, got %v", err)
	}
}

func TestWriteDeleteScriptRepoHeader_ReturnsWriteError(t *testing.T) {
	err := writeDeleteScriptRepoHeader(failingWriter{}, "/work", "repo-a")
	if err == nil {
		t.Fatal("expected write error")
	}
	if !strings.Contains(err.Error(), "failed to write repository header") {
		t.Fatalf("expected write context in error, got %v", err)
	}
}

func TestGenerate_ReturnsEmptyWhenNoReports(t *testing.T) {
	scriptPath, err := Generate(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if scriptPath != "" {
		t.Fatalf("expected no script for empty reports, got %q", scriptPath)
	}
}

func TestGenerate_ReturnsEmptyWhenReportsHaveNoBranches(t *testing.T) {
	scriptPath, err := Generate(t.TempDir(), []RepoReport{{RepoName: "repo-a"}})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if scriptPath != "" {
		t.Fatalf("expected no script when every report is empty, got %q", scriptPath)
	}
}

func TestGenerate_WritesRegularAndIgnoredBlocks(t *testing.T) {
	workDir := t.TempDir()
	reports := []RepoReport{
		{
			RepoName: "repo-a",
			Regular: []BranchInfo{
				{
					Name:              "feature/old",
					LastCommit:        time.Date(2024, time.January, 3, 0, 0, 0, 0, time.UTC),
					RemotesWithBranch: []string{"origin", "backup"},
				},
			},
			Ignored: []BranchInfo{
				{
					Name:              "ignored/old",
					LastCommit:        time.Date(2024, time.January, 4, 0, 0, 0, 0, time.UTC),
					RemotesWithBranch: []string{"origin"},
				},
			},
		},
	}

	scriptPath, err := Generate(workDir, reports)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if scriptPath == "" {
		t.Fatal("expected script path to be returned")
	}

	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("expected generated script to be readable, got %v", err)
	}

	script := string(content)
	expectedBase := filepath.Base(scriptPath)
	expectedSnippets := []string{
		"#   bash " + expectedBase + " --review-full  # Review full diffs",
		"# Regular abandoned branches",
		"review_branch \"feature/old\" \"$main_branch\" \"2024-01-03\" \"regular\"",
		"echo \"  🔸 Deleting branch: feature/old (last commit: 2024-01-03)\"",
		"execute_cmd git push origin --delete \"feature/old\"",
		"execute_cmd git push backup --delete \"feature/old\"",
		"# Ignored abandoned branches",
		"review_branch \"ignored/old\" \"$main_branch\" \"2024-01-04\" \"ignored\"",
		"echo \"  🔹 Deleting ignored branch: ignored/old (last commit: 2024-01-04)\"",
		"execute_cmd git push origin --delete \"ignored/old\"",
		"cd \"" + workDir + "/repo-a\"",
		"To delete branches, run: bash " + expectedBase,
	}
	for _, snippet := range expectedSnippets {
		if !strings.Contains(script, snippet) {
			t.Fatalf("expected script to contain %q, got:\n%s", snippet, script)
		}
	}

	info, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("expected generated script to be stat-able, got %v", err)
	}
	if info.Mode().Perm() != 0755 {
		t.Fatalf("expected script permissions to be 0755, got %o", info.Mode().Perm())
	}
}

func TestGenerate_ReturnsErrorWhenWorkDirIsFile(t *testing.T) {
	tempDir := t.TempDir()
	workDirFile := filepath.Join(tempDir, "work-dir-file")
	if err := os.WriteFile(workDirFile, []byte("not a directory"), 0600); err != nil {
		t.Fatalf("failed to create temp file for test setup: %v", err)
	}

	reports := []RepoReport{
		{
			RepoName: "repo-a",
			Regular: []BranchInfo{
				{Name: "feature/old", LastCommit: time.Date(2024, time.January, 3, 0, 0, 0, 0, time.UTC)},
			},
		},
	}

	scriptPath, err := Generate(workDirFile, reports)
	if err == nil {
		t.Fatal("expected an error when workDir is not a directory")
	}
	if scriptPath != "" {
		t.Fatalf("expected empty script path on creation failure, got %q", scriptPath)
	}
	if !strings.Contains(err.Error(), "failed to create script file") {
		t.Fatalf("expected create-file error, got %v", err)
	}
}
