package sync

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codeberg.org/snonux/gitsyncer/internal/config"
)

// TestCollectAbandoned_RegularAndExcludedBranches exercises the
// collectAbandoned helper shared by analyzeAbandonedBranches' regular and
// excluded-branch passes, confirming both categories apply the same
// six-months-old cutoff and main/master skip, while only the reason-suffix
// text differs between them.
func TestCollectAbandoned_RegularAndExcludedBranches(t *testing.T) {
	repoPath := t.TempDir()
	initBranchAnalyzerTestRepo(t, repoPath)

	oldTime := time.Now().AddDate(-1, 0, 0) // well past the six-month cutoff
	newTime := time.Now()

	commitOnBranch(t, repoPath, "old-feature", oldTime)
	commitOnBranch(t, repoPath, "old-ignored", oldTime)
	commitOnBranch(t, repoPath, "fresh-feature", newTime)

	syncer := New(&config.Config{}, filepath.Dir(repoPath))
	syncer.repoName = filepath.Base(repoPath)

	sixMonthsAgo := time.Now().AddDate(0, -6, 0)

	regular := syncer.collectAbandoned([]string{"old-feature", "fresh-feature", "main"}, sixMonthsAgo, "")
	if len(regular) != 1 || regular[0].Name != "old-feature" {
		t.Fatalf("expected only old-feature to be abandoned, got %#v", regular)
	}
	if !regular[0].IsAbandoned {
		t.Fatalf("expected IsAbandoned to be true, got %#v", regular[0])
	}
	if !strings.HasPrefix(regular[0].AbandonReason, "No commits for ") || strings.Contains(regular[0].AbandonReason, "ignored") {
		t.Fatalf("expected plain abandon reason without ignored suffix, got %q", regular[0].AbandonReason)
	}

	ignored := syncer.collectAbandoned([]string{"old-ignored", "main"}, sixMonthsAgo, " (ignored branch)")
	if len(ignored) != 1 || ignored[0].Name != "old-ignored" {
		t.Fatalf("expected only old-ignored to be abandoned, got %#v", ignored)
	}
	if !strings.HasSuffix(ignored[0].AbandonReason, "(ignored branch)") {
		t.Fatalf("expected abandon reason to carry the ignored-branch suffix, got %q", ignored[0].AbandonReason)
	}

	// A branch with a recent commit must never be reported as abandoned,
	// regardless of which reason suffix is requested.
	fresh := syncer.collectAbandoned([]string{"fresh-feature"}, sixMonthsAgo, " (ignored branch)")
	if len(fresh) != 0 {
		t.Fatalf("expected fresh-feature to not be abandoned, got %#v", fresh)
	}
}

// initBranchAnalyzerTestRepo creates a git repository with a "main" branch
// containing a single initial commit, ready for commitOnBranch to add
// feature branches with controlled commit dates on top of.
func initBranchAnalyzerTestRepo(t *testing.T, repoPath string) {
	t.Helper()

	runGit(t, repoPath, "init")
	runGit(t, repoPath, "checkout", "-b", "main")
	runGit(t, repoPath, "config", "user.name", "Test User")
	runGit(t, repoPath, "config", "user.email", "test@example.com")

	readme := filepath.Join(repoPath, "README.md")
	if err := os.WriteFile(readme, []byte("init"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGit(t, repoPath, "add", "README.md")
	runGit(t, repoPath, "commit", "-m", "initial commit")
}

// commitOnBranch creates a new branch off main with a single commit dated
// "when" (via GIT_AUTHOR_DATE/GIT_COMMITTER_DATE), then returns to main so
// getLastCommitTime("", branch) picks up a deterministic last-commit time.
func commitOnBranch(t *testing.T, repoPath, branch string, when time.Time) {
	t.Helper()

	runGit(t, repoPath, "checkout", "-b", branch)

	filePath := filepath.Join(repoPath, branch+".txt")
	if err := os.WriteFile(filePath, []byte(branch), 0o644); err != nil {
		t.Fatalf("write file for branch %s: %v", branch, err)
	}
	runGit(t, repoPath, "add", branch+".txt")

	dateStr := when.Format(time.RFC3339)
	cmd := exec.Command("git", "commit", "-m", "commit on "+branch)
	cmd.Dir = repoPath
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_DATE="+dateStr,
		"GIT_COMMITTER_DATE="+dateStr,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit on branch %s: %v\n%s", branch, err, output)
	}

	runGit(t, repoPath, "checkout", "main")
}

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
