package cli

// The sync-scheduling policy (daily interval + --throttle window based on
// recent local commits) previously lived in this file. Task w01 moved the
// actual decision logic to internal/sync/policy.go, since it is a domain
// rule about repository/branch state, not CLI flag handling; this file is
// now just a re-export so existing call sites in this package keep working
// as a thin flag-parsing -> function-call layer.
import (
	"github.com/snonux/gitsyncer/internal/state"
	"github.com/snonux/gitsyncer/internal/sync"
)

// syncDecision is an alias for sync.SyncDecision so call sites in this
// package do not need to change their type references.
type syncDecision = sync.SyncDecision

func loadSyncState(workDir string) (*state.Manager, *state.State, error) {
	return sync.LoadSyncState(workDir)
}

func evaluateSyncPolicy(repoName, workDir string, st *state.State, dryRun bool, force bool, throttle bool) syncDecision {
	return sync.EvaluateSyncPolicy(repoName, workDir, st, dryRun, force, throttle)
}

func recordRepoSync(repoName string, st *state.State, throttle bool) {
	sync.RecordRepoSync(repoName, st, throttle)
}
