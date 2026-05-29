package cli

import (
	"testing"

	"codeberg.org/snonux/gitsyncer/internal/codeberg"
	"codeberg.org/snonux/gitsyncer/internal/config"
	"codeberg.org/snonux/gitsyncer/internal/forge"
	"codeberg.org/snonux/gitsyncer/internal/github"
)

type createCall struct {
	repoName    string
	description string
	privateRepo bool
}

type updateCall struct {
	repoName    string
	description string
}

type stubDescriptionRepoClient struct {
	hasToken           bool
	descriptionByRepo  map[string]string
	updatedDescription []updateCall
	createCalls        []createCall
}

func (s *stubDescriptionRepoClient) HasToken() bool { return s.hasToken }

func (s *stubDescriptionRepoClient) RepoExists(repoName string) (bool, error) {
	_, exists := s.descriptionByRepo[repoName]
	return exists, nil
}

func (s *stubDescriptionRepoClient) CreateRepo(repoName, description string, private bool) error {
	s.createCalls = append(s.createCalls, createCall{
		repoName:    repoName,
		description: description,
		privateRepo: private,
	})
	return nil
}

func (s *stubDescriptionRepoClient) DeleteRepo(_ string) error {
	return nil
}

func (s *stubDescriptionRepoClient) GetRepoDescription(repoName string) (string, bool, error) {
	description, exists := s.descriptionByRepo[repoName]
	return description, exists, nil
}

func (s *stubDescriptionRepoClient) UpdateRepoDescription(repoName, description string) error {
	s.updatedDescription = append(s.updatedDescription, updateCall{
		repoName:    repoName,
		description: description,
	})
	if s.descriptionByRepo == nil {
		s.descriptionByRepo = map[string]string{}
	}
	s.descriptionByRepo[repoName] = description
	return nil
}

type stubGitHubPublicRepoClient struct {
	hasToken bool
	repos    []github.Repository
}

func (s *stubGitHubPublicRepoClient) HasToken() bool {
	return s.hasToken
}

func (s *stubGitHubPublicRepoClient) ListPublicRepos() ([]github.Repository, error) {
	return s.repos, nil
}

type stubCodebergPublicRepoClient struct {
	orgRepos  []codeberg.Repository
	userRepos []codeberg.Repository
}

func (s *stubCodebergPublicRepoClient) ListPublicRepos() ([]codeberg.Repository, error) {
	return s.orgRepos, nil
}

func (s *stubCodebergPublicRepoClient) ListUserPublicRepos() ([]codeberg.Repository, error) {
	return s.userRepos, nil
}

type stubRepoClientFactory struct {
	githubRepoClient        forge.RepoClient
	codebergRepoClient      forge.RepoClient
	githubDescriptionClient forge.RepoDescriptionClient
	codebergDescClient      forge.RepoDescriptionClient
	githubPublicClient      githubPublicRepoClient
	codebergPublicClient    codebergPublicRepoClient

	githubRepoCalls       int
	codebergRepoCalls     int
	githubDescCalls       int
	codebergDescCalls     int
	githubPublicRepoCalls int
	codebergPublicCalls   int
}

func (s *stubRepoClientFactory) NewGitHubRepoClient(_, _ string) forge.RepoClient {
	s.githubRepoCalls++
	return s.githubRepoClient
}

func (s *stubRepoClientFactory) NewCodebergRepoClient(_, _ string) forge.RepoClient {
	s.codebergRepoCalls++
	return s.codebergRepoClient
}

func (s *stubRepoClientFactory) NewGitHubDescriptionClient(_, _ string) forge.RepoDescriptionClient {
	s.githubDescCalls++
	return s.githubDescriptionClient
}

func (s *stubRepoClientFactory) NewCodebergDescriptionClient(_, _ string) forge.RepoDescriptionClient {
	s.codebergDescCalls++
	return s.codebergDescClient
}

func (s *stubRepoClientFactory) NewGitHubPublicRepoClient(_, _ string) githubPublicRepoClient {
	s.githubPublicRepoCalls++
	return s.githubPublicClient
}

func (s *stubRepoClientFactory) NewCodebergPublicRepoClient(_, _ string) codebergPublicRepoClient {
	s.codebergPublicCalls++
	return s.codebergPublicClient
}

func TestSyncRepoDescriptionsWithFactory_UsesInjectedClients(t *testing.T) {
	t.Parallel()

	githubClient := &stubDescriptionRepoClient{
		hasToken:          true,
		descriptionByRepo: map[string]string{"demo": "from github"},
	}
	codebergClient := &stubDescriptionRepoClient{
		hasToken:          true,
		descriptionByRepo: map[string]string{"demo": "from codeberg"},
	}
	factory := &stubRepoClientFactory{
		githubDescriptionClient: githubClient,
		codebergDescClient:      codebergClient,
	}

	cfg := &config.Config{
		Organizations: []config.Organization{
			{
				Host:          "git@github.com",
				Name:          "acme",
				GitHubToken:   "gh-token",
				CodebergToken: "",
			},
			{
				Host:          "git@codeberg.org",
				Name:          "acme",
				CodebergToken: "cb-token",
			},
		},
	}
	cache := map[string]string{}

	syncRepoDescriptionsWithFactory(cfg, false, "demo", "", "", cache, factory)

	if factory.githubDescCalls != 1 || factory.codebergDescCalls != 1 {
		t.Fatalf("expected one injected description client creation per forge, got github=%d codeberg=%d", factory.githubDescCalls, factory.codebergDescCalls)
	}

	if len(codebergClient.updatedDescription) != 0 {
		t.Fatalf("expected no Codeberg update when it already has canonical description, got %d updates", len(codebergClient.updatedDescription))
	}

	if len(githubClient.updatedDescription) != 1 {
		t.Fatalf("expected GitHub description to be updated once, got %d updates", len(githubClient.updatedDescription))
	}
	if githubClient.updatedDescription[0].description != "from codeberg" {
		t.Fatalf("github description update = %q, want %q", githubClient.updatedDescription[0].description, "from codeberg")
	}

	if cache["demo"] != "from codeberg" {
		t.Fatalf("cache description = %q, want %q", cache["demo"], "from codeberg")
	}
}

func TestCreateRepoHelpersWithFactory_UseInjectedCreateClients(t *testing.T) {
	t.Parallel()

	githubClient := &stubDescriptionRepoClient{hasToken: true}
	codebergClient := &stubDescriptionRepoClient{hasToken: true}
	factory := &stubRepoClientFactory{
		githubRepoClient:   githubClient,
		codebergRepoClient: codebergClient,
	}

	cfg := &config.Config{
		Organizations: []config.Organization{
			{
				Host:        "git@github.com",
				Name:        "acme",
				GitHubToken: "gh-token",
			},
			{
				Host:          "git@codeberg.org",
				Name:          "acme",
				CodebergToken: "cb-token",
			},
		},
	}

	if err := createGitHubRepoIfNeededWithFactory(cfg, "demo", factory); err != nil {
		t.Fatalf("createGitHubRepoIfNeededWithFactory() error = %v", err)
	}
	if err := createCodebergRepoIfNeededWithFactory(cfg, "demo", factory); err != nil {
		t.Fatalf("createCodebergRepoIfNeededWithFactory() error = %v", err)
	}

	if factory.githubRepoCalls != 1 || factory.codebergRepoCalls != 1 {
		t.Fatalf("expected one injected create client per forge, got github=%d codeberg=%d", factory.githubRepoCalls, factory.codebergRepoCalls)
	}

	if len(githubClient.createCalls) != 1 {
		t.Fatalf("expected one GitHub create call, got %d", len(githubClient.createCalls))
	}
	if len(codebergClient.createCalls) != 1 {
		t.Fatalf("expected one Codeberg create call, got %d", len(codebergClient.createCalls))
	}

	if githubClient.createCalls[0].description != "Mirror of demo" {
		t.Fatalf("github create description = %q, want %q", githubClient.createCalls[0].description, "Mirror of demo")
	}
	if codebergClient.createCalls[0].description != "Mirror of demo" {
		t.Fatalf("codeberg create description = %q, want %q", codebergClient.createCalls[0].description, "Mirror of demo")
	}
}
