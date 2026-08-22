package sync

import (
	"errors"
	"fmt"
	stdsync "sync"
	"sync/atomic"
	"testing"

	"github.com/snonux/gitsyncer/internal/config"
	"github.com/snonux/gitsyncer/internal/forge"
)

// fakePublicRepoEnsurer is a test double for forge.PublicRepoEnsurer. Since
// ensureForgejoBackups now depends on that interface (injected via
// SetForgejoBackupClientFactory) instead of constructing a concrete
// codeberg.Client, tests can exercise the call/error paths without an HTTP
// server standing in for a real Forgejo API.
type fakePublicRepoEnsurer struct {
	calls []fakeEnsureCall
	err   error
}

type fakeEnsureCall struct {
	name        string
	description string
}

func (f *fakePublicRepoEnsurer) EnsurePublicRepo(name, description string) error {
	f.calls = append(f.calls, fakeEnsureCall{name: name, description: description})
	return f.err
}

func forgejoBackupOrgConfig() *config.Config {
	return &config.Config{Organizations: []config.Organization{{
		Host: "ssh://git@forgejo.example:2022", ForgejoAPIBase: "https://forgejo.example", ForgejoOwner: "owner", BackupLocation: true,
	}}}
}

func TestEnsureForgejoBackups_DryRunDoesNotInvokeFactory(t *testing.T) {
	fake := &fakePublicRepoEnsurer{}
	cfg := forgejoBackupOrgConfig()
	syncer := New(cfg, t.TempDir())
	syncer.SetBackupEnabled(true)
	syncer.SetDryRun(true)
	syncer.SetForgejoBackupClientFactory(func(*config.Organization) forge.PublicRepoEnsurer { return fake })
	syncer.ensureForgejoBackups("demo")
	if len(fake.calls) != 0 {
		t.Fatalf("dry run invoked EnsurePublicRepo %d times, want zero", len(fake.calls))
	}
}

func TestEnsureForgejoBackups_NilFactoryIsNoOp(t *testing.T) {
	cfg := forgejoBackupOrgConfig()
	syncer := New(cfg, t.TempDir())
	syncer.SetBackupEnabled(true)
	// No factory injected: a Syncer built without cli's wiring must not panic.
	syncer.ensureForgejoBackups("demo")
}

func TestEnsureForgejoBackups_CallsInjectedFactory(t *testing.T) {
	fake := &fakePublicRepoEnsurer{}
	cfg := forgejoBackupOrgConfig()
	syncer := New(cfg, t.TempDir())
	syncer.SetBackupEnabled(true)
	syncer.SetForgejoBackupClientFactory(func(*config.Organization) forge.PublicRepoEnsurer { return fake })
	syncer.ensureForgejoBackups("demo")
	if len(fake.calls) != 1 {
		t.Fatalf("expected exactly one EnsurePublicRepo call, got %d", len(fake.calls))
	}
	if got := fake.calls[0]; got.name != "demo" || got.description != "Mirror of demo" {
		t.Fatalf("EnsurePublicRepo called with %+v, want {name:demo description:Mirror of demo}", got)
	}
}

func TestEnsureForgejoBackups_FactoryErrorDisablesBackupForSession(t *testing.T) {
	fake := &fakePublicRepoEnsurer{err: errors.New("boom")}
	cfg := forgejoBackupOrgConfig()
	syncer := New(cfg, t.TempDir())
	syncer.SetBackupEnabled(true)
	syncer.SetForgejoBackupClientFactory(func(*config.Organization) forge.PublicRepoEnsurer { return fake })
	syncer.ensureForgejoBackups("demo")
	if syncer.backupActive(syncer.getRemoteName(&cfg.Organizations[0])) {
		t.Fatal("expected backup sync to be disabled after EnsurePublicRepo error")
	}
}

func TestPushToAllRemotes_DryRunDoesNotInvokeGitPush(t *testing.T) {
	syncer := New(&config.Config{}, t.TempDir())
	syncer.SetDryRun(true)
	remotes := map[string]*config.Organization{"origin": {Host: "git@example.invalid", Name: "owner"}}
	if err := syncer.pushToAllRemotes("/path/that/does/not/exist", "main", remotes, nil); err != nil {
		t.Fatalf("pushToAllRemotes() error = %v; dry run must not execute git push", err)
	}
}

func TestHandlePushError_DisablesBackupForSession(t *testing.T) {
	syncer := &Syncer{}
	syncer.SetBackupEnabled(true)

	err := syncer.handlePushError("backup", &config.Organization{BackupLocation: true}, errors.New("dial tcp: connection refused"))
	if err != nil {
		t.Fatalf("expected backup push failure to be downgraded, got %v", err)
	}
	if syncer.backupActive("backup") {
		t.Fatal("expected backup sync to be disabled for the remainder of the session")
	}
	if !syncer.backupActive("other-backup") {
		t.Fatal("expected another backup remote to remain active")
	}
}

func TestHandlePushError_PropagatesPrimaryRemoteFailure(t *testing.T) {
	syncer := &Syncer{}
	syncer.SetBackupEnabled(true)

	pushErr := errors.New("push rejected")
	err := syncer.handlePushError("origin", &config.Organization{}, pushErr)
	if !errors.Is(err, pushErr) {
		t.Fatalf("expected primary remote error to be returned, got %v", err)
	}
}

func TestHandlePushError_BackupDisableIsIsolatedPerSyncer(t *testing.T) {
	backupOrg := &config.Organization{BackupLocation: true}

	syncerA := &Syncer{}
	syncerA.SetBackupEnabled(true)

	syncerB := &Syncer{}
	syncerB.SetBackupEnabled(true)

	err := syncerA.handlePushError("backup-a", backupOrg, errors.New("dial tcp: connection refused"))
	if err != nil {
		t.Fatalf("expected backup push failure to be downgraded, got %v", err)
	}

	if syncerA.backupActive("backup-a") {
		t.Fatal("expected syncerA backup sync to be disabled for the remainder of the session")
	}
	if !syncerB.backupActive("backup-a") {
		t.Fatal("expected syncerB backup session to remain active")
	}
}

func TestBackupSessionState_DisableIsThreadSafe(t *testing.T) {
	var session backupSessionState
	var firstDisableCount atomic.Int32

	const workers = 32
	var wg stdsync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func(i int) {
			defer wg.Done()
			if session.disable("backup", fmt.Sprintf("reason-%d", i)) {
				firstDisableCount.Add(1)
			}
		}(i)
	}

	wg.Wait()

	if got := firstDisableCount.Load(); got != 1 {
		t.Fatalf("expected exactly one successful disable transition, got %d", got)
	}

	disabled, reason := session.status("backup")
	if !disabled {
		t.Fatal("expected backup session to be disabled")
	}
	if reason == "" {
		t.Fatal("expected disable reason to be recorded")
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

func TestBareRepoInitCommand_UsesSharedGroupMode(t *testing.T) {
	t.Parallel()

	got := bareRepoInitCommand("/data/nfs/k3svolumes/git-server/repos/example.git")
	want := `mkdir -p "/data/nfs/k3svolumes/git-server/repos/example.git" && cd "/data/nfs/k3svolumes/git-server/repos/example.git" && git init --bare --shared=group`
	if got != want {
		t.Fatalf("bareRepoInitCommand() = %q, want %q", got, want)
	}
}

func TestRepositoryCreationLocation_UsesDescriptionSyncShellAccess(t *testing.T) {
	t.Parallel()

	org := &config.Organization{
		Host:                "ssh://git@r0:30022/repos",
		DescriptionSyncHost: "root@r0",
		DescriptionSyncRoot: "/srv/git/repos",
	}

	userHost, sshArgs, basePath, err := repositoryCreationLocation(org)
	if err != nil {
		t.Fatalf("repositoryCreationLocation() error = %v", err)
	}
	if userHost != "root@r0" {
		t.Fatalf("userHost = %q, want %q", userHost, "root@r0")
	}
	if len(sshArgs) != 1 || sshArgs[0] != "root@r0" {
		t.Fatalf("sshArgs = %#v, want %#v", sshArgs, []string{"root@r0"})
	}
	if basePath != "/srv/git/repos" {
		t.Fatalf("basePath = %q, want %q", basePath, "/srv/git/repos")
	}
}
