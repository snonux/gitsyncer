package cli

import (
	"strings"
	"testing"
	"time"

	"codeberg.org/snonux/gitsyncer/internal/state"
)

func TestEvaluateSyncPolicy_SkipsRepoSyncedWithinDay(t *testing.T) {
	st := &state.State{}
	st.SetLastRepoSync("repo", time.Now().Add(-23*time.Hour))

	decision := evaluateSyncPolicy("repo", st, false, false, false)

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

	decision := evaluateSyncPolicy("repo", st, false, false, false)

	if decision.Skip {
		t.Fatalf("expected repo synced more than 24 hours ago to proceed, got %q", decision.Message)
	}
}

func TestEvaluateSyncPolicy_ForceBypassesDailyAndThrottleLimits(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	st := &state.State{}
	now := time.Now()
	st.SetRepoSync("repo", now.Add(-1*time.Hour), now.Add(30*24*time.Hour))

	decision := evaluateSyncPolicy("repo", st, false, true, true)

	if decision.Skip {
		t.Fatalf("expected --force to bypass sync limits, got %q", decision.Message)
	}
	if decision.SetNextAllowed {
		t.Fatal("did not expect --force to request throttle-window persistence")
	}
}

func TestEvaluateSyncPolicy_ThrottleSetsWindowWhenRepoIsIdle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	start := time.Now()
	decision := evaluateSyncPolicy("repo", &state.State{}, false, false, true)
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

func TestRecordRepoSync_ClearsThrottleWindowWhenThrottleDisabled(t *testing.T) {
	st := &state.State{}
	st.SetRepoSync("repo", time.Now().Add(-72*time.Hour), time.Now().Add(72*time.Hour))

	recordRepoSync("repo", st, false)

	if st.GetLastRepoSync("repo").IsZero() {
		t.Fatal("expected last sync time to be recorded")
	}
	if !st.GetNextRepoSyncAllowed("repo").IsZero() {
		t.Fatal("expected throttle window to be cleared when throttle is disabled")
	}
}

func TestRecordRepoSync_SetsThrottleWindowWhenThrottleEnabled(t *testing.T) {
	st := &state.State{}

	recordRepoSync("repo", st, true)

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
