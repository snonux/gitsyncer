package cli

import "testing"

func TestShouldEnableBackupSync_FullSyncImplicitlyEnablesBackup(t *testing.T) {
	t.Parallel()

	if !shouldEnableBackupSync(&Flags{FullSync: true}) {
		t.Fatal("expected full sync to enable backup sync implicitly")
	}

	if !shouldEnableBackupSync(&Flags{Backup: true}) {
		t.Fatal("expected explicit --backup to enable backup sync")
	}

	if shouldEnableBackupSync(&Flags{}) {
		t.Fatal("did not expect backup sync to be enabled by default")
	}
}
