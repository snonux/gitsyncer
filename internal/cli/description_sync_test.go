package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"codeberg.org/snonux/gitsyncer/internal/config"
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

	syncRepoDescriptions(cfg, false, nil, "demo", "canonical", "", nil)
	if mutations != 0 {
		t.Fatalf("inactive backup issued %d Forgejo API mutations, want zero", mutations)
	}
	syncRepoDescriptions(cfg, true, func(*config.Organization) bool { return true }, "demo", "canonical", "", nil)
	if mutations != 0 {
		t.Fatalf("dry run issued %d Forgejo API mutations, want zero", mutations)
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
