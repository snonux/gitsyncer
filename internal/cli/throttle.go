package cli

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"codeberg.org/snonux/gitsyncer/internal/state"
)

const (
	throttleMinDays = 60
	throttleMaxDays = 120
	recentDays      = 7
)

func loadThrottleState(workDir string) (*state.Manager, *state.State, error) {
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

type throttleDecision struct {
	Skip           bool
	Message        string
	NextAllowed    time.Time
	SetNextAllowed bool
}

func evaluateThrottle(repoName string, st *state.State, dryRun bool) throttleDecision {
	syncAction := "Syncing"
	if dryRun {
		syncAction = "[DRY RUN] Would sync"
	}

	recent, err := hasRecentLocalCommits(repoName)
	if err != nil {
		actionMsg := "Sync will proceed"
		if dryRun {
			actionMsg = "Sync would proceed"
		}
		return throttleDecision{
			Skip:    false,
			Message: fmt.Sprintf("Warning: failed to check local activity for %s: %v. %s.", repoName, err, actionMsg),
		}
	}

	if recent {
		return throttleDecision{
			Skip:    false,
			Message: fmt.Sprintf("%s %s: recent local commits within last %d days.", syncAction, repoName, recentDays),
		}
	}

	now := time.Now()
	if st == nil {
		return throttleDecision{
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
		return throttleDecision{
			Skip:           true,
			NextAllowed:    nextAllowed,
			SetNextAllowed: true,
			Message: fmt.Sprintf("%s %s: no recent local commits; throttle window set until %s.",
				skipAction, repoName, nextAllowed.Format("2006-01-02")),
		}
	}

	if now.Before(nextAllowed) {
		return throttleDecision{
			Skip:    true,
			Message: fmt.Sprintf("%s %s: no recent local commits; next allowed sync at %s.", skipAction, repoName, nextAllowed.Format("2006-01-02")),
		}
	}

	return throttleDecision{
		Skip:    false,
		Message: fmt.Sprintf("%s %s: throttle window elapsed (next allowed was %s).", syncAction, repoName, nextAllowed.Format("2006-01-02")),
	}
}

func updateRepoSyncState(repoName string, st *state.State) {
	if st == nil {
		return
	}
	now := time.Now()
	nextAllowed := now.Add(randomThrottleDuration())
	st.SetRepoSync(repoName, now, nextAllowed)
}

func randomThrottleDuration() time.Duration {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	days := throttleMinDays + rng.Intn(throttleMaxDays-throttleMinDays+1)
	return time.Duration(days) * 24 * time.Hour
}

func hasRecentLocalCommits(repoName string) (bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, fmt.Errorf("failed to resolve home directory: %w", err)
	}

	repoPath := filepath.Join(home, "git", repoName)
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
