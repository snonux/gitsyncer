package showcase

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestEffectiveActivityAge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		avgCommitAge float64
		recentCount  int
		recentAvgAge float64
		want         float64
	}{
		// dtail after its history was squashed: 9 recent commits plus 33
		// years-old upstream commits in the 42-commit window.
		{"squashed history uses recent age", 935, 8, 27, 27},
		{"exactly minRecentCommits is enough", 935, minRecentCommits, 40, 40},
		// A few stray commits must not revive a dormant project.
		{"too few recent commits keeps 42-commit age", 935, minRecentCommits - 1, 10, 935},
		// A very active project: the last 42 commits are younger than the
		// average over the whole 90-day window, so it must not get worse.
		{"active project never gets worse", 5, 300, 40, 5},
		{"no recent commits", 1200, 0, 0, 1200},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := effectiveActivityAge(tt.avgCommitAge, tt.recentCount, tt.recentAvgAge)
			if got != tt.want {
				t.Fatalf("effectiveActivityAge(%v, %d, %v) = %v, want %v",
					tt.avgCommitAge, tt.recentCount, tt.recentAvgAge, got, tt.want)
			}
		})
	}
}

func TestAverageAgeDays(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_000_000_000, 0)
	day := int64(86400)
	output := fmt.Sprintf("%d\n\nnot-a-timestamp\n%d\n", now.Unix()-2*day, now.Unix()-4*day)

	count, avg := averageAgeDays(output, now)
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
	if math.Abs(avg-3) > 1e-9 {
		t.Fatalf("avg = %v, want 3", avg)
	}

	if count, avg := averageAgeDays("", now); count != 0 || avg != 0 {
		t.Fatalf("empty output = (%d, %v), want (0, 0)", count, avg)
	}
}

func TestIsInactive_FallsBackToAvgCommitAgeForOldCacheEntries(t *testing.T) {
	t.Parallel()

	// Summaries cached before ActivityAge existed decode with it as 0.
	cached := &RepoMetadata{AvgCommitAge: 1200}
	if !cached.IsInactive() {
		t.Fatal("cached entry with AvgCommitAge 1200 and no ActivityAge should be inactive")
	}

	revived := &RepoMetadata{AvgCommitAge: 1200, ActivityAge: 20}
	if revived.IsInactive() {
		t.Fatal("entry with ActivityAge 20 should be active regardless of AvgCommitAge")
	}
}

// TestExtractRepoMetadata_SquashedHistoryCountsAsActive reproduces the dtail
// case end to end: a long-dormant history followed by a handful of recent
// (squashed) commits. The 42-commit average stays above two years, but the
// project must be scored and reported as active.
func TestExtractRepoMetadata_SquashedHistoryCountsAsActive(t *testing.T) {
	t.Parallel()

	repoPath := newDatedHistoryRepo(t, 40, minRecentCommits)

	metadata, err := extractRepoMetadata(repoPath)
	if err != nil {
		t.Fatalf("extractRepoMetadata() error = %v", err)
	}
	if metadata.AvgCommitAge <= inactiveAfterDays {
		t.Fatalf("AvgCommitAge = %v, want > %d (precondition)", metadata.AvgCommitAge, inactiveAfterDays)
	}
	if metadata.RecentCommitCount != minRecentCommits {
		t.Fatalf("RecentCommitCount = %d, want %d", metadata.RecentCommitCount, minRecentCommits)
	}
	if metadata.ActivityAge >= recentActivityWindowDays {
		t.Fatalf("ActivityAge = %v, want < %d", metadata.ActivityAge, recentActivityWindowDays)
	}
	if metadata.IsInactive() {
		t.Fatal("squashed but recently active project reported as inactive")
	}
}

// TestExtractRepoMetadata_FewRecentCommitsStayInactive guards the other side
// of the rule: fewer than minRecentCommits recent commits (e.g. a
// deprecation notice and a dependency bump) must not revive a dormant
// project.
func TestExtractRepoMetadata_FewRecentCommitsStayInactive(t *testing.T) {
	t.Parallel()

	repoPath := newDatedHistoryRepo(t, 40, minRecentCommits-1)

	metadata, err := extractRepoMetadata(repoPath)
	if err != nil {
		t.Fatalf("extractRepoMetadata() error = %v", err)
	}
	if metadata.ActivityAge != metadata.AvgCommitAge {
		t.Fatalf("ActivityAge = %v, want AvgCommitAge %v", metadata.ActivityAge, metadata.AvgCommitAge)
	}
	if !metadata.IsInactive() {
		t.Fatal("dormant project with only a few recent commits reported as active")
	}
}

// newDatedHistoryRepo creates a repository with oldCommits commits dated
// about three years ago followed by recentCommits commits from the last few
// days. Author and committer dates are both set because git log --since
// filters on the committer date while the age average reads the author date.
func newDatedHistoryRepo(t *testing.T, oldCommits, recentCommits int) string {
	t.Helper()

	repoPath := t.TempDir()
	runGit(t, repoPath, "init", "--initial-branch=main")
	runGit(t, repoPath, "config", "user.name", "Test User")
	runGit(t, repoPath, "config", "user.email", "test@example.com")

	now := time.Now()
	old := now.AddDate(-3, 0, 0)
	for i := 0; i < oldCommits; i++ {
		commitAt(t, repoPath, old.Add(time.Duration(i)*time.Hour), fmt.Sprintf("old %d", i))
	}
	for i := 0; i < recentCommits; i++ {
		commitAt(t, repoPath, now.AddDate(0, 0, -(recentCommits-i)), fmt.Sprintf("recent %d", i))
	}
	return repoPath
}

// commitAt records an empty commit with both author and committer date set
// to when.
func commitAt(t *testing.T, repoPath string, when time.Time, message string) {
	t.Helper()

	date := when.Format(time.RFC3339)
	cmd := exec.Command("git", "-C", repoPath, "commit", "--allow-empty", "-q", "-m", message)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit %q failed: %v\n%s", message, err, output)
	}
}
