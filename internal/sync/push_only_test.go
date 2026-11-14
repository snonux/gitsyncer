package sync

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/snonux/gitsyncer/internal/config"
)

// commitAndPush clones source's main branch into a scratch directory under
// root, commits one new file, optionally tags and/or branches that commit,
// and pushes the result to target. It lets a test put content on exactly one
// bare remote.
func commitAndPush(t *testing.T, root, name, source, target, branch, tag string) {
	t.Helper()

	repoPath := filepath.Join(root, name)
	runGit(t, root, "clone", "--branch", "main", source, repoPath)
	runGit(t, repoPath, "config", "user.name", "GitSyncer Test")
	runGit(t, repoPath, "config", "user.email", "gitsyncer@example.test")
	if branch != "main" {
		runGit(t, repoPath, "checkout", "-b", branch)
	}
	if err := os.WriteFile(filepath.Join(repoPath, name+".txt"), []byte(name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoPath, "add", ".")
	runGit(t, repoPath, "commit", "-m", name)
	refs := []string{branch}
	if tag != "" {
		runGit(t, repoPath, "tag", tag)
		refs = append(refs, tag)
	}
	runGit(t, repoPath, append([]string{"push", target}, refs...)...)
}

// refExists reports whether ref resolves in the bare repository gitDir.
func refExists(gitDir, ref string) bool {
	return exec.Command("git", "--git-dir", gitDir, "rev-parse", "--verify", "--quiet", ref).Run() == nil
}

// TestSyncRepository_PushOnlyMirrorIsOneWay covers the one-way contract of a
// pushOnly organization: everything on the primary forge reaches the mirror,
// while branches and tags that exist only on the mirror never reach the
// primary forge. The working copy is cloned up front with the mirror already
// fetched, reproducing a peer that used to be bidirectional and therefore
// left stale remote-tracking refs behind.
func TestSyncRepository_PushOnlyMirrorIsOneWay(t *testing.T) {
	root := t.TempDir()
	githubRoot := filepath.Join(root, "github")
	mirrorRoot := filepath.Join(root, "mirror")
	github := filepath.Join(githubRoot, "demo.git")
	mirror := filepath.Join(mirrorRoot, "demo.git")
	for _, dir := range []string{githubRoot, mirrorRoot} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, root, "init", "--bare", "-b", "main", github)
	runGit(t, root, "init", "--bare", "-b", "main", mirror)

	seed := filepath.Join(root, "seed")
	runGit(t, root, "init", "-b", "main", seed)
	runGit(t, seed, "config", "user.name", "GitSyncer Test")
	runGit(t, seed, "config", "user.email", "gitsyncer@example.test")
	runGit(t, seed, "commit", "--allow-empty", "-m", "base")
	runGit(t, seed, "push", github, "main")
	runGit(t, seed, "push", mirror, "main")

	// Content that exists only on the mirror.
	commitAndPush(t, root, "mirror-branch", mirror, mirror, "mirror-only", "mirror-tag")

	// Working copy that already fetched the mirror (stale tracking refs).
	workDir := filepath.Join(root, "work")
	if err := os.Mkdir(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	repoPath := filepath.Join(workDir, "demo")
	runGit(t, workDir, "clone", "--branch", "main", github, repoPath)
	runGit(t, repoPath, "remote", "rename", "origin", "github")
	runGit(t, repoPath, "config", "user.name", "GitSyncer Test")
	runGit(t, repoPath, "config", "user.email", "gitsyncer@example.test")
	runGit(t, repoPath, "remote", "add", "mirror", "file://"+mirror)
	runGit(t, repoPath, "fetch", "--no-tags", "mirror")

	// New content on the primary forge after the working copy was made.
	commitAndPush(t, root, "github-main", github, github, "main", "v1.0.0")
	commitAndPush(t, root, "github-feature", github, github, "feature", "")

	cfg := &config.Config{Branches: []string{"main", "feature"}, Organizations: []config.Organization{
		{Host: "file://" + githubRoot},
		{Host: "file://" + mirrorRoot, PushOnly: true},
	}}
	if err := New(cfg, workDir).SyncRepository("demo"); err != nil {
		t.Fatalf("SyncRepository() error = %v", err)
	}

	for _, ref := range []string{"refs/heads/main", "refs/heads/feature", "refs/tags/v1.0.0"} {
		want := runGit(t, root, "--git-dir", github, "rev-parse", ref)
		if got := runGit(t, root, "--git-dir", mirror, "rev-parse", ref); got != want {
			t.Errorf("mirror %s = %s, want %s (primary)", ref, got, want)
		}
	}
	for _, ref := range []string{"refs/heads/mirror-only", "refs/tags/mirror-tag"} {
		if refExists(github, ref) {
			t.Errorf("mirror-only ref %s leaked to the primary forge", ref)
		}
		if !refExists(mirror, ref) {
			t.Errorf("mirror-only ref %s was removed from the mirror", ref)
		}
	}
}
