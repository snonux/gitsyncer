package cli

import (
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

// LookupRepo implements forge.RepoLookuper on top of github.Client.GetRepo,
// so configured extra repositories can be checked before they are synced.
func (l githubPublicRepoLister) LookupRepo(name string) (forge.RepoLookup, error) {
	repo, exists, err := l.Client.GetRepo(name)
	if err != nil || !exists {
		return forge.RepoLookup{}, err
	}
	return forge.RepoLookup{Exists: true, Name: repo.Name, Private: repo.Private, Archived: repo.Archived}, nil
}

// newGitHubPublicRepoLister builds the forge.PublicRepoLister for a GitHub
// organization. It is the injection seam HandleSyncGitHubPublic uses so
// tests can substitute a fake lister instead of making real GitHub API
// calls.
func newGitHubPublicRepoLister(org config.Organization) forge.PublicRepoLister {
	return githubPublicRepoLister{github.NewClient(org.GitHubToken, org.Name)}
}
