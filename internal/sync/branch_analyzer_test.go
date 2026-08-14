package sync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFilterProtectedAbandonedBranchReport_SkipsProtectedBranches(t *testing.T) {
	report := &AbandonedBranchReport{
		AbandonedBranches: []BranchInfo{
			{Name: "hosts"},
			{Name: "feature/still-delete"},
		},
		AbandonedIgnoredBranches: []BranchInfo{
			{Name: "hosts"},
			{Name: "ignored/still-delete"},
		},
	}

	filtered := filterProtectedAbandonedBranchReport("xerl", report)

	if len(filtered.AbandonedBranches) != 1 || filtered.AbandonedBranches[0].Name != "feature/still-delete" {
		t.Fatalf("expected protected abandoned branch to be filtered, got %#v", filtered.AbandonedBranches)
	}

	if len(filtered.AbandonedIgnoredBranches) != 1 || filtered.AbandonedIgnoredBranches[0].Name != "ignored/still-delete" {
		t.Fatalf("expected protected ignored branch to be filtered, got %#v", filtered.AbandonedIgnoredBranches)
	}

	if len(report.AbandonedBranches) != 2 || len(report.AbandonedIgnoredBranches) != 2 {
		t.Fatalf("expected original report to remain unchanged, got %#v", report)
	}
}

func TestGenerateDeleteCommands_SkipsProtectedXerlHostsBranchOnly(t *testing.T) {
	syncer := &Syncer{}
	report := &AbandonedBranchReport{
		AbandonedBranches: []BranchInfo{
			{
				Name:              "hosts",
				LastCommit:        time.Date(2024, time.January, 2, 0, 0, 0, 0, time.UTC),
				RemotesWithBranch: []string{"origin"},
			},
			{
				Name:              "feature/still-delete",
				LastCommit:        time.Date(2024, time.January, 3, 0, 0, 0, 0, time.UTC),
				RemotesWithBranch: []string{"origin"},
			},
		},
	}

	commands := syncer.GenerateDeleteCommands(report, "xerl")

	if strings.Contains(commands, "hosts") {
		t.Fatalf("expected protected branch to be omitted from delete commands, got %q", commands)
	}

	if !strings.Contains(commands, "feature/still-delete") {
		t.Fatalf("expected non-protected branch to remain in delete commands, got %q", commands)
	}
}

func TestGenerateDeleteScript_ReturnsEmptyWhenOnlyProtectedBranchesRemain(t *testing.T) {
	syncer := &Syncer{
		workDir: t.TempDir(),
		abandonedReports: map[string]*AbandonedBranchReport{
			"xerl": {
				MainBranchUpdated: true,
				AbandonedBranches: []BranchInfo{
					{
						Name:              "hosts",
						LastCommit:        time.Date(2024, time.January, 2, 0, 0, 0, 0, time.UTC),
						RemotesWithBranch: []string{"origin"},
					},
				},
			},
		},
	}

	scriptPath, err := syncer.GenerateDeleteScript()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if scriptPath != "" {
		t.Fatalf("expected no delete script for protected branches, got %q", scriptPath)
	}
}

func TestGenerateDeleteScript_WritesRegularAndIgnoredBlocks(t *testing.T) {
	workDir := t.TempDir()
	syncer := &Syncer{
		workDir: workDir,
		abandonedReports: map[string]*AbandonedBranchReport{
			"repo-a": {
				AbandonedBranches: []BranchInfo{
					{
						Name:              "feature/old",
						LastCommit:        time.Date(2024, time.January, 3, 0, 0, 0, 0, time.UTC),
						RemotesWithBranch: []string{"origin", "backup"},
					},
				},
				AbandonedIgnoredBranches: []BranchInfo{
					{
						Name:              "ignored/old",
						LastCommit:        time.Date(2024, time.January, 4, 0, 0, 0, 0, time.UTC),
						RemotesWithBranch: []string{"origin"},
					},
				},
			},
		},
	}

	scriptPath, err := syncer.GenerateDeleteScript()
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

func TestGenerateDeleteScript_ReturnsErrorWhenWorkDirIsFile(t *testing.T) {
	tempDir := t.TempDir()
	workDirFile := filepath.Join(tempDir, "work-dir-file")
	if err := os.WriteFile(workDirFile, []byte("not a directory"), 0600); err != nil {
		t.Fatalf("failed to create temp file for test setup: %v", err)
	}

	syncer := &Syncer{
		workDir: workDirFile,
		abandonedReports: map[string]*AbandonedBranchReport{
			"repo-a": {
				AbandonedBranches: []BranchInfo{
					{
						Name:       "feature/old",
						LastCommit: time.Date(2024, time.January, 3, 0, 0, 0, 0, time.UTC),
					},
				},
			},
		},
	}

	scriptPath, err := syncer.GenerateDeleteScript()
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
