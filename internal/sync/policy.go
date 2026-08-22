package sync

// Sync-scheduling policy: whether a repository is due for a sync at all
// (daily interval) and, when --throttle is set, whether it should be skipped
// further because it has had no recent local activity. This lived in
// internal/cli/throttle.go until task w01 moved it here: it is a decision
// about repository/branch state (the same domain this package already
// reasons about via Syncer), not CLI flag plumbing. internal/cli's
// throttle.go now only translates flags into calls into this package.
import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/snonux/gitsyncer/internal/state"
)

const (
	defaultSyncInterval = 24 * time.Hour
	throttleMinDays     = 60
	throttleMaxDays     = 120
	recentDays          = 17
)

// LoadSyncState opens (or initializes) the persisted sync state under
// workDir. It always returns a non-nil State even on error, so callers can
// proceed with an empty-state fallback the same way the pre-move cli code
// did.
func LoadSyncState(workDir string) (*state.Manager, *state.State, error) {
	manager := state.NewManager(workDir)
	st, err := manager.Load()
	if err != nil {
		return manager, &state.State{}, err
	}
	if st == nil {
		st = &state.State{}
	}
	return manager, st, nil
}

// SyncDecision is the result of evaluating sync policy for one repository:
// whether to skip it, the message to print, and (for throttle) the
// next-allowed time that should be persisted.
type SyncDecision struct {
	Skip           bool
	Message        string
	NextAllowed    time.Time
	SetNextAllowed bool
}

// EvaluateSyncPolicy decides whether repoName should be synced now, applying
// (in order) the --force override, the daily sync-interval check, and, when
// throttle is enabled, the throttle window based on recent local activity.
func EvaluateSyncPolicy(repoName, workDir string, st *state.State, dryRun bool, force bool, throttle bool) SyncDecision {
	if force {
		return SyncDecision{}
	}

	decision := evaluateDailySync(repoName, st, dryRun)
	if decision.Skip || !throttle {
		return decision
	}

	return evaluateThrottle(repoName, workDir, st, dryRun)
}

func evaluateDailySync(repoName string, st *state.State, dryRun bool) SyncDecision {
	if st == nil {
		return SyncDecision{}
	}

	lastSync := st.GetLastRepoSync(repoName)
	if lastSync.IsZero() {
		return SyncDecision{}
	}

	nextAllowed := lastSync.Add(defaultSyncInterval)
	if time.Now().Before(nextAllowed) {
		skipAction := "Skipping"
		if dryRun {
			skipAction = "[DRY RUN] Would skip"
		}

		return SyncDecision{
			Skip: true,
			Message: fmt.Sprintf("%s %s: last synced at %s; next sync after %s. Use --force to override.",
				skipAction,
				repoName,
				lastSync.Format("2006-01-02 15:04"),
				nextAllowed.Format("2006-01-02 15:04")),
		}
	}

	return SyncDecision{}
}

func evaluateThrottle(repoName, workDir string, st *state.State, dryRun bool) SyncDecision {
	syncAction := "Syncing"
	if dryRun {
		syncAction = "[DRY RUN] Would sync"
	}

	recent, err := hasRecentLocalCommits(workDir, repoName)
	if err != nil {
		actionMsg := "Sync will proceed"
		if dryRun {
			actionMsg = "Sync would proceed"
		}
		return SyncDecision{
			Skip:    false,
			Message: fmt.Sprintf("Warning: failed to check local activity for %s: %v. %s.", repoName, err, actionMsg),
		}
	}

	if recent {
		return SyncDecision{
			Skip:    false,
			Message: fmt.Sprintf("%s %s: recent local commits within last %d days.", syncAction, repoName, recentDays),
		}
	}

	now := time.Now()
	if st == nil {
		return SyncDecision{
			Skip:    false,
			Message: fmt.Sprintf("%s %s: no recent local commits; throttle state unavailable.", syncAction, repoName),
		}
	}
	nextAllowed := st.GetNextRepoSyncAllowed(repoName)
	skipAction := "Skipping"
	if dryRun {
		skipAction = "[DRY RUN] Would skip"
	}

	if nextAllowed.IsZero() {
		lastSync := st.GetLastRepoSync(repoName)
		if !lastSync.IsZero() {
			nextAllowed = lastSync.Add(randomThrottleDuration())
		} else {
			nextAllowed = now.Add(randomThrottleDuration())
		}
		return SyncDecision{
			Skip:           true,
			NextAllowed:    nextAllowed,
			SetNextAllowed: true,
			Message: fmt.Sprintf("%s %s: no recent local commits; throttle window set until %s.",
				skipAction, repoName, nextAllowed.Format("2006-01-02")),
		}
	}

	if now.Before(nextAllowed) {
		return SyncDecision{
			Skip:    true,
			Message: fmt.Sprintf("%s %s: no recent local commits; next allowed sync at %s.", skipAction, repoName, nextAllowed.Format("2006-01-02")),
		}
	}

	return SyncDecision{
		Skip:    false,
		Message: fmt.Sprintf("%s %s: throttle window elapsed (next allowed was %s).", syncAction, repoName, nextAllowed.Format("2006-01-02")),
	}
}

// RecordRepoSync updates st to reflect that repoName was just synced,
// setting (or clearing, if throttle is disabled) the throttle window.
func RecordRepoSync(repoName string, st *state.State, throttle bool) {
	if st == nil {
		return
	}
	now := time.Now()
	st.SetLastRepoSync(repoName, now)
	if throttle {
		st.SetNextRepoSyncAllowed(repoName, now.Add(randomThrottleDuration()))
		return
	}
	st.ClearNextRepoSyncAllowed(repoName)
}

func randomThrottleDuration() time.Duration {
	days := throttleMinDays + rand.Intn(throttleMaxDays-throttleMinDays+1)
	return time.Duration(days) * 24 * time.Hour
}

// hasRecentLocalCommits checks whether repoName's local clone under the
// configured workDir (the same directory gitsyncer clones/syncs into, see
// Flags.WorkDir) has commits within the last recentDays days. It previously
// hardcoded ~/git/<repoName>, which meant a custom --work-dir/config work_dir
// was silently ignored and every repo looked inactive, causing --throttle to
// skip repos indefinitely. That fix (task 501) is preserved by this move.
func hasRecentLocalCommits(workDir, repoName string) (bool, error) {
	repoPath := filepath.Join(workDir, repoName)
	info, err := os.Stat(repoPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to stat %s: %w", repoPath, err)
	}
	if !info.IsDir() {
		return false, nil
	}

	cmd := exec.Command("git", "-C", repoPath, "log", "-1", "--since="+fmt.Sprintf("%d.days", recentDays), "--format=%ct")
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("git log failed for %s: %w", repoPath, err)
	}

	return strings.TrimSpace(string(output)) != "", nil
}
