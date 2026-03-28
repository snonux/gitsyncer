package sync

import (
	"errors"
	"testing"

	"codeberg.org/snonux/gitsyncer/internal/config"
)

func TestHandlePushError_DisablesBackupForSession(t *testing.T) {
	resetBackupSessionState()
	t.Cleanup(resetBackupSessionState)

	syncer := &Syncer{}
	syncer.SetBackupEnabled(true)

	err := syncer.handlePushError("backup", &config.Organization{BackupLocation: true}, errors.New("dial tcp: connection refused"))
	if err != nil {
		t.Fatalf("expected backup push failure to be downgraded, got %v", err)
	}
	if syncer.backupActive() {
		t.Fatal("expected backup sync to be disabled for the remainder of the session")
	}
}

func TestHandlePushError_PropagatesPrimaryRemoteFailure(t *testing.T) {
	resetBackupSessionState()
	t.Cleanup(resetBackupSessionState)

	syncer := &Syncer{}
	syncer.SetBackupEnabled(true)

	pushErr := errors.New("push rejected")
	err := syncer.handlePushError("origin", &config.Organization{}, pushErr)
	if !errors.Is(err, pushErr) {
		t.Fatalf("expected primary remote error to be returned, got %v", err)
	}
}

func TestParseSSHLocation_SupportsSSHURLWithPort(t *testing.T) {
	t.Parallel()

	userHost, sshArgs, basePath, err := parseSSHLocation("ssh://git@r0:30022/repos")
	if err != nil {
		t.Fatalf("parseSSHLocation() error = %v", err)
	}
	if userHost != "git@r0" {
		t.Fatalf("userHost = %q, want %q", userHost, "git@r0")
	}
	if basePath != "/repos" {
		t.Fatalf("basePath = %q, want %q", basePath, "/repos")
	}

	wantArgs := []string{"-p", "30022", "git@r0"}
	if len(sshArgs) != len(wantArgs) {
		t.Fatalf("sshArgs = %#v, want %#v", sshArgs, wantArgs)
	}
	for i := range wantArgs {
		if sshArgs[i] != wantArgs[i] {
			t.Fatalf("sshArgs = %#v, want %#v", sshArgs, wantArgs)
		}
	}
}
