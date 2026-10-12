package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/snonux/gitsyncer/internal/config"
	"github.com/snonux/gitsyncer/internal/forge"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	original := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = writer
	defer func() { os.Stdout = original }()

	fn()
	if err := writer.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}
	var output bytes.Buffer
	if _, err := output.ReadFrom(reader); err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	return output.String()
}

type stubForgeClientResolver struct{}

func (stubForgeClientResolver) ClientFor(*config.Organization) (forge.RepoDescriptionClient, bool) {
	return nil, false
}

type stubGitHubPublicRepoLister struct {
	hasToken bool
	repos    []forge.PublicRepo
}

func (s *stubGitHubPublicRepoLister) HasToken() bool { return s.hasToken }

func (s *stubGitHubPublicRepoLister) ListPublicRepos() ([]forge.PublicRepo, error) {
	return s.repos, nil
}

func githubDiscoveryDryRun(t *testing.T, extraOrgs ...config.Organization) string {
	t.Helper()

	lister := &stubGitHubPublicRepoLister{
		hasToken: true,
		repos:    []forge.PublicRepo{{Name: "wanted"}, {Name: "not-configured"}},
	}
	newLister := func(config.Organization) forge.PublicRepoLister { return lister }
	cfg := &config.Config{
		Organizations: append([]config.Organization{{Host: "git@github.com", Name: "acme"}}, extraOrgs...),
		Repositories:  []string{"wanted"},
	}
	flags := &Flags{DryRun: true, WorkDir: t.TempDir()}

	return captureStdout(t, func() {
		if got := handleSyncGitHubPublicWithDeps(cfg, flags, newLister, stubForgeClientResolver{}); got != 0 {
			t.Fatalf("handleSyncGitHubPublicWithDeps() = %d, want 0", got)
		}
	})
}

func TestHandleSyncGitHubPublicWithDeps_UsesRepositoryAllowlist(t *testing.T) {
	output := githubDiscoveryDryRun(t)
	if strings.Contains(output, "not-configured") {
		t.Fatalf("non-allowlisted repo appeared in sync output:\n%s", output)
	}
	if !strings.Contains(output, "Would sync 1 repositories from GitHub to configured destinations") {
		t.Fatalf("unexpected dry-run summary:\n%s", output)
	}
}

func TestHandleSyncGitHubPublicWithDeps_PushOnlyMirrorSyncsAllRepos(t *testing.T) {
	output := githubDiscoveryDryRun(t, config.Organization{
		Host:           "ssh://git@forgejo.example:2022",
		ForgejoAPIBase: "https://forgejo.example/api/v1",
		ForgejoOwner:   "acme",
		PushOnly:       true,
	})
	if !strings.Contains(output, "not-configured") {
		t.Fatalf("expected non-allowlisted repo to be mirrored:\n%s", output)
	}
	if strings.Contains(output, "allowlist") {
		t.Fatalf("allowlist was applied with a pushOnly mirror:\n%s", output)
	}
	if !strings.Contains(output, "Would sync 2 repositories from GitHub to push-only Forgejo mirrors") {
		t.Fatalf("unexpected mirror dry-run summary:\n%s", output)
	}
}
