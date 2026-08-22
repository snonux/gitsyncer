package cli

import (
	"github.com/snonux/gitsyncer/internal/codeberg"
	"github.com/snonux/gitsyncer/internal/config"
	"github.com/snonux/gitsyncer/internal/forge"
	"github.com/snonux/gitsyncer/internal/github"
)

// githubPublicRepoLister adapts *github.Client's ListPublicRepos (which
// returns github.Repository, a GitHub-specific DTO with many fields the
// public-sync pipeline doesn't care about) to forge.PublicRepoLister's
// forge-agnostic forge.PublicRepo, so callers don't need to import the
// github package for its concrete Repository type.
type githubPublicRepoLister struct {
	*github.Client
}

func (l githubPublicRepoLister) ListPublicRepos() ([]forge.PublicRepo, error) {
	repos, err := l.Client.ListPublicRepos()
	converted := make([]forge.PublicRepo, len(repos))
	for i, repo := range repos {
		converted[i] = forge.PublicRepo{Name: repo.Name, Description: repo.Description}
	}
	return converted, err
}

// newGitHubPublicRepoLister builds the forge.PublicRepoLister for a GitHub
// organization. It is the injection seam HandleSyncGitHubPublic uses so
// tests can substitute a fake lister instead of making real GitHub API
// calls.
func newGitHubPublicRepoLister(org config.Organization) forge.PublicRepoLister {
	return githubPublicRepoLister{github.NewClient(org.GitHubToken, org.Name)}
}

// codebergPublicRepoLister adapts *codeberg.Client's ListPublicRepos and
// ListUserPublicRepos (which return codeberg.Repository) to
// forge.UserFallbackPublicRepoLister's forge.PublicRepo, mirroring
// githubPublicRepoLister above.
type codebergPublicRepoLister struct {
	*codeberg.Client
}

func (l codebergPublicRepoLister) ListPublicRepos() ([]forge.PublicRepo, error) {
	repos, err := l.Client.ListPublicRepos()
	return convertCodebergRepos(repos), err
}

func (l codebergPublicRepoLister) ListUserPublicRepos() ([]forge.PublicRepo, error) {
	repos, err := l.Client.ListUserPublicRepos()
	return convertCodebergRepos(repos), err
}

func convertCodebergRepos(repos []codeberg.Repository) []forge.PublicRepo {
	converted := make([]forge.PublicRepo, len(repos))
	for i, repo := range repos {
		converted[i] = forge.PublicRepo{Name: repo.Name, Description: repo.Description}
	}
	return converted
}

// newCodebergPublicRepoLister builds the forge.UserFallbackPublicRepoLister
// for a Codeberg organization. It is the injection seam
// HandleSyncCodebergPublic uses so tests can substitute a fake lister
// instead of making real Codeberg API calls.
func newCodebergPublicRepoLister(org config.Organization) forge.UserFallbackPublicRepoLister {
	return codebergPublicRepoLister{codeberg.NewClient(org.CodebergToken, org.Name)}
}
