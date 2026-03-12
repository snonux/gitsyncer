package sync

import (
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
