package sync

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/snonux/gitsyncer/internal/state"
)

func TestEvaluateSyncPolicy_SkipsRepoSyncedWithinDay(t *testing.T) {
	st := &state.State{}
	st.SetLastRepoSync("repo", time.Now().Add(-23*time.Hour))

	decision := EvaluateSyncPolicy("repo", "", st, false, false, false)

	if !decision.Skip {
		t.Fatal("expected repo synced within 24 hours to be skipped")
	}
	if !strings.Contains(decision.Message, "Use --force to override.") {
		t.Fatalf("expected force override hint, got %q", decision.Message)
	}
}

func TestEvaluateSyncPolicy_AllowsRepoAfterDailyWindow(t *testing.T) {
	st := &state.State{}
	st.SetLastRepoSync("repo", time.Now().Add(-25*time.Hour))

	decision := EvaluateSyncPolicy("repo", "", st, false, false, false)

	if decision.Skip {
		t.Fatalf("expected repo synced more than 24 hours ago to proceed, got %q", decision.Message)
	}
}

func TestEvaluateSyncPolicy_ForceBypassesDailyAndThrottleLimits(t *testing.T) {
	workDir := t.TempDir()

	st := &state.State{}
	now := time.Now()
	st.SetRepoSync("repo", now.Add(-1*time.Hour), now.Add(30*24*time.Hour))

	decision := EvaluateSyncPolicy("repo", workDir, st, false, true, true)

	if decision.Skip {
		t.Fatalf("expected --force to bypass sync limits, got %q", decision.Message)
	}
	if decision.SetNextAllowed {
		t.Fatal("did not expect --force to request throttle-window persistence")
	}
}

func TestEvaluateSyncPolicy_ThrottleSetsWindowWhenRepoIsIdle(t *testing.T) {
	// No repo directory exists under workDir, so the repo is treated as
	// having no local clone at all (i.e. definitely no recent commits).
	workDir := t.TempDir()

	start := time.Now()
	decision := EvaluateSyncPolicy("repo", workDir, &state.State{}, false, false, true)
	end := time.Now()

	if !decision.Skip {
		t.Fatal("expected idle repo to be skipped when throttle is enabled")
	}
	if !decision.SetNextAllowed {
		t.Fatal("expected throttle evaluation to request a persisted next-allowed time")
	}

	minAllowed := start.Add(throttleMinDays * 24 * time.Hour)
	maxAllowed := end.Add(throttleMaxDays*24*time.Hour + time.Minute)
	if decision.NextAllowed.Before(minAllowed) || decision.NextAllowed.After(maxAllowed) {
		t.Fatalf("expected throttle window between %s and %s, got %s", minAllowed, maxAllowed, decision.NextAllowed)
	}
}

// TestEvaluateSyncPolicy_UsesConfiguredWorkDirForRecentCommits is a regression
// test for the bug where hasRecentLocalCommits hardcoded ~/git/<repo> instead
// of using the configured work directory. With a custom workDir (deliberately
// not under ~/git) containing a repo with a commit from today, the throttle
// check must detect that activity and NOT skip the repo.
func TestEvaluateSyncPolicy_UsesConfiguredWorkDirForRecentCommits(t *testing.T) {
	workDir := t.TempDir()
	repoName := "custom-repo"
	repoPath := filepath.Join(workDir, repoName)

	initRepoWithCommit(t, repoPath)

	decision := EvaluateSyncPolicy(repoName, workDir, &state.State{}, false, false, true)

	if decision.Skip {
		t.Fatalf("expected repo with recent local commits under custom work dir to proceed, got %q", decision.Message)
	}
	if !strings.Contains(decision.Message, "recent local commits") {
		t.Fatalf("expected message to mention recent local commits, got %q", decision.Message)
	}
}

// initRepoWithCommit creates a git repository at repoPath with a single
// commit dated now, so tests can exercise the "recent local commits" path
// without depending on any real repository on disk.
func initRepoWithCommit(t *testing.T, repoPath string) {
	t.Helper()

	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}

	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repoPath
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, output)
		}
	}

	runGit("init")
	runGit("commit", "--allow-empty", "-m", "initial commit")
}

func TestRecordRepoSync_ClearsThrottleWindowWhenThrottleDisabled(t *testing.T) {
	st := &state.State{}
	st.SetRepoSync("repo", time.Now().Add(-72*time.Hour), time.Now().Add(72*time.Hour))

	RecordRepoSync("repo", st, false)

	if st.GetLastRepoSync("repo").IsZero() {
		t.Fatal("expected last sync time to be recorded")
	}
	if !st.GetNextRepoSyncAllowed("repo").IsZero() {
		t.Fatal("expected throttle window to be cleared when throttle is disabled")
	}
}

func TestRecordRepoSync_SetsThrottleWindowWhenThrottleEnabled(t *testing.T) {
	st := &state.State{}

	RecordRepoSync("repo", st, true)

	lastSync := st.GetLastRepoSync("repo")
	if lastSync.IsZero() {
		t.Fatal("expected last sync time to be recorded")
	}

	nextAllowed := st.GetNextRepoSyncAllowed("repo")
	if nextAllowed.IsZero() {
		t.Fatal("expected throttle window to be recorded")
	}

	minAllowed := lastSync.Add(throttleMinDays * 24 * time.Hour)
	maxAllowed := lastSync.Add(throttleMaxDays*24*time.Hour + time.Minute)
	if nextAllowed.Before(minAllowed) || nextAllowed.After(maxAllowed) {
		t.Fatalf("expected throttle window between %s and %s, got %s", minAllowed, maxAllowed, nextAllowed)
	}
}

func TestRandomThrottleDuration_WithinBoundsAndNotDegenerate(t *testing.T) {
	const draws = 200

	seen := map[time.Duration]struct{}{}
	minDuration := time.Duration(throttleMinDays) * 24 * time.Hour
	maxDuration := time.Duration(throttleMaxDays) * 24 * time.Hour

	for i := 0; i < draws; i++ {
		d := randomThrottleDuration()
		if d < minDuration || d > maxDuration {
			t.Fatalf("expected duration between %s and %s, got %s", minDuration, maxDuration, d)
		}
		if d%(24*time.Hour) != 0 {
			t.Fatalf("expected whole-day duration, got %s", d)
		}
		seen[d] = struct{}{}
	}

	if len(seen) < 2 {
		t.Fatalf("expected at least 2 distinct durations across %d draws, got %d", draws, len(seen))
	}
}
