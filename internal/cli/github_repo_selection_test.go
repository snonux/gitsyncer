package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/snonux/gitsyncer/internal/config"
	"github.com/snonux/gitsyncer/internal/forge"
)

// stubLookupLister is a public-repo lister that can also look up single
// repositories, like the real GitHub adapter.
type stubLookupLister struct {
	stubGitHubPublicRepoLister
	lookups map[string]forge.RepoLookup
	errs    map[string]error
	calls   []string
}

func (s *stubLookupLister) LookupRepo(name string) (forge.RepoLookup, error) {
	s.calls = append(s.calls, name)
	if err := s.errs[name]; err != nil {
		return forge.RepoLookup{}, err
	}
	return s.lookups[name], nil
}

// newExtrasLister models the real-world mix of configured extras: a fork
// that must be synced plus entries that are gone, renamed, archived,
// private or whose lookup fails.
func newExtrasLister() *stubLookupLister {
	return &stubLookupLister{
		stubGitHubPublicRepoLister: stubGitHubPublicRepoLister{
			hasToken: true,
			repos:    []forge.PublicRepo{{Name: "wanted"}, {Name: "not-configured"}},
		},
		lookups: map[string]forge.RepoLookup{
			"myfork":   {Exists: true, Name: "myfork"},
			"renamed":  {Exists: true, Name: "new-name"},
			"archived": {Exists: true, Name: "archived", Archived: true},
			"secret":   {Exists: true, Name: "secret", Private: true},
			// "gone" is absent: Exists == false.
		},
		errs: map[string]error{"flaky": errors.New("boom")},
	}
}

func TestHandleSyncGitHubPublicWithDeps_SyncAllGitHubReposAddsVerifiedExtras(t *testing.T) {
	lister := newExtrasLister()
	newLister := func(config.Organization) forge.PublicRepoLister { return lister }
	cfg := &config.Config{
		Organizations:      []config.Organization{{Host: "git@github.com", Name: "acme"}},
		Repositories:       []string{"wanted", "myfork", "gone", "renamed", "archived", "secret", "flaky", "myfork"},
		SyncAllGitHubRepos: true,
	}
	flags := &Flags{DryRun: true, WorkDir: t.TempDir()}

	output := captureStdout(t, func() {
		if got := handleSyncGitHubPublicWithDeps(cfg, flags, newLister, stubForgeClientResolver{}); got != 0 {
			t.Fatalf("handleSyncGitHubPublicWithDeps() = %d, want 0", got)
		}
	})

	// Discovered repos are all synced (no allowlist), plus the verified fork.
	for _, want := range []string{
		"  - not-configured",
		"  - myfork",
		"Added 1 configured repositories not found by GitHub discovery",
		"Would sync 3 repositories from GitHub",
		"Skipping configured repository gone: not found on GitHub",
		"Skipping configured repository renamed: renamed to new-name on GitHub",
		"Skipping configured repository archived: archived on GitHub (read-only)",
		"Skipping configured repository secret: private on GitHub",
		"Skipping configured repository flaky: GitHub lookup failed: boom",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "allowlist") {
		t.Errorf("allowlist applied with sync_all_github_repos:\n%s", output)
	}
	// "wanted" was discovered, so it must not be looked up again, and the
	// duplicate "myfork" entry must only be looked up once.
	if got := strings.Join(lister.calls, ","); got != "myfork,gone,renamed,archived,secret,flaky" {
		t.Errorf("lookups = %s", got)
	}
}

func TestExtraSkipReason_WithoutLookuperSkips(t *testing.T) {
	if got := extraSkipReason("anything", nil); got == "" {
		t.Fatal("extra without a lookuper must be skipped, got empty reason")
	}
}

func TestExtraSkipReason_RenameIsCaseInsensitive(t *testing.T) {
	lister := &stubLookupLister{lookups: map[string]forge.RepoLookup{
		"DTail": {Exists: true, Name: "dtail"},
	}}
	if got := extraSkipReason("DTail", lister); got != "" {
		t.Fatalf("case-only difference treated as rename: %q", got)
	}
}
