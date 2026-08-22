package sync

import (
	"testing"

	"github.com/snonux/gitsyncer/internal/config"
)

func TestSyncer_CodebergActiveForRepo(t *testing.T) {
	t.Parallel()

	orgs := []config.Organization{
		{Host: "git@github.com", Name: "snonux"},
		{Host: "git@codeberg.org", Name: "snonux"},
	}

	// sync_codeberg disabled: Codeberg is never active, regardless of allowlist.
	disabled := &Syncer{config: &config.Config{Organizations: orgs, Repositories: []string{"hypr"}}}
	disabled.repoName = "hypr"
	if disabled.codebergActiveForRepo() {
		t.Fatal("codebergActiveForRepo() = true when sync_codeberg disabled")
	}

	// sync_codeberg enabled and repo in allowlist: active.
	allowed := &Syncer{config: &config.Config{Organizations: orgs, Repositories: []string{"hypr"}, SyncCodeberg: true}}
	allowed.repoName = "hypr"
	if !allowed.codebergActiveForRepo() {
		t.Fatal("codebergActiveForRepo() = false for allowlisted repo with sync_codeberg enabled")
	}

	// sync_codeberg enabled but repo NOT in allowlist: not active.
	notAllowed := &Syncer{config: &config.Config{Organizations: orgs, Repositories: []string{"other"}, SyncCodeberg: true}}
	notAllowed.repoName = "hypr"
	if notAllowed.codebergActiveForRepo() {
		t.Fatal("codebergActiveForRepo() = true for repo not in allowlist")
	}

	// Discovery mode (empty allowlist) with sync_codeberg enabled: active for any repo.
	discovery := &Syncer{config: &config.Config{Organizations: orgs, SyncCodeberg: true}}
	discovery.repoName = "anything"
	if !discovery.codebergActiveForRepo() {
		t.Fatal("codebergActiveForRepo() = false in discovery mode")
	}
}

func TestSyncer_SyncOrgs_ExcludesCodebergForNonAllowlistedRepo(t *testing.T) {
	t.Parallel()

	orgs := []config.Organization{
		{Host: "git@github.com", Name: "snonux"},
		{Host: "git@codeberg.org", Name: "snonux"},
	}

	// hypr is not in the allowlist: syncOrgs must drop the Codeberg org.
	s := &Syncer{config: &config.Config{Organizations: orgs, Repositories: []string{"cpuinfo"}, SyncCodeberg: true}}
	s.repoName = "hypr"
	got := s.syncOrgs()
	if len(got) != 1 {
		t.Fatalf("syncOrgs() = %d orgs, want 1 (Codeberg excluded)", len(got))
	}
	for _, o := range got {
		if o.IsCodeberg() {
			t.Fatalf("syncOrgs() included Codeberg org for non-allowlisted repo")
		}
	}
	// Disabled remote names must include the Codeberg remote so a leftover
	// codeberg remote is not fetched.
	if !s.disabledRemoteNames()["codeberg_org"] {
		t.Fatalf("disabledRemoteNames() did not include codeberg remote for non-allowlisted repo")
	}

	// cpuinfo IS in the allowlist: syncOrgs keeps the Codeberg org.
	s.repoName = "cpuinfo"
	got = s.syncOrgs()
	if len(got) != 2 {
		t.Fatalf("syncOrgs() = %d orgs, want 2 (Codeberg included) for allowlisted repo", len(got))
	}
	if s.disabledRemoteNames()["codeberg_org"] {
		t.Fatalf("disabledRemoteNames() included codeberg remote for allowlisted repo")
	}
}
