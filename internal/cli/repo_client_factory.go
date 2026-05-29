package cli

import (
	"codeberg.org/snonux/gitsyncer/internal/codeberg"
	"codeberg.org/snonux/gitsyncer/internal/forge"
	"codeberg.org/snonux/gitsyncer/internal/github"
)

type githubPublicRepoClient interface {
	HasToken() bool
	ListPublicRepos() ([]github.Repository, error)
}

type codebergPublicRepoClient interface {
	ListPublicRepos() ([]codeberg.Repository, error)
	ListUserPublicRepos() ([]codeberg.Repository, error)
}

type repoClientFactory interface {
	NewGitHubRepoClient(token, org string) forge.RepoClient
	NewCodebergRepoClient(token, org string) forge.RepoClient
	NewGitHubDescriptionClient(token, org string) forge.RepoDescriptionClient
	NewCodebergDescriptionClient(token, org string) forge.RepoDescriptionClient
	NewGitHubPublicRepoClient(token, org string) githubPublicRepoClient
	NewCodebergPublicRepoClient(token, org string) codebergPublicRepoClient
}

type defaultRepoClientFactory struct{}

func (defaultRepoClientFactory) NewGitHubRepoClient(token, org string) forge.RepoClient {
	return github.NewClient(token, org)
}

func (defaultRepoClientFactory) NewCodebergRepoClient(token, org string) forge.RepoClient {
	return codeberg.NewClient(token, org)
}

func (defaultRepoClientFactory) NewGitHubDescriptionClient(token, org string) forge.RepoDescriptionClient {
	return github.NewClient(token, org)
}

func (defaultRepoClientFactory) NewCodebergDescriptionClient(token, org string) forge.RepoDescriptionClient {
	return codeberg.NewClient(token, org)
}

func (defaultRepoClientFactory) NewGitHubPublicRepoClient(token, org string) githubPublicRepoClient {
	return github.NewClient(token, org)
}

func (defaultRepoClientFactory) NewCodebergPublicRepoClient(token, org string) codebergPublicRepoClient {
	return codeberg.NewClient(token, org)
}

var cliRepoClientFactory repoClientFactory = defaultRepoClientFactory{}
