package sync

import "testing"

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
