package description

import (
	"os"
	"path/filepath"
	"testing"

	"codeberg.org/snonux/gitsyncer/internal/config"
)

func TestFileBackupDescriptionWriter_WritesDescription(t *testing.T) {
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

	supported, err := (FileBackupDescriptionWriter{}).WriteBackupDescription(org, "sample", "Sample description", false)
	if err != nil {
		t.Fatalf("WriteBackupDescription() error = %v", err)
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

func TestFileBackupDescriptionWriter_UnsupportedForNonFileHost(t *testing.T) {
	t.Parallel()

	org := &config.Organization{Host: "ssh://git@example.com/repos", BackupLocation: true}

	supported, err := (FileBackupDescriptionWriter{}).WriteBackupDescription(org, "sample", "Sample description", false)
	if err != nil {
		t.Fatalf("WriteBackupDescription() error = %v", err)
	}
	if supported {
		t.Fatal("expected file writer to report unsupported for a non-file:// host")
	}
}

func TestSSHBackupDescriptionWriter_WithoutDescriptionSyncConfigIsUnsupported(t *testing.T) {
	t.Parallel()

	org := &config.Organization{
		Host:           "ssh://git@example.com/repos",
		BackupLocation: true,
	}

	supported, err := (SSHBackupDescriptionWriter{}).WriteBackupDescription(org, "sample", "Sample description", false)
	if err != nil {
		t.Fatalf("WriteBackupDescription() error = %v", err)
	}
	if supported {
		t.Fatal("expected SSH backup description sync without config to be unsupported")
	}
}

func TestForgejoBackupDescriptionWriter_UnsupportedForNonForgejoOrg(t *testing.T) {
	t.Parallel()

	org := &config.Organization{Host: "ssh://git@example.com/repos", BackupLocation: true}
	writer := ForgejoBackupDescriptionWriter{NewClient: func(*config.Organization) ForgejoDescriptionClient {
		t.Fatal("NewClient should not be called for a non-Forgejo org")
		return nil
	}}

	supported, err := writer.WriteBackupDescription(org, "sample", "Sample description", false)
	if err != nil {
		t.Fatalf("WriteBackupDescription() error = %v", err)
	}
	if supported {
		t.Fatal("expected Forgejo writer to report unsupported for a non-Forgejo org")
	}
}

func TestForgejoBackupDescriptionWriter_MissingTokenReturnsError(t *testing.T) {
	t.Parallel()

	org := &config.Organization{
		Host:           "ssh://git@forgejo.example:2022",
		ForgejoAPIBase: "https://forgejo.example",
		ForgejoOwner:   "owner",
		BackupLocation: true,
	}
	writer := ForgejoBackupDescriptionWriter{NewClient: func(*config.Organization) ForgejoDescriptionClient {
		return &fakeForgejoDescriptionClient{hasToken: false}
	}}

	supported, err := writer.WriteBackupDescription(org, "sample", "Sample description", false)
	if !supported {
		t.Fatal("expected Forgejo writer to report supported even when the token is missing")
	}
	if err == nil {
		t.Fatal("expected an error when no Forgejo token is configured")
	}
}
