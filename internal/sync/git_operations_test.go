package sync

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"codeberg.org/snonux/gitsyncer/internal/config"
)

func TestGitCommand_SetsDir(t *testing.T) {
	cmd := gitCommand("/tmp/example-repo", "status")

	if cmd.Dir != "/tmp/example-repo" {
		t.Fatalf("expected command dir to be set, got %q", cmd.Dir)
	}
}

func TestGitCommand_LeavesDirEmptyForGlobalCommands(t *testing.T) {
	cmd := gitCommand("", "ls-remote", "--tags", "origin", "v1.0.0")

	if cmd.Dir != "" {
		t.Fatalf("expected empty dir for global command, got %q", cmd.Dir)
	}
}

func TestPushBranchArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setUpstream bool
		org         *config.Organization
		want        []string
	}{
		{
			name: "regular backup push",
			org:  &config.Organization{BackupLocation: true},
			want: []string{"push", "backup", "main", "--tags"},
		},
		{
			name: "forced backup push",
			org:  &config.Organization{BackupLocation: true, ForcePush: true},
			want: []string{"push", "backup", "main", "--tags", "--force"},
		},
		{
			name:        "forced backup push with upstream",
			setUpstream: true,
			org:         &config.Organization{BackupLocation: true, ForcePush: true},
			want:        []string{"push", "-u", "backup", "main", "--tags", "--force"},
		},
		{
			name: "force ignored for primary remote",
			org:  &config.Organization{ForcePush: true},
			want: []string{"push", "backup", "main", "--tags"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := pushBranchArgs("backup", "main", tt.setUpstream, tt.org)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("pushBranchArgs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestParseTagHashOutput_EmptyOutput(t *testing.T) {
	for _, in := range [][]byte{nil, []byte(""), []byte("   \n\t  \n")} {
		hash, err := parseTagHashOutput(in, "v1.0.0", "origin")
		if err == nil {
			t.Fatalf("expected error for empty output %q, got hash %q", in, hash)
		}
		if hash != "" {
			t.Fatalf("expected empty hash for empty output, got %q", hash)
		}
		if !strings.Contains(err.Error(), "v1.0.0") || !strings.Contains(err.Error(), "origin") {
			t.Fatalf("expected error to mention tag and source, got %v", err)
		}
	}
}

func TestParseTagHashOutput_ReturnsFirstField(t *testing.T) {
	hash, err := parseTagHashOutput([]byte("abc123\trefs/tags/v1.0.0\n"), "v1.0.0", "origin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash != "abc123" {
		t.Fatalf("expected hash %q, got %q", "abc123", hash)
	}
}

func TestGetTagCommitHash_LocalTagPeelsToCommitHash(t *testing.T) {
	repoPath := t.TempDir()

	runGit(t, repoPath, "init")
	runGit(t, repoPath, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "initial commit")
	runGit(t, repoPath, "tag", "-a", "v1.0.0", "-m", "release v1.0.0")

	headHash := runGit(t, repoPath, "rev-parse", "HEAD")
	tagHash, err := getTagCommitHash(repoPath, "v1.0.0", "local")
	if err != nil {
		t.Fatalf("expected local tag hash lookup to succeed, got error: %v", err)
	}
	if tagHash != headHash {
		t.Fatalf("expected peeled local tag hash %q, got %q", headHash, tagHash)
	}
}

func TestGetTagCommitHash_LocalTagMissingReturnsError(t *testing.T) {
	repoPath := t.TempDir()
	runGit(t, repoPath, "init")

	tagHash, err := getTagCommitHash(repoPath, "v9.9.9", "local")
	if err == nil {
		t.Fatalf("expected error for missing local tag, got hash %q", tagHash)
	}
	if tagHash != "" {
		t.Fatalf("expected empty hash for missing local tag, got %q", tagHash)
	}
}

func TestPopStash_NoStashReturnsError(t *testing.T) {
	repoPath := t.TempDir()
	runGit(t, repoPath, "init")

	if err := popStash(repoPath); err == nil {
		t.Fatal("expected error when popping stash with no entries")
	}
}

func TestPopStash_RestoresStashedChanges(t *testing.T) {
	repoPath := t.TempDir()

	runGit(t, repoPath, "init")
	runGit(t, repoPath, "config", "user.name", "Test User")
	runGit(t, repoPath, "config", "user.email", "test@example.com")

	trackedFile := filepath.Join(repoPath, "tracked.txt")
	if err := os.WriteFile(trackedFile, []byte("first\n"), 0o644); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}
	runGit(t, repoPath, "add", "tracked.txt")
	runGit(t, repoPath, "commit", "-m", "initial")

	if err := os.WriteFile(trackedFile, []byte("second\n"), 0o644); err != nil {
		t.Fatalf("update tracked file: %v", err)
	}
	runGit(t, repoPath, "stash", "push", "-m", "test-stash")

	if err := popStash(repoPath); err != nil {
		t.Fatalf("expected popStash to succeed, got error: %v", err)
	}

	content, err := os.ReadFile(trackedFile)
	if err != nil {
		t.Fatalf("read tracked file: %v", err)
	}
	if string(content) != "second\n" {
		t.Fatalf("expected stashed content to be restored, got %q", string(content))
	}
}

func TestStashChanges_NoStashEntryReturnsFalse(t *testing.T) {
	repoPath := t.TempDir()

	runGit(t, repoPath, "init")
	runGit(t, repoPath, "config", "user.name", "Test User")
	runGit(t, repoPath, "config", "user.email", "test@example.com")

	trackedFile := filepath.Join(repoPath, "tracked.txt")
	if err := os.WriteFile(trackedFile, []byte("committed\n"), 0o644); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}
	runGit(t, repoPath, "add", "tracked.txt")
	runGit(t, repoPath, "commit", "-m", "initial")

	untrackedFile := filepath.Join(repoPath, "untracked.txt")
	if err := os.WriteFile(untrackedFile, []byte("new\n"), 0o644); err != nil {
		t.Fatalf("write untracked file: %v", err)
	}

	stashed, err := stashChanges(repoPath)
	if err != nil {
		t.Fatalf("stashChanges() unexpected error: %v", err)
	}
	if stashed {
		t.Fatal("stashChanges() = true, want false when no stash entry is created")
	}

	if stashList := runGit(t, repoPath, "stash", "list"); stashList != "" {
		t.Fatalf("expected no stash entries, got %q", stashList)
	}
}

func TestHandleWorkingDirectoryState_NoStashEntryReturnsNotStashed(t *testing.T) {
	workDir := t.TempDir()
	repoName := "repo"
	repoPath := filepath.Join(workDir, repoName)

	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo path: %v", err)
	}

	runGit(t, repoPath, "init")
	runGit(t, repoPath, "config", "user.name", "Test User")
	runGit(t, repoPath, "config", "user.email", "test@example.com")

	trackedFile := filepath.Join(repoPath, "tracked.txt")
	if err := os.WriteFile(trackedFile, []byte("committed\n"), 0o644); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}
	runGit(t, repoPath, "add", "tracked.txt")
	runGit(t, repoPath, "commit", "-m", "initial")

	untrackedFile := filepath.Join(repoPath, "untracked.txt")
	if err := os.WriteFile(untrackedFile, []byte("new\n"), 0o644); err != nil {
		t.Fatalf("write untracked file: %v", err)
	}

	s := &Syncer{workDir: workDir, repoName: repoName}
	stashed, err := s.handleWorkingDirectoryState()
	if err != nil {
		t.Fatalf("handleWorkingDirectoryState() unexpected error: %v", err)
	}
	if stashed {
		t.Fatal("handleWorkingDirectoryState() = true, want false when stash push creates no entry")
	}
}

func runGit(t *testing.T, repoPath string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(output))
	}

	return strings.TrimSpace(string(output))
}
