package cli

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"codeberg.org/snonux/gitsyncer/internal/codeberg"
	"codeberg.org/snonux/gitsyncer/internal/config"
	"codeberg.org/snonux/gitsyncer/internal/forge"
	"codeberg.org/snonux/gitsyncer/internal/github"
)

// captureStdout runs fn while redirecting os.Stdout, returning the captured
// output. Used by handler tests that need to assert on user-facing messages.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()
	if err := w.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	return buf.String()
}

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
	orgErr    error
	userErr   error
}

func (s *stubCodebergPublicRepoClient) ListPublicRepos() ([]codeberg.Repository, error) {
	return s.orgRepos, s.orgErr
}

func (s *stubCodebergPublicRepoClient) ListUserPublicRepos() ([]codeberg.Repository, error) {
	return s.userRepos, s.userErr
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
	githubPublicToken     string
	githubPublicOrg       string
	codebergPublicToken   string
	codebergPublicOrg     string
	githubRepoToken       string
	githubRepoOrg         string
	codebergRepoToken     string
	codebergRepoOrg       string
}

func (s *stubRepoClientFactory) NewGitHubRepoClient(token, org string) forge.RepoClient {
	s.githubRepoCalls++
	s.githubRepoToken = token
	s.githubRepoOrg = org
	return s.githubRepoClient
}

func (s *stubRepoClientFactory) NewCodebergRepoClient(token, org string) forge.RepoClient {
	s.codebergRepoCalls++
	s.codebergRepoToken = token
	s.codebergRepoOrg = org
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

func (s *stubRepoClientFactory) NewGitHubPublicRepoClient(token, org string) githubPublicRepoClient {
	s.githubPublicRepoCalls++
	s.githubPublicToken = token
	s.githubPublicOrg = org
	return s.githubPublicClient
}

func (s *stubRepoClientFactory) NewCodebergPublicRepoClient(token, org string) codebergPublicRepoClient {
	s.codebergPublicCalls++
	s.codebergPublicToken = token
	s.codebergPublicOrg = org
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
		SyncCodeberg: true,
	}
	cache := map[string]string{}

	syncRepoDescriptionsWithFactory(cfg, false, nil, nil, "demo", "", "", cache, factory)

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
		SyncCodeberg: true,
	}

	if err := createGitHubRepoIfNeededWithFactory(cfg, "demo", false, factory); err != nil {
		t.Fatalf("createGitHubRepoIfNeededWithFactory() error = %v", err)
	}
	if err := createCodebergRepoIfNeededWithFactory(cfg, "demo", false, factory); err != nil {
		t.Fatalf("createCodebergRepoIfNeededWithFactory() error = %v", err)
	}

	if factory.githubRepoCalls != 1 || factory.codebergRepoCalls != 1 {
		t.Fatalf("expected one injected create client per forge, got github=%d codeberg=%d", factory.githubRepoCalls, factory.codebergRepoCalls)
	}
	if factory.githubRepoToken != "gh-token" || factory.githubRepoOrg != "acme" {
		t.Fatalf("github repo client init args = (%q, %q), want (%q, %q)", factory.githubRepoToken, factory.githubRepoOrg, "gh-token", "acme")
	}
	if factory.codebergRepoToken != "cb-token" || factory.codebergRepoOrg != "acme" {
		t.Fatalf("codeberg repo client init args = (%q, %q), want (%q, %q)", factory.codebergRepoToken, factory.codebergRepoOrg, "cb-token", "acme")
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

func TestCreateRepoHelpersWithFactory_DryRunDispatchesNoMutations(t *testing.T) {
	t.Parallel()

	githubClient := &stubDescriptionRepoClient{hasToken: true}
	codebergClient := &stubDescriptionRepoClient{hasToken: true}
	factory := &stubRepoClientFactory{
		githubRepoClient:   githubClient,
		codebergRepoClient: codebergClient,
	}
	cfg := &config.Config{
		Organizations: []config.Organization{
			{Host: "git@github.com", Name: "acme", GitHubToken: "gh-token"},
			{Host: "git@codeberg.org", Name: "acme", CodebergToken: "cb-token"},
		},
		Repositories: []string{"demo"},
		SyncCodeberg: true,
	}

	if err := createGitHubRepoIfNeededWithFactory(cfg, "demo", true, factory); err != nil {
		t.Fatalf("dry-run GitHub create returned error: %v", err)
	}
	if err := createCodebergRepoIfNeededWithFactory(cfg, "demo", true, factory); err != nil {
		t.Fatalf("dry-run Codeberg create returned error: %v", err)
	}
	if len(githubClient.createCalls) != 0 || len(codebergClient.createCalls) != 0 {
		t.Fatalf("dry run dispatched create mutations: github=%d codeberg=%d", len(githubClient.createCalls), len(codebergClient.createCalls))
	}
	if factory.githubRepoCalls != 0 || factory.codebergRepoCalls != 0 {
		t.Fatalf("dry run initialized mutation clients: github=%d codeberg=%d", factory.githubRepoCalls, factory.codebergRepoCalls)
	}
}

func TestSyncCodebergRepos_DryRunCreateReposDispatchesNoMutations(t *testing.T) {
	t.Parallel()

	client := &stubDescriptionRepoClient{hasToken: true}
	factory := &stubRepoClientFactory{githubRepoClient: client}
	flags := &Flags{DryRun: true, CreateGitHubRepos: true, WorkDir: t.TempDir()}

	if got := syncCodebergRepos(&config.Config{}, flags, nil, []string{"demo"}, factory); got != 0 {
		t.Fatalf("syncCodebergRepos() = %d, want 0", got)
	}
	if factory.githubRepoCalls != 0 {
		t.Fatalf("dry run initialized %d GitHub mutation clients, want 0", factory.githubRepoCalls)
	}
	if len(client.createCalls) != 0 {
		t.Fatalf("dry run dispatched %d GitHub create calls, want 0", len(client.createCalls))
	}
}

func TestSyncGitHubRepos_DryRunCreateReposDispatchesNoMutations(t *testing.T) {
	t.Parallel()

	client := &stubDescriptionRepoClient{hasToken: true}
	factory := &stubRepoClientFactory{codebergRepoClient: client}
	flags := &Flags{DryRun: true, CreateCodebergRepos: true, WorkDir: t.TempDir()}

	if got := syncGitHubRepos(&config.Config{}, flags, nil, []string{"demo"}, factory); got != 0 {
		t.Fatalf("syncGitHubRepos() = %d, want 0", got)
	}
	if factory.codebergRepoCalls != 0 {
		t.Fatalf("dry run initialized %d Codeberg mutation clients, want 0", factory.codebergRepoCalls)
	}
	if len(client.createCalls) != 0 {
		t.Fatalf("dry run dispatched %d Codeberg create calls, want 0", len(client.createCalls))
	}
}

func TestHandleSyncGitHubPublic_UsesInjectedFactoryClient(t *testing.T) {
	t.Parallel()

	factory := &stubRepoClientFactory{
		githubPublicClient: &stubGitHubPublicRepoClient{
			hasToken: true,
			repos: []github.Repository{
				{Name: "demo"},
			},
		},
	}

	cfg := &config.Config{
		Organizations: []config.Organization{
			{Host: "git@github.com", Name: "acme", GitHubToken: "gh-token"},
		},
	}
	flags := &Flags{
		DryRun:  true,
		WorkDir: t.TempDir(),
	}

	if got := handleSyncGitHubPublicWithFactory(cfg, flags, factory); got != 0 {
		t.Fatalf("handleSyncGitHubPublicWithFactory() = %d, want 0", got)
	}
	if factory.githubPublicRepoCalls != 1 {
		t.Fatalf("expected exactly one injected GitHub public client creation, got %d", factory.githubPublicRepoCalls)
	}
	if factory.githubPublicToken != "gh-token" || factory.githubPublicOrg != "acme" {
		t.Fatalf("github public client init args = (%q, %q), want (%q, %q)", factory.githubPublicToken, factory.githubPublicOrg, "gh-token", "acme")
	}
}

func TestHandleSyncCodebergPublic_UsesInjectedFactoryClient(t *testing.T) {
	t.Parallel()

	factory := &stubRepoClientFactory{
		codebergPublicClient: &stubCodebergPublicRepoClient{
			orgErr: errors.New("org lookup failed"),
			userRepos: []codeberg.Repository{
				{Name: "demo"},
			},
		},
	}

	cfg := &config.Config{
		Organizations: []config.Organization{
			{Host: "git@codeberg.org", Name: "acme", CodebergToken: "cb-token"},
		},
		SyncCodeberg: true,
	}
	flags := &Flags{
		DryRun:           true,
		SyncGitHubPublic: false,
		WorkDir:          t.TempDir(),
	}

	if got := handleSyncCodebergPublicWithFactory(cfg, flags, factory); got != 0 {
		t.Fatalf("handleSyncCodebergPublicWithFactory() = %d, want 0", got)
	}
	if factory.codebergPublicCalls != 1 {
		t.Fatalf("expected exactly one injected Codeberg public client creation, got %d", factory.codebergPublicCalls)
	}
	if factory.codebergPublicToken != "cb-token" || factory.codebergPublicOrg != "acme" {
		t.Fatalf("codeberg public client init args = (%q, %q), want (%q, %q)", factory.codebergPublicToken, factory.codebergPublicOrg, "cb-token", "acme")
	}
}

func TestHandleSyncCodebergPublicWithFactory_RestrictsToConfiguredRepos(t *testing.T) {
	t.Parallel()

	factory := &stubRepoClientFactory{
		codebergPublicClient: &stubCodebergPublicRepoClient{
			orgRepos: []codeberg.Repository{
				{Name: "wanted"},
				{Name: "also-wanted"},
				{Name: "not-configured"},
			},
		},
	}
	// Repositories acts as an allowlist: only "wanted" and "also-wanted"
	// should be synced even though Codeberg reports a third public repo.
	cfg := &config.Config{
		Organizations: []config.Organization{
			{Host: "git@codeberg.org", Name: "acme", CodebergToken: "cb-token"},
		},
		Repositories: []string{"wanted", "also-wanted", "missing-on-codeberg"},
		SyncCodeberg: true,
	}
	flags := &Flags{DryRun: true, WorkDir: t.TempDir()}

	out := captureStdout(t, func() {
		if got := handleSyncCodebergPublicWithFactory(cfg, flags, factory); got != 0 {
			t.Fatalf("handleSyncCodebergPublicWithFactory() = %d, want 0", got)
		}
	})

	if !strings.Contains(out, "wanted") || !strings.Contains(out, "also-wanted") {
		t.Fatalf("expected allowlisted repos in output, got:\n%s", out)
	}
	if strings.Contains(out, "not-configured") {
		t.Fatalf("non-allowlisted repo 'not-configured' leaked into sync output:\n%s", out)
	}
	if !strings.Contains(out, "allowlist") {
		t.Fatalf("expected allowlist notice in output, got:\n%s", out)
	}
}

func TestHandleSyncCodebergPublicWithFactory_SkipsWhenCodebergSyncDisabled(t *testing.T) {
	t.Parallel()

	factory := &stubRepoClientFactory{
		codebergPublicClient: &stubCodebergPublicRepoClient{
			userRepos: []codeberg.Repository{{Name: "demo"}},
		},
	}
	// A Codeberg org is configured, but sync_codeberg is not enabled.
	cfg := &config.Config{
		Organizations: []config.Organization{
			{Host: "git@codeberg.org", Name: "acme", CodebergToken: "cb-token"},
		},
	}
	flags := &Flags{DryRun: true, WorkDir: t.TempDir()}

	if got := handleSyncCodebergPublicWithFactory(cfg, flags, factory); got != 0 {
		t.Fatalf("handleSyncCodebergPublicWithFactory() = %d, want 0 when Codeberg sync disabled", got)
	}
	if factory.codebergPublicCalls != 0 {
		t.Fatalf("expected no Codeberg public client creation when disabled, got %d", factory.codebergPublicCalls)
	}
}

func TestCreateCodebergRepoIfNeededWithFactory_SkipsWhenCodebergSyncDisabled(t *testing.T) {
	t.Parallel()

	factory := &stubRepoClientFactory{
		codebergRepoClient: &stubDescriptionRepoClient{hasToken: true},
	}
	cfg := &config.Config{
		Organizations: []config.Organization{
			{Host: "git@codeberg.org", Name: "acme", CodebergToken: "cb-token"},
		},
	}

	if err := createCodebergRepoIfNeededWithFactory(cfg, "demo", false, factory); err != nil {
		t.Fatalf("createCodebergRepoIfNeededWithFactory() error = %v", err)
	}
	if factory.codebergRepoCalls != 0 {
		t.Fatalf("expected no Codeberg repo client creation when disabled, got %d", factory.codebergRepoCalls)
	}
}

func TestHandleSyncGitHubPublicWithFactory_ReturnsErrorWhenFactoryReturnsNilClient(t *testing.T) {
	t.Parallel()

	factory := &stubRepoClientFactory{githubPublicClient: nil}
	cfg := &config.Config{
		Organizations: []config.Organization{
			{Host: "git@github.com", Name: "acme", GitHubToken: "gh-token"},
		},
	}
	flags := &Flags{
		DryRun:  true,
		WorkDir: t.TempDir(),
	}

	if got := handleSyncGitHubPublicWithFactory(cfg, flags, factory); got != 1 {
		t.Fatalf("handleSyncGitHubPublicWithFactory() = %d, want 1", got)
	}
	if factory.githubPublicRepoCalls != 1 {
		t.Fatalf("expected one GitHub public client factory call, got %d", factory.githubPublicRepoCalls)
	}
}

func TestHandleSyncCodebergPublicWithFactory_ReturnsErrorWhenFactoryReturnsNilClient(t *testing.T) {
	t.Parallel()

	factory := &stubRepoClientFactory{codebergPublicClient: nil}
	cfg := &config.Config{
		Organizations: []config.Organization{
			{Host: "git@codeberg.org", Name: "acme", CodebergToken: "cb-token"},
		},
		SyncCodeberg: true,
	}
	flags := &Flags{
		DryRun:           true,
		SyncGitHubPublic: false,
		WorkDir:          t.TempDir(),
	}

	if got := handleSyncCodebergPublicWithFactory(cfg, flags, factory); got != 1 {
		t.Fatalf("handleSyncCodebergPublicWithFactory() = %d, want 1", got)
	}
	if factory.codebergPublicCalls != 1 {
		t.Fatalf("expected one Codeberg public client factory call, got %d", factory.codebergPublicCalls)
	}
}

func TestInitGitHubClientWithFactory_Branches(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Organizations: []config.Organization{
			{Host: "git@github.com", Name: "acme", GitHubToken: "gh-token"},
		},
	}

	t.Run("returns client when token exists", func(t *testing.T) {
		t.Parallel()
		client := &stubDescriptionRepoClient{hasToken: true}
		factory := &stubRepoClientFactory{githubRepoClient: client}

		got := initGitHubClientWithFactory(cfg, factory)
		if got == nil {
			t.Fatal("expected GitHub client, got nil")
		}
		if factory.githubRepoCalls != 1 {
			t.Fatalf("github repo factory calls = %d, want 1", factory.githubRepoCalls)
		}
	})

	t.Run("returns nil without token", func(t *testing.T) {
		t.Parallel()
		factory := &stubRepoClientFactory{githubRepoClient: &stubDescriptionRepoClient{hasToken: false}}

		if got := initGitHubClientWithFactory(cfg, factory); got != nil {
			t.Fatal("expected nil GitHub client when token is missing")
		}
	})

	t.Run("returns nil when factory returns nil client", func(t *testing.T) {
		t.Parallel()
		factory := &stubRepoClientFactory{githubRepoClient: nil}

		if got := initGitHubClientWithFactory(cfg, factory); got != nil {
			t.Fatal("expected nil GitHub client when factory returns nil")
		}
	})
}

func TestInitCodebergClientWithFactory_Branches(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Organizations: []config.Organization{
			{Host: "git@codeberg.org", Name: "acme", CodebergToken: "cb-token"},
		},
		SyncCodeberg: true,
	}

	t.Run("returns client when token exists", func(t *testing.T) {
		t.Parallel()
		client := &stubDescriptionRepoClient{hasToken: true}
		factory := &stubRepoClientFactory{codebergRepoClient: client}

		got := initCodebergClientWithFactory(cfg, factory)
		if got == nil {
			t.Fatal("expected Codeberg client, got nil")
		}
		if factory.codebergRepoCalls != 1 {
			t.Fatalf("codeberg repo factory calls = %d, want 1", factory.codebergRepoCalls)
		}
	})

	t.Run("returns nil without token", func(t *testing.T) {
		t.Parallel()
		factory := &stubRepoClientFactory{codebergRepoClient: &stubDescriptionRepoClient{hasToken: false}}

		if got := initCodebergClientWithFactory(cfg, factory); got != nil {
			t.Fatal("expected nil Codeberg client when token is missing")
		}
	})

	t.Run("returns nil when factory returns nil client", func(t *testing.T) {
		t.Parallel()
		factory := &stubRepoClientFactory{codebergRepoClient: nil}

		if got := initCodebergClientWithFactory(cfg, factory); got != nil {
			t.Fatal("expected nil Codeberg client when factory returns nil")
		}
	})
}
