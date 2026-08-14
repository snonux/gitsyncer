package description

import (
	"errors"
	"testing"

	"codeberg.org/snonux/gitsyncer/internal/config"
	"codeberg.org/snonux/gitsyncer/internal/forge"
	syncpkg "codeberg.org/snonux/gitsyncer/internal/sync"
)

// fakeForgeClientResolver stands in for internal/cli's ForgeClientResolver.
// The tests below never configure a GitHub or Codeberg organization, so
// ClientFor is never actually invoked; it exists only to satisfy the
// ForgeClientResolver parameter.
type fakeForgeClientResolver struct{}

func (fakeForgeClientResolver) ClientFor(*config.Organization) (forge.RepoDescriptionClient, bool) {
	return nil, false
}

// fakeForgejoDescriptionClient is a test double for ForgejoDescriptionClient,
// replacing the httptest.Server + real codeberg.NewForgejoClient the
// pre-move cli-level test used - the HTTP transport is codeberg's concern,
// not this package's; here we only need to verify the orchestration
// (backupActive gating, dry-run suppression, disable-on-failure) around
// whatever client the injected factory returns.
type fakeForgejoDescriptionClient struct {
	hasToken bool
	updates  []descriptionUpdate
	err      error
}

type descriptionUpdate struct {
	repoName    string
	description string
}

func (f *fakeForgejoDescriptionClient) HasToken() bool { return f.hasToken }

func (f *fakeForgejoDescriptionClient) UpdateRepoDescription(repoName, description string) error {
	f.updates = append(f.updates, descriptionUpdate{repoName: repoName, description: description})
	return f.err
}

func TestSyncRepoDescriptions_ForgejoWritesOnlyForActiveNonDryRunBackup(t *testing.T) {
	client := &fakeForgejoDescriptionClient{hasToken: true}
	writers := []BackupDescriptionWriter{
		ForgejoBackupDescriptionWriter{NewClient: func(*config.Organization) ForgejoDescriptionClient { return client }},
	}
	cfg := &config.Config{Organizations: []config.Organization{{
		Host: "ssh://git@forgejo.example:2022", ForgejoAPIBase: "https://forgejo.example", ForgejoOwner: "owner", BackupLocation: true,
	}}}

	SyncRepoDescriptions(cfg, false, nil, nil, "demo", "canonical", "", nil, fakeForgeClientResolver{}, writers)
	if len(client.updates) != 0 {
		t.Fatalf("inactive backup issued %d Forgejo updates, want zero", len(client.updates))
	}
	SyncRepoDescriptions(cfg, true, func(*config.Organization) bool { return true }, nil, "demo", "canonical", "", nil, fakeForgeClientResolver{}, writers)
	if len(client.updates) != 0 {
		t.Fatalf("dry run issued %d Forgejo updates, want zero", len(client.updates))
	}
}

func TestSyncRepoDescriptions_ForgejoFailureDisablesDestinationAcrossRepos(t *testing.T) {
	failedClient := &fakeForgejoDescriptionClient{hasToken: true, err: errors.New("unavailable")}
	continuingClient := &fakeForgejoDescriptionClient{hasToken: true}

	clients := map[string]*fakeForgejoDescriptionClient{
		"ssh://git@failed.example:2022":     failedClient,
		"ssh://git@continuing.example:2022": continuingClient,
	}
	writers := []BackupDescriptionWriter{
		ForgejoBackupDescriptionWriter{NewClient: func(org *config.Organization) ForgejoDescriptionClient {
			return clients[org.Host]
		}},
	}

	cfg := &config.Config{Organizations: []config.Organization{
		{Host: "ssh://git@failed.example:2022", ForgejoAPIBase: "https://failed.example", ForgejoOwner: "owner", BackupLocation: true},
		{Host: "ssh://git@continuing.example:2022", ForgejoAPIBase: "https://continuing.example", ForgejoOwner: "owner", BackupLocation: true},
	}}
	syncer := syncpkg.New(cfg, t.TempDir())
	syncer.SetBackupEnabled(true)

	for _, repo := range []string{"one", "two"} {
		SyncRepoDescriptions(cfg, false, syncer.BackupActive, syncer.DisableBackup, repo, "canonical", "", nil, fakeForgeClientResolver{}, writers)
	}

	if len(failedClient.updates) != 1 {
		t.Fatalf("failed Forgejo received %d update calls across two repos, want one", len(failedClient.updates))
	}
	if len(continuingClient.updates) != 2 {
		t.Fatalf("healthy Forgejo received %d update calls across two repos, want two", len(continuingClient.updates))
	}
	if syncer.BackupActive(&cfg.Organizations[0]) {
		t.Fatal("failed Forgejo remained active for subsequent API and push work")
	}
	if !syncer.BackupActive(&cfg.Organizations[1]) {
		t.Fatal("healthy Forgejo was disabled by another destination's failure")
	}
}
