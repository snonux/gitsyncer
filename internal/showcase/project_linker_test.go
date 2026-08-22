package showcase

import (
	"os"
	"os/exec"
	"testing"

	"github.com/snonux/gitsyncer/internal/config"
)

func TestBuildProjectLinks_CodebergLinkOnlyWhenSyncedToCodeberg(t *testing.T) {
	t.Parallel()

	// Build a throwaway git repo with a codeberg remote so that
	// repoHasCodebergRemote detects an active Codeberg sync.
	repoPath := t.TempDir()
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for _, args := range [][]string{
		{"init"},
		{"remote", "add", "codeberg_org", "git@codeberg.org:snonux/cpuinfo.git"},
	} {
		cmd := exec.Command("git", append([]string{"-C", repoPath}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	codebergOrg := config.Organization{Host: "git@codeberg.org", Name: "snonux"}

	// Codeberg sync disabled: no codeberg link even with a codeberg remote and
	// the repo in the allowlist.
	disabled := NewProjectLinker(&config.Config{
		Organizations: []config.Organization{codebergOrg},
		Repositories:  []string{"cpuinfo"},
	})
	if codebergURL, _ := disabled.BuildLinks("cpuinfo", repoPath); codebergURL != "" {
		t.Fatalf("disabled BuildLinks() codeberg URL = %q, want empty", codebergURL)
	}

	// Codeberg sync enabled and repo is in the allowlist and has a codeberg
	// remote: link emitted.
	enabled := NewProjectLinker(&config.Config{
		Organizations: []config.Organization{codebergOrg},
		Repositories:  []string{"cpuinfo"},
		SyncCodeberg:  true,
	})
	codebergURL, _ := enabled.BuildLinks("cpuinfo", repoPath)
	want := "https://codeberg.org/snonux/cpuinfo"
	if codebergURL != want {
		t.Fatalf("enabled BuildLinks() codeberg URL = %q, want %q", codebergURL, want)
	}

	// Codeberg sync enabled and repo has a codeberg remote, but repo is NOT in
	// the allowlist: no link (it is not one of the repos we sync with Codeberg).
	if codebergURL, _ := enabled.BuildLinks("other-repo", repoPath); codebergURL != "" {
		t.Fatalf("non-allowlisted BuildLinks() codeberg URL = %q, want empty", codebergURL)
	}

	// Codeberg sync enabled, repo in allowlist, but repo has no codeberg remote
	// (not actually on Codeberg yet): no link (would point to a dead URL).
	plainRepo := t.TempDir()
	if err := os.MkdirAll(plainRepo, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if out, err := exec.Command("git", "-C", plainRepo, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if codebergURL, _ := enabled.BuildLinks("cpuinfo", plainRepo); codebergURL != "" {
		t.Fatalf("BuildLinks() codeberg URL = %q, want empty when repo not on Codeberg", codebergURL)
	}

	// Discovery mode (Repositories empty): a repo with a codeberg remote still
	// gets a link when sync_codeberg is enabled.
	discovery := NewProjectLinker(&config.Config{
		Organizations: []config.Organization{codebergOrg},
		SyncCodeberg:  true,
	})
	if codebergURL, _ := discovery.BuildLinks("cpuinfo", repoPath); codebergURL != want {
		t.Fatalf("discovery BuildLinks() codeberg URL = %q, want %q", codebergURL, want)
	}
}
