package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"codeberg.org/snonux/gitsyncer/internal/config"
	syncpkg "codeberg.org/snonux/gitsyncer/internal/sync"
)

func TestSyncRepoDescriptions_ForgejoWritesOnlyForActiveNonDryRunBackup(t *testing.T) {
	var mutations int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost || r.Method == http.MethodPatch {
			mutations++
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	t.Setenv("FORGEJO_TOKEN", "secret")
	cfg := &config.Config{Organizations: []config.Organization{{
		Host: "ssh://git@forgejo.example:2022", ForgejoAPIBase: server.URL, ForgejoOwner: "owner", BackupLocation: true,
	}}}

	syncRepoDescriptions(cfg, false, nil, nil, "demo", "canonical", "", nil)
	if mutations != 0 {
		t.Fatalf("inactive backup issued %d Forgejo API mutations, want zero", mutations)
	}
	syncRepoDescriptions(cfg, true, func(*config.Organization) bool { return true }, nil, "demo", "canonical", "", nil)
	if mutations != 0 {
		t.Fatalf("dry run issued %d Forgejo API mutations, want zero", mutations)
	}
}

func TestSyncRepoDescriptions_ForgejoFailureDisablesDestinationAcrossRepos(t *testing.T) {
	failedMutations := 0
	failedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			failedMutations++
		}
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer failedServer.Close()

	continuingMutations := 0
	continuingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			continuingMutations++
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer continuingServer.Close()

	t.Setenv("FORGEJO_TOKEN", "secret")
	cfg := &config.Config{Organizations: []config.Organization{
		{Host: "ssh://git@failed.example:2022", ForgejoAPIBase: failedServer.URL, ForgejoOwner: "owner", BackupLocation: true},
		{Host: "ssh://git@continuing.example:2022", ForgejoAPIBase: continuingServer.URL, ForgejoOwner: "owner", BackupLocation: true},
	}}
	syncer := syncpkg.New(cfg, t.TempDir())
	syncer.SetBackupEnabled(true)

	for _, repo := range []string{"one", "two"} {
		syncRepoDescriptions(cfg, false, syncer.BackupActive, syncer.DisableBackup, repo, "canonical", "", nil)
	}

	if failedMutations != 1 {
		t.Fatalf("failed Forgejo received %d PATCH requests across two repos, want one", failedMutations)
	}
	if continuingMutations != 2 {
		t.Fatalf("healthy Forgejo received %d PATCH requests across two repos, want two", continuingMutations)
	}
	if syncer.BackupActive(&cfg.Organizations[0]) {
		t.Fatal("failed Forgejo remained active for subsequent API and push work")
	}
	if !syncer.BackupActive(&cfg.Organizations[1]) {
		t.Fatal("healthy Forgejo was disabled by another destination's failure")
	}
}

func TestSyncBackupDescription_FileURLWritesDescription(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	repoDir := filepath.Join(rootDir, "sample.git")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatalf("mkdir repo dir: %v", err)
	}

	org := &config.Organization{
		Host:           "file://" + rootDir,
		BackupLocation: true,
	}

	supported, err := syncBackupDescription(org, "sample", "Sample description", false)
	if err != nil {
		t.Fatalf("syncBackupDescription() error = %v", err)
	}
	if !supported {
		t.Fatal("expected file backup description sync to be supported")
	}

	content, err := os.ReadFile(filepath.Join(repoDir, "description"))
	if err != nil {
		t.Fatalf("read description: %v", err)
	}
	if string(content) != "Sample description\n" {
		t.Fatalf("description = %q, want %q", string(content), "Sample description\n")
	}
}

func TestSyncBackupDescription_SSHWithoutDescriptionSyncConfigIsUnsupported(t *testing.T) {
	t.Parallel()

	org := &config.Organization{
		Host:           "ssh://git@example.com/repos",
		BackupLocation: true,
	}

	supported, err := syncBackupDescription(org, "sample", "Sample description", false)
	if err != nil {
		t.Fatalf("syncBackupDescription() error = %v", err)
	}
	if supported {
		t.Fatal("expected SSH backup description sync without config to be unsupported")
	}
}
