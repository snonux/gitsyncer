package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/snonux/gitsyncer/internal/config"
	"github.com/snonux/gitsyncer/internal/state"
)

func TestShouldEnableBackupSync_FullSyncImplicitlyEnablesBackup(t *testing.T) {
	t.Parallel()

	if !shouldEnableBackupSync(&Flags{FullSync: true}) {
		t.Fatal("expected full sync to enable backup sync implicitly")
	}

	if !shouldEnableBackupSync(&Flags{Backup: true}) {
		t.Fatal("expected explicit --backup to enable backup sync")
	}
	if !shouldEnableBackupSync(&Flags{SyncCodebergPublic: true}) {
		t.Fatal("expected codeberg-to-github sync to enable backups")
	}
	if !shouldEnableBackupSync(&Flags{SyncGitHubPublic: true}) {
		t.Fatal("expected github-to-codeberg sync to enable backups")
	}

	if shouldEnableBackupSync(&Flags{}) {
		t.Fatal("did not expect backup sync to be enabled by default")
	}
}

// TestSyncDiscoveredRepos_ContinuesAfterPerRepoFailure locks in the
// continue-on-partial-failure behavior (task y01): a config with no
// organizations makes every SyncRepository call fail fast and
// deterministically (no network involved - see setupNewRepository's "no
// organizations configured" error), which previously made syncDiscoveredRepos
// abort after the first repo. It should now attempt every repo and report
// all the failures at the end instead.
func TestSyncDiscoveredRepos_ContinuesAfterPerRepoFailure(t *testing.T) {
	// Not t.Parallel(): captureStdout swaps the global os.Stdout (see comment
	// on TestSyncCodebergRepos_PrintsSeparatorWhenChainingIntoGitHubSync in
	// forge_client_injection_test.go).

	resolver := &stubForgeClientResolver{}
	flags := &Flags{WorkDir: t.TempDir()}
	repoNames := []string{"repo-a", "repo-b"}

	var got int
	out := captureStdout(t, func() {
		got = syncGitHubRepos(&config.Config{}, flags, nil, repoNames, resolver)
	})

	if got != 1 {
		t.Fatalf("syncGitHubRepos() = %d, want 1 (some repos failed)", got)
	}
	for _, repo := range repoNames {
		if !strings.Contains(out, "Syncing "+repo) {
			t.Fatalf("expected %s to be attempted despite an earlier repo failing, got:\n%s", repo, out)
		}
		if !strings.Contains(out, "ERROR: Failed to sync "+repo) {
			t.Fatalf("expected a recorded failure for %s, got:\n%s", repo, out)
		}
	}
	if !strings.Contains(out, "Failed to sync: 2 repositories") {
		t.Fatalf("expected a failure summary listing 2 repositories, got:\n%s", out)
	}
}

// TestHandleSyncAll_ContinuesAfterPerRepoFailure is the same regression as
// TestSyncDiscoveredRepos_ContinuesAfterPerRepoFailure above, but for the
// statically-configured-repos path (HandleSyncAll) rather than the
// discovered-repos path (syncDiscoveredRepos) - the two used to have
// independent "Stopping sync due to error" abort points.
func TestHandleSyncAll_ContinuesAfterPerRepoFailure(t *testing.T) {
	// Not t.Parallel(): see comment above.

	flags := &Flags{WorkDir: t.TempDir()}
	cfg := &config.Config{Repositories: []string{"repo-a", "repo-b"}}

	var got int
	out := captureStdout(t, func() {
		got = HandleSyncAll(cfg, flags)
	})

	if got != 1 {
		t.Fatalf("HandleSyncAll() = %d, want 1 (some repos failed)", got)
	}
	for _, repo := range cfg.Repositories {
		if !strings.Contains(out, "Syncing "+repo) {
			t.Fatalf("expected %s to be attempted despite an earlier repo failing, got:\n%s", repo, out)
		}
	}
	if !strings.Contains(out, "Failed to sync: 2 repositories") {
		t.Fatalf("expected a failure summary listing 2 repositories, got:\n%s", out)
	}
	if strings.Contains(out, "Successfully synced all") {
		t.Fatalf("did not expect the all-succeeded message when repos failed, got:\n%s", out)
	}
}

// TestHandleSync_ExplicitRepoIgnoresDailyInterval is a regression for the
// case where `gitsyncer sync repo NAME` skipped because the repo had been
// synced within 24 hours. Naming a repo is an explicit request to sync it
// now, so the daily interval (and --throttle) must not apply.
func TestHandleSync_ExplicitRepoIgnoresDailyInterval(t *testing.T) {
	workDir := t.TempDir()
	st := &state.State{}
	st.SetLastRepoSync("tasksamurai", time.Now())
	if err := state.NewManager(workDir).Save(st); err != nil {
		t.Fatalf("save state: %v", err)
	}

	var got int
	out := captureStdout(t, func() {
		got = HandleSync(&config.Config{}, &Flags{
			WorkDir:  workDir,
			SyncRepo: "tasksamurai",
		})
	})

	if strings.Contains(out, "Skipping tasksamurai") {
		t.Fatalf("explicit sync repo skipped by daily interval, got:\n%s", out)
	}
	if got != 1 {
		t.Fatalf("HandleSync() = %d, want 1 (sync attempted and failed with no orgs)", got)
	}
}
