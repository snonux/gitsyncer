package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadBatchRunState_CorruptedStateFile is a regression test for a bug
// where a corrupted .gitsyncer-state.json caused state.Manager.Load() to
// return a nil *State alongside a non-nil error. The 'manage batch-run'
// command then called s.HasRunWithinWeek() on that nil pointer, panicking
// and crashing the weekly cron job. loadBatchRunState must default to a
// zero-value state instead, so batch-run can proceed as if it had never
// run before.
func TestLoadBatchRunState_CorruptedStateFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateFile := filepath.Join(dir, ".gitsyncer-state.json")
	if err := os.WriteFile(stateFile, []byte("{not valid json"), 0644); err != nil {
		t.Fatalf("failed to write corrupted state file: %v", err)
	}

	manager, s, err := loadBatchRunState(dir)

	if err == nil {
		t.Fatal("expected an error loading a corrupted state file, got nil")
	}
	if manager == nil {
		t.Fatal("expected a non-nil state manager even when Load() fails")
	}
	if s == nil {
		t.Fatal("expected loadBatchRunState to default to a zero-value state, got nil")
	}

	// This must not panic (that was the original bug), and a fresh state
	// must report that no batch run has happened yet.
	if s.HasRunWithinWeek() {
		t.Fatal("expected zero-value state to report HasRunWithinWeek() == false")
	}
}

// TestLoadBatchRunState_MissingStateFile covers the common "first ever run"
// case: no state file exists yet. Load() itself returns an empty state with
// no error, and loadBatchRunState should pass it through unchanged.
func TestLoadBatchRunState_MissingStateFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	manager, s, err := loadBatchRunState(dir)

	if err != nil {
		t.Fatalf("expected no error for a missing state file, got: %v", err)
	}
	if manager == nil {
		t.Fatal("expected a non-nil state manager")
	}
	if s == nil {
		t.Fatal("expected a non-nil state")
	}
	if s.HasRunWithinWeek() {
		t.Fatal("expected fresh state to report HasRunWithinWeek() == false")
	}
}
