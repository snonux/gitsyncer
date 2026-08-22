package cli

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/snonux/gitsyncer/internal/config"
	"github.com/snonux/gitsyncer/internal/forge"
)

// captureStdout runs fn while redirecting os.Stdout, returning the captured
// output. Used by handler tests that need to assert on user-facing messages.
//
// os.Stdout is a single process-global variable, so swapping it here is not
// safe under concurrent use: if two tests call captureStdout at the same
// time (e.g. both marked t.Parallel()), their swap/restore calls race and
// each test can end up capturing a mix of its own and the other test's
// output. There is no per-goroutine os.Stdout to scope this to, so callers
// of captureStdout must not run in parallel with each other — do not add
// t.Parallel() to tests that call this helper (see the callers below for
// the reasoning inline).
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

// stubDescriptionRepoClient fakes the concrete client that
// ForgeClientResolver.ClientFor would normally construct (github.Client or
// codeberg.Client). Since ClientFor hands back the same forge.RepoDescriptionClient
// value for every role (create/delete/exists, description get/update), a
// single stub covers both roles instead of separate per-role stubs.
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

// stubForgeClientResolver fakes ForgeClientResolver, the seam replacing the
// old repoClientFactory. Unlike the fat factory (which had a separate method
// per forge x role combination), the resolver dispatches purely on
// org.IsGitHub()/IsCodeberg()/IsForgejo() and hands back one client per org
// regardless of which role (RepoClient vs RepoDescriptionClient) the caller
// actually needs - so this stub only needs one client field per forge kind.
type stubForgeClientResolver struct {
	githubClient   forge.RepoDescriptionClient
	codebergClient forge.RepoDescriptionClient
	forgejoClient  forge.RepoDescriptionClient

	githubCalls   int
	codebergCalls int
	githubToken   string
	githubOrg     string
	codebergToken string
	codebergOrg   string
}

func (s *stubForgeClientResolver) ClientFor(org *config.Organization) (forge.RepoDescriptionClient, bool) {
	switch {
	case org.IsGitHub():
		s.githubCalls++
		s.githubToken = org.GitHubToken
		s.githubOrg = org.Name
		return s.githubClient, s.githubClient != nil
	case org.IsCodeberg():
		s.codebergCalls++
		s.codebergToken = org.CodebergToken
		s.codebergOrg = org.Name
		return s.codebergClient, s.codebergClient != nil
	case org.IsForgejo():
		return s.forgejoClient, s.forgejoClient != nil
	default:
		return nil, false
	}
}

// stubGitHubPublicRepoLister fakes the forge.PublicRepoLister that
// newGitHubPublicRepoLister would normally build, returning forge-agnostic
// forge.PublicRepo values instead of the concrete github.Repository DTO.
type stubGitHubPublicRepoLister struct {
	hasToken bool
	repos    []forge.PublicRepo
}

func (s *stubGitHubPublicRepoLister) HasToken() bool { return s.hasToken }

func (s *stubGitHubPublicRepoLister) ListPublicRepos() ([]forge.PublicRepo, error) {
	return s.repos, nil
}

// stubCodebergPublicRepoLister fakes the forge.UserFallbackPublicRepoLister
// that newCodebergPublicRepoLister would normally build.
type stubCodebergPublicRepoLister struct {
	orgRepos  []forge.PublicRepo
	userRepos []forge.PublicRepo
	orgErr    error
	userErr   error
}

func (s *stubCodebergPublicRepoLister) ListPublicRepos() ([]forge.PublicRepo, error) {
	return s.orgRepos, s.orgErr
}

func (s *stubCodebergPublicRepoLister) ListUserPublicRepos() ([]forge.PublicRepo, error) {
	return s.userRepos, s.userErr
}

func TestSyncRepoDescriptionsWithResolver_UsesInjectedClients(t *testing.T) {
	t.Parallel()

	githubClient := &stubDescriptionRepoClient{
		hasToken:          true,
		descriptionByRepo: map[string]string{"demo": "from github"},
	}
	codebergClient := &stubDescriptionRepoClient{
		hasToken:          true,
		descriptionByRepo: map[string]string{"demo": "from codeberg"},
	}
	resolver := &stubForgeClientResolver{
		githubClient:   githubClient,
		codebergClient: codebergClient,
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

	syncRepoDescriptionsWithResolver(cfg, false, nil, nil, "demo", "", "", cache, resolver)

	if resolver.githubCalls != 1 || resolver.codebergCalls != 1 {
		t.Fatalf("expected one injected client resolution per forge, got github=%d codeberg=%d", resolver.githubCalls, resolver.codebergCalls)
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

func TestCreateRepoHelpersWithResolver_UseInjectedCreateClients(t *testing.T) {
	t.Parallel()

	githubClient := &stubDescriptionRepoClient{hasToken: true}
	codebergClient := &stubDescriptionRepoClient{hasToken: true}
	resolver := &stubForgeClientResolver{
		githubClient:   githubClient,
		codebergClient: codebergClient,
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

	if err := createGitHubRepoIfNeededWithResolver(cfg, "demo", false, resolver); err != nil {
		t.Fatalf("createGitHubRepoIfNeededWithResolver() error = %v", err)
	}
	if err := createCodebergRepoIfNeededWithResolver(cfg, "demo", false, resolver); err != nil {
		t.Fatalf("createCodebergRepoIfNeededWithResolver() error = %v", err)
	}

	if resolver.githubCalls != 1 || resolver.codebergCalls != 1 {
		t.Fatalf("expected one injected client resolution per forge, got github=%d codeberg=%d", resolver.githubCalls, resolver.codebergCalls)
	}
	if resolver.githubToken != "gh-token" || resolver.githubOrg != "acme" {
		t.Fatalf("github client resolve args = (%q, %q), want (%q, %q)", resolver.githubToken, resolver.githubOrg, "gh-token", "acme")
	}
	if resolver.codebergToken != "cb-token" || resolver.codebergOrg != "acme" {
		t.Fatalf("codeberg client resolve args = (%q, %q), want (%q, %q)", resolver.codebergToken, resolver.codebergOrg, "cb-token", "acme")
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

func TestCreateRepoHelpersWithResolver_DryRunDispatchesNoMutations(t *testing.T) {
	t.Parallel()

	githubClient := &stubDescriptionRepoClient{hasToken: true}
	codebergClient := &stubDescriptionRepoClient{hasToken: true}
	resolver := &stubForgeClientResolver{
		githubClient:   githubClient,
		codebergClient: codebergClient,
	}
	cfg := &config.Config{
		Organizations: []config.Organization{
			{Host: "git@github.com", Name: "acme", GitHubToken: "gh-token"},
			{Host: "git@codeberg.org", Name: "acme", CodebergToken: "cb-token"},
		},
		Repositories: []string{"demo"},
		SyncCodeberg: true,
	}

	if err := createGitHubRepoIfNeededWithResolver(cfg, "demo", true, resolver); err != nil {
		t.Fatalf("dry-run GitHub create returned error: %v", err)
	}
	if err := createCodebergRepoIfNeededWithResolver(cfg, "demo", true, resolver); err != nil {
		t.Fatalf("dry-run Codeberg create returned error: %v", err)
	}
	if len(githubClient.createCalls) != 0 || len(codebergClient.createCalls) != 0 {
		t.Fatalf("dry run dispatched create mutations: github=%d codeberg=%d", len(githubClient.createCalls), len(codebergClient.createCalls))
	}
	if resolver.githubCalls != 0 || resolver.codebergCalls != 0 {
		t.Fatalf("dry run resolved mutation clients: github=%d codeberg=%d", resolver.githubCalls, resolver.codebergCalls)
	}
}

func TestSyncCodebergRepos_DryRunCreateReposDispatchesNoMutations(t *testing.T) {
	t.Parallel()

	client := &stubDescriptionRepoClient{hasToken: true}
	resolver := &stubForgeClientResolver{githubClient: client}
	flags := &Flags{DryRun: true, CreateGitHubRepos: true, WorkDir: t.TempDir()}

	if got := syncCodebergRepos(&config.Config{}, flags, nil, []string{"demo"}, resolver); got != 0 {
		t.Fatalf("syncCodebergRepos() = %d, want 0", got)
	}
	if resolver.githubCalls != 0 {
		t.Fatalf("dry run resolved %d GitHub mutation clients, want 0", resolver.githubCalls)
	}
	if len(client.createCalls) != 0 {
		t.Fatalf("dry run dispatched %d GitHub create calls, want 0", len(client.createCalls))
	}
}

func TestSyncGitHubRepos_DryRunCreateReposDispatchesNoMutations(t *testing.T) {
	t.Parallel()

	client := &stubDescriptionRepoClient{hasToken: true}
	resolver := &stubForgeClientResolver{codebergClient: client}
	flags := &Flags{DryRun: true, CreateCodebergRepos: true, WorkDir: t.TempDir()}

	if got := syncGitHubRepos(&config.Config{}, flags, nil, []string{"demo"}, resolver); got != 0 {
		t.Fatalf("syncGitHubRepos() = %d, want 0", got)
	}
	if resolver.codebergCalls != 0 {
		t.Fatalf("dry run resolved %d Codeberg mutation clients, want 0", resolver.codebergCalls)
	}
	if len(client.createCalls) != 0 {
		t.Fatalf("dry run dispatched %d Codeberg create calls, want 0", len(client.createCalls))
	}
}

// TestSyncCodebergRepos_PrintsSeparatorWhenChainingIntoGitHubSync covers the
// forgeSyncSpec.afterSync branch that is unique to syncCodebergRepos: when
// SyncGitHubPublic is set, the full-sync separator should print after the
// Codeberg->GitHub loop so combined log output reads as two phases.
// syncGitHubRepos has no equivalent afterSync behavior (its spec always
// returns 0 without printing), which TestSyncGitHubRepos_DryRunCreateReposDispatchesNoMutations
// already exercises.
func TestSyncCodebergRepos_PrintsSeparatorWhenChainingIntoGitHubSync(t *testing.T) {
	// Not t.Parallel(): this test calls captureStdout, which swaps the
	// process-global os.Stdout for the duration of fn(). Running it
	// concurrently with another captureStdout-using test races on that
	// global and cross-contaminates captured output between tests.

	resolver := &stubForgeClientResolver{}
	flags := &Flags{DryRun: true, SyncGitHubPublic: true, WorkDir: t.TempDir()}
	repos := []forge.PublicRepo{{Name: "demo", Description: "demo repo"}}

	out := captureStdout(t, func() {
		if got := syncCodebergRepos(&config.Config{}, flags, repos, []string{"demo"}, resolver); got != 0 {
			t.Fatalf("syncCodebergRepos() = %d, want 0", got)
		}
	})

	if !strings.Contains(out, "Continuing with GitHub to Codeberg sync") {
		t.Fatalf("expected full-sync separator in output, got:\n%s", out)
	}
}

// TestSyncCodebergRepos_NoSeparatorWithoutChainedGitHubSync confirms the
// separator is only printed when SyncGitHubPublic chains into a follow-on
// GitHub sync, not on every syncCodebergRepos call.
func TestSyncCodebergRepos_NoSeparatorWithoutChainedGitHubSync(t *testing.T) {
	// Not t.Parallel(): see the comment in
	// TestSyncCodebergRepos_PrintsSeparatorWhenChainingIntoGitHubSync above
	// — this test also uses captureStdout, which is unsafe to run
	// concurrently with other captureStdout callers.

	resolver := &stubForgeClientResolver{}
	flags := &Flags{DryRun: true, SyncGitHubPublic: false, WorkDir: t.TempDir()}
	repos := []forge.PublicRepo{{Name: "demo", Description: "demo repo"}}

	out := captureStdout(t, func() {
		if got := syncCodebergRepos(&config.Config{}, flags, repos, []string{"demo"}, resolver); got != 0 {
			t.Fatalf("syncCodebergRepos() = %d, want 0", got)
		}
	})

	if strings.Contains(out, "Continuing with GitHub to Codeberg sync") {
		t.Fatalf("did not expect full-sync separator in output, got:\n%s", out)
	}
}

func TestHandleSyncGitHubPublicWithDeps_UsesInjectedLister(t *testing.T) {
	t.Parallel()

	lister := &stubGitHubPublicRepoLister{
		hasToken: true,
		repos:    []forge.PublicRepo{{Name: "demo"}},
	}
	var listerCalls int
	var gotToken, gotOrg string
	newLister := func(org config.Organization) forge.PublicRepoLister {
		listerCalls++
		gotToken = org.GitHubToken
		gotOrg = org.Name
		return lister
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

	if got := handleSyncGitHubPublicWithDeps(cfg, flags, newLister, &stubForgeClientResolver{}); got != 0 {
		t.Fatalf("handleSyncGitHubPublicWithDeps() = %d, want 0", got)
	}
	if listerCalls != 1 {
		t.Fatalf("expected exactly one injected GitHub lister construction, got %d", listerCalls)
	}
	if gotToken != "gh-token" || gotOrg != "acme" {
		t.Fatalf("github lister construction args = (%q, %q), want (%q, %q)", gotToken, gotOrg, "gh-token", "acme")
	}
}

func TestHandleSyncCodebergPublicWithDeps_UsesInjectedLister(t *testing.T) {
	t.Parallel()

	lister := &stubCodebergPublicRepoLister{
		orgErr:    errors.New("org lookup failed"),
		userRepos: []forge.PublicRepo{{Name: "demo"}},
	}
	var listerCalls int
	var gotToken, gotOrg string
	newLister := func(org config.Organization) forge.UserFallbackPublicRepoLister {
		listerCalls++
		gotToken = org.CodebergToken
		gotOrg = org.Name
		return lister
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

	if got := handleSyncCodebergPublicWithDeps(cfg, flags, newLister, &stubForgeClientResolver{}); got != 0 {
		t.Fatalf("handleSyncCodebergPublicWithDeps() = %d, want 0", got)
	}
	if listerCalls != 1 {
		t.Fatalf("expected exactly one injected Codeberg lister construction, got %d", listerCalls)
	}
	if gotToken != "cb-token" || gotOrg != "acme" {
		t.Fatalf("codeberg lister construction args = (%q, %q), want (%q, %q)", gotToken, gotOrg, "cb-token", "acme")
	}
}

// TestHandleSyncCodebergPublicWithDeps_DryRunChainedFullSyncStillReturnsZero
// covers the SyncGitHubPublic:true dry-run path. Before the l01 dedup, this
// branch fell through the (now-removed) redundant `if !flags.SyncGitHubPublic
// { return 0 }` check inside the dry-run block without ever reaching the
// syncCodebergRepos dispatch; the observable result was always 0 either way.
// This test locks in that the merged publicSyncPipeline.run preserves that
// same outcome.
func TestHandleSyncCodebergPublicWithDeps_DryRunChainedFullSyncStillReturnsZero(t *testing.T) {
	// Not t.Parallel(): see the comment in
	// TestSyncCodebergRepos_PrintsSeparatorWhenChainingIntoGitHubSync above
	// — this test also uses captureStdout, which is unsafe to run
	// concurrently with other captureStdout callers.

	lister := &stubCodebergPublicRepoLister{
		orgRepos: []forge.PublicRepo{{Name: "demo"}},
	}
	newLister := func(config.Organization) forge.UserFallbackPublicRepoLister { return lister }
	cfg := &config.Config{
		Organizations: []config.Organization{
			{Host: "git@codeberg.org", Name: "acme", CodebergToken: "cb-token"},
		},
		SyncCodeberg: true,
	}
	flags := &Flags{
		DryRun:           true,
		SyncGitHubPublic: true,
		WorkDir:          t.TempDir(),
	}

	out := captureStdout(t, func() {
		if got := handleSyncCodebergPublicWithDeps(cfg, flags, newLister, &stubForgeClientResolver{}); got != 0 {
			t.Fatalf("handleSyncCodebergPublicWithDeps() = %d, want 0", got)
		}
	})

	if !strings.Contains(out, "[DRY RUN] Would sync 1 repositories from Codeberg to GitHub") {
		t.Fatalf("expected dry-run summary in output, got:\n%s", out)
	}
}

func TestHandleSyncCodebergPublicWithDeps_RestrictsToConfiguredRepos(t *testing.T) {
	// Not t.Parallel(): see the comment in
	// TestSyncCodebergRepos_PrintsSeparatorWhenChainingIntoGitHubSync above
	// — this test also uses captureStdout, which is unsafe to run
	// concurrently with other captureStdout callers.

	lister := &stubCodebergPublicRepoLister{
		orgRepos: []forge.PublicRepo{
			{Name: "wanted"},
			{Name: "also-wanted"},
			{Name: "not-configured"},
		},
	}
	newLister := func(config.Organization) forge.UserFallbackPublicRepoLister { return lister }
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
		if got := handleSyncCodebergPublicWithDeps(cfg, flags, newLister, &stubForgeClientResolver{}); got != 0 {
			t.Fatalf("handleSyncCodebergPublicWithDeps() = %d, want 0", got)
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

func TestHandleSyncCodebergPublicWithDeps_SkipsWhenCodebergSyncDisabled(t *testing.T) {
	t.Parallel()

	lister := &stubCodebergPublicRepoLister{
		userRepos: []forge.PublicRepo{{Name: "demo"}},
	}
	var listerCalls int
	newLister := func(config.Organization) forge.UserFallbackPublicRepoLister {
		listerCalls++
		return lister
	}
	// A Codeberg org is configured, but sync_codeberg is not enabled.
	cfg := &config.Config{
		Organizations: []config.Organization{
			{Host: "git@codeberg.org", Name: "acme", CodebergToken: "cb-token"},
		},
	}
	flags := &Flags{DryRun: true, WorkDir: t.TempDir()}

	if got := handleSyncCodebergPublicWithDeps(cfg, flags, newLister, &stubForgeClientResolver{}); got != 0 {
		t.Fatalf("handleSyncCodebergPublicWithDeps() = %d, want 0 when Codeberg sync disabled", got)
	}
	if listerCalls != 0 {
		t.Fatalf("expected no Codeberg lister construction when disabled, got %d", listerCalls)
	}
}

func TestCreateCodebergRepoIfNeededWithResolver_SkipsWhenCodebergSyncDisabled(t *testing.T) {
	t.Parallel()

	resolver := &stubForgeClientResolver{
		codebergClient: &stubDescriptionRepoClient{hasToken: true},
	}
	cfg := &config.Config{
		Organizations: []config.Organization{
			{Host: "git@codeberg.org", Name: "acme", CodebergToken: "cb-token"},
		},
	}

	if err := createCodebergRepoIfNeededWithResolver(cfg, "demo", false, resolver); err != nil {
		t.Fatalf("createCodebergRepoIfNeededWithResolver() error = %v", err)
	}
	if resolver.codebergCalls != 0 {
		t.Fatalf("expected no Codeberg client resolution when disabled, got %d", resolver.codebergCalls)
	}
}

func TestHandleSyncGitHubPublicWithDeps_ReturnsErrorWhenListerIsNil(t *testing.T) {
	t.Parallel()

	newLister := func(config.Organization) forge.PublicRepoLister { return nil }
	cfg := &config.Config{
		Organizations: []config.Organization{
			{Host: "git@github.com", Name: "acme", GitHubToken: "gh-token"},
		},
	}
	flags := &Flags{
		DryRun:  true,
		WorkDir: t.TempDir(),
	}

	if got := handleSyncGitHubPublicWithDeps(cfg, flags, newLister, &stubForgeClientResolver{}); got != 1 {
		t.Fatalf("handleSyncGitHubPublicWithDeps() = %d, want 1", got)
	}
}

func TestHandleSyncCodebergPublicWithDeps_ReturnsErrorWhenListerIsNil(t *testing.T) {
	t.Parallel()

	newLister := func(config.Organization) forge.UserFallbackPublicRepoLister { return nil }
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

	if got := handleSyncCodebergPublicWithDeps(cfg, flags, newLister, &stubForgeClientResolver{}); got != 1 {
		t.Fatalf("handleSyncCodebergPublicWithDeps() = %d, want 1", got)
	}
}

func TestInitGitHubClientWithResolver_Branches(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Organizations: []config.Organization{
			{Host: "git@github.com", Name: "acme", GitHubToken: "gh-token"},
		},
	}

	t.Run("returns client when token exists", func(t *testing.T) {
		t.Parallel()
		client := &stubDescriptionRepoClient{hasToken: true}
		resolver := &stubForgeClientResolver{githubClient: client}

		got := initGitHubClientWithResolver(cfg, resolver)
		if got == nil {
			t.Fatal("expected GitHub client, got nil")
		}
		if resolver.githubCalls != 1 {
			t.Fatalf("github client resolutions = %d, want 1", resolver.githubCalls)
		}
	})

	t.Run("returns nil without token", func(t *testing.T) {
		t.Parallel()
		resolver := &stubForgeClientResolver{githubClient: &stubDescriptionRepoClient{hasToken: false}}

		if got := initGitHubClientWithResolver(cfg, resolver); got != nil {
			t.Fatal("expected nil GitHub client when token is missing")
		}
	})

	t.Run("returns nil when resolver has no client", func(t *testing.T) {
		t.Parallel()
		resolver := &stubForgeClientResolver{githubClient: nil}

		if got := initGitHubClientWithResolver(cfg, resolver); got != nil {
			t.Fatal("expected nil GitHub client when resolver returns none")
		}
	})
}

func TestInitCodebergClientWithResolver_Branches(t *testing.T) {
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
		resolver := &stubForgeClientResolver{codebergClient: client}

		got := initCodebergClientWithResolver(cfg, resolver)
		if got == nil {
			t.Fatal("expected Codeberg client, got nil")
		}
		if resolver.codebergCalls != 1 {
			t.Fatalf("codeberg client resolutions = %d, want 1", resolver.codebergCalls)
		}
	})

	t.Run("returns nil without token", func(t *testing.T) {
		t.Parallel()
		resolver := &stubForgeClientResolver{codebergClient: &stubDescriptionRepoClient{hasToken: false}}

		if got := initCodebergClientWithResolver(cfg, resolver); got != nil {
			t.Fatal("expected nil Codeberg client when token is missing")
		}
	})

	t.Run("returns nil when resolver has no client", func(t *testing.T) {
		t.Parallel()
		resolver := &stubForgeClientResolver{codebergClient: nil}

		if got := initCodebergClientWithResolver(cfg, resolver); got != nil {
			t.Fatal("expected nil Codeberg client when resolver returns none")
		}
	})
}
