package cli

import (
	"fmt"
	"math/rand"
	"strings"

	"codeberg.org/snonux/gitsyncer/internal/config"
	"codeberg.org/snonux/gitsyncer/internal/forge"
	"codeberg.org/snonux/gitsyncer/internal/state"
	"codeberg.org/snonux/gitsyncer/internal/sync"
)

func shouldEnableBackupSync(flags *Flags) bool {
	if flags == nil {
		return false
	}

	return flags.Backup || flags.FullSync || flags.SyncCodebergPublic || flags.SyncGitHubPublic
}

// HandleSync handles syncing a single repository
func HandleSync(cfg *config.Config, flags *Flags) int {
	stateManager, syncState, err := loadSyncState(flags.WorkDir)
	if err != nil {
		fmt.Printf("Warning: Failed to load sync state: %v\n", err)
	}

	decision := evaluateSyncPolicy(flags.SyncRepo, flags.WorkDir, syncState, flags.DryRun, flags.Force, flags.Throttle)
	if decision.Message != "" {
		fmt.Println(decision.Message)
	}
	if decision.SetNextAllowed && stateManager != nil && !flags.DryRun {
		syncState.SetNextRepoSyncAllowed(flags.SyncRepo, decision.NextAllowed)
		if err := stateManager.Save(syncState); err != nil {
			fmt.Printf("Warning: Failed to save sync state: %v\n", err)
		}
	}
	if decision.Skip {
		return 0
	}

	// If create-github-repos is enabled, create the repo if needed
	if flags.CreateGitHubRepos && !flags.DryRun {
		if err := createGitHubRepoIfNeeded(cfg, flags.SyncRepo, flags.DryRun); err != nil {
			fmt.Printf("ERROR: %v\n", err)
			return 1
		}
	}

	// If create-codeberg-repos is enabled, create the repo if needed
	if flags.CreateCodebergRepos && !flags.DryRun {
		if err := createCodebergRepoIfNeeded(cfg, flags.SyncRepo, flags.DryRun); err != nil {
			fmt.Printf("ERROR: %v\n", err)
			return 1
		}
	}

	syncer := sync.New(cfg, flags.WorkDir)
	syncer.SetBackupEnabled(shouldEnableBackupSync(flags))
	syncer.SetDryRun(flags.DryRun)
	syncer.SetForgejoBackupClientFactory(newForgejoBackupClient)
	if err := syncer.SyncRepository(flags.SyncRepo); err != nil {
		fmt.Printf("ERROR: Sync failed: %v\n", err)
		return 1
	}

	if stateManager != nil && !flags.DryRun {
		recordRepoSync(flags.SyncRepo, syncState, flags.Throttle)
		if err := stateManager.Save(syncState); err != nil {
			fmt.Printf("Warning: Failed to save sync state: %v\n", err)
		}
	}

	// Also sync descriptions for this single repository
	descCache := loadDescriptionCache(flags.WorkDir)
	syncRepoDescriptions(cfg, flags.DryRun, syncer.BackupActive, syncer.DisableBackup, flags.SyncRepo, "", "", descCache)
	if !flags.DryRun {
		if err := saveDescriptionCache(flags.WorkDir, descCache); err != nil {
			fmt.Printf("Warning: Failed to save descriptions cache: %v\n", err)
		}
	}
	return 0
}

// HandleSyncAll handles syncing all statically configured repositories. A
// per-repo mirror-creation or SyncRepository failure is recorded and the
// batch continues with the next repo (see syncExecution.recordFailure) -
// only the config-level "no repositories configured" check above aborts
// immediately, since that is an unrecoverable setup error rather than a
// per-repo one. The final exit code reflects whether any repo failed.
func HandleSyncAll(cfg *config.Config, flags *Flags) int {
	if len(cfg.Repositories) == 0 {
		fmt.Println("No repositories configured. Add repositories to the config file.")
		return 1
	}

	repoNames := shuffledRepoNames(cfg.Repositories)
	execution := newSyncExecution(cfg, flags)
	githubClient, codebergClient := configuredRepoCreationClients(cfg, flags)

	successCount := 0
	for i, repo := range repoNames {
		fmt.Printf("\n[%d/%d] Syncing %s...\n", i+1, len(repoNames), repo)

		if execution.maybeSkipRepo(repo, flags) {
			continue
		}

		createConfiguredMirrors(repo, githubClient, codebergClient)

		if err := execution.syncer.SyncRepository(repo); err != nil {
			execution.recordFailure(repo, err)
			continue
		}
		execution.markRepoSynced(repo, flags)
		successCount++

		// Sync descriptions after repo sync
		syncRepoDescriptions(cfg, flags.DryRun, execution.syncer.BackupActive, execution.syncer.DisableBackup, repo, "", "", execution.descCache)
	}

	finishConfiguredSync(execution, successCount, flags)
	return execution.exitCode()
}

// configuredRepoCreationClients initializes the optional GitHub/Codeberg
// clients used to create missing mirror repos before syncing each
// statically configured repository; nil when the corresponding
// --create-*-repos flag wasn't set (or this is a dry run).
func configuredRepoCreationClients(cfg *config.Config, flags *Flags) (githubClient, codebergClient forge.RepoClient) {
	if flags.CreateGitHubRepos && !flags.DryRun {
		githubClient = initGitHubClient(cfg)
	}
	if flags.CreateCodebergRepos && !flags.DryRun {
		codebergClient = initCodebergClient(cfg)
	}
	return githubClient, codebergClient
}

// createConfiguredMirrors creates repo's GitHub/Codeberg mirror if the
// corresponding client was configured. A creation failure is only a
// warning: sync still proceeds to attempt SyncRepository for repo, since a
// failed mirror-creation call is a recoverable per-repo issue, not the kind
// of unrecoverable config/state error that should stop the whole batch.
func createConfiguredMirrors(repo string, githubClient, codebergClient forge.RepoClient) {
	if githubClient != nil {
		if err := createRepoWithClient(githubClient, repo, fmt.Sprintf("Mirror of %s", repo)); err != nil {
			fmt.Printf("Warning: Failed to create GitHub repo %s: %v\n", repo, err)
		}
	}

	if codebergClient != nil {
		fmt.Printf("Checking/creating Codeberg repository %s...\n", repo)
		if err := codebergClient.CreateRepo(repo, fmt.Sprintf("Mirror of %s", repo), false); err != nil {
			fmt.Printf("Warning: Failed to create Codeberg repo %s: %v\n", repo, err)
		}
	}
}

// finishConfiguredSync saves the description cache and prints the batch
// summary (success count, any per-repo failures, abandoned branches, delete
// script) once the configured-repos loop finishes. It mirrors
// syncExecution.finishDiscoveredSync, which does the same for the
// discovered-repos loop in syncDiscoveredRepos.
func finishConfiguredSync(execution *syncExecution, successCount int, flags *Flags) {
	if !flags.DryRun {
		if err := saveDescriptionCache(flags.WorkDir, execution.descCache); err != nil {
			fmt.Printf("Warning: Failed to save descriptions cache: %v\n", err)
		}
	}

	if len(execution.failedRepos) == 0 {
		fmt.Printf("\nSuccessfully synced all %d repositories!\n", successCount)
	} else {
		fmt.Printf("\nSynced %d of %d repositories.\n", successCount, successCount+len(execution.failedRepos))
	}
	execution.printFailureSummary()

	if summary := execution.syncer.GenerateAbandonedBranchSummary(); summary != "" {
		fmt.Print(summary)
	}

	if !flags.DryRun {
		printDeleteScript(execution.syncer)
	}
}

// HandleSyncCodebergPublic handles syncing all public Codeberg repositories
func HandleSyncCodebergPublic(cfg *config.Config, flags *Flags) int {
	return handleSyncCodebergPublicWithDeps(cfg, flags, newCodebergPublicRepoLister, ForgeClientResolver{})
}

// handleSyncCodebergPublicWithDeps discovers Codeberg's public repositories
// and hands them to the shared fetch+allowlist+dryrun+shuffle pipeline
// (publicSyncPipeline.run). Only the Codeberg-specific bits live here: the
// sync_codeberg config gate, org lookup, client construction, and the
// org-then-user listing fallback that Codeberg's API requires. newLister
// builds the public-repo-listing client (injected so tests can fake it);
// resolver builds the mirror-side create/update client that
// syncCodebergRepos may need if --create-github-repos is set.
func handleSyncCodebergPublicWithDeps(cfg *config.Config, flags *Flags, newLister func(config.Organization) forge.UserFallbackPublicRepoLister, resolver forgeClientResolver) int {
	if !cfg.CodebergSyncEnabled() {
		fmt.Println("Codeberg sync is disabled in config (set \"sync_codeberg\": true to enable)")
		return 0
	}

	codebergOrg := cfg.FindCodebergOrg()
	if codebergOrg == nil {
		fmt.Println("No Codeberg organization found in configuration")
		return 1
	}

	fmt.Printf("Fetching public repositories from Codeberg user/org: %s...\n", codebergOrg.Name)

	client := newLister(*codebergOrg)
	if client == nil {
		fmt.Println("ERROR: Failed to initialize Codeberg public repository client")
		return 1
	}

	// Try fetching as organization first, then as user
	repos, err := client.ListPublicRepos()
	if err != nil {
		fmt.Println("Trying as user account...")
		repos, err = client.ListUserPublicRepos()
		if err != nil {
			fmt.Printf("ERROR: Failed to fetch repositories: %v\n", err)
			return 1
		}
	}

	pipeline := publicSyncPipeline{
		sourceLabel:  "Codeberg",
		targetLabel:  "GitHub",
		createTarget: flags.CreateGitHubRepos,
		sync: func(repoNames []string) int {
			return syncCodebergRepos(cfg, flags, repos, repoNames, resolver)
		},
	}

	return pipeline.run(cfg, flags, publicRepoNames(repos))
}

// HandleSyncGitHubPublic handles syncing all public GitHub repositories
func HandleSyncGitHubPublic(cfg *config.Config, flags *Flags) int {
	return handleSyncGitHubPublicWithDeps(cfg, flags, newGitHubPublicRepoLister, ForgeClientResolver{})
}

// handleSyncGitHubPublicWithDeps mirrors handleSyncCodebergPublicWithDeps
// for GitHub: it resolves the GitHub org, requires a token (GitHub's public
// listing API needs one even for public repos, unlike Codeberg's), fetches the
// repos, then delegates the shared post-processing to publicSyncPipeline.run.
func handleSyncGitHubPublicWithDeps(cfg *config.Config, flags *Flags, newLister func(config.Organization) forge.PublicRepoLister, resolver forgeClientResolver) int {
	githubOrg := cfg.FindGitHubOrg()
	if githubOrg == nil {
		fmt.Println("No GitHub organization found in configuration")
		return 1
	}

	fmt.Printf("Fetching public repositories from GitHub user/org: %s...\n", githubOrg.Name)

	client := newLister(*githubOrg)
	if client == nil {
		fmt.Println("ERROR: Failed to initialize GitHub public repository client")
		return 1
	}
	if !client.HasToken() {
		fmt.Println("ERROR: GitHub token required to list repositories")
		fmt.Println("Set GITHUB_TOKEN env var or create ~/.gitsyncer_github_token file")
		return 1
	}

	repos, err := client.ListPublicRepos()
	if err != nil {
		fmt.Printf("ERROR: Failed to fetch repositories: %v\n", err)
		return 1
	}

	pipeline := publicSyncPipeline{
		sourceLabel:  "GitHub",
		targetLabel:  "Codeberg",
		createTarget: flags.CreateCodebergRepos,
		sync: func(repoNames []string) int {
			return syncGitHubRepos(cfg, flags, repos, repoNames, resolver)
		},
	}

	return pipeline.run(cfg, flags, publicRepoNames(repos))
}

// publicSyncPipeline is the fetch+allowlist+dryrun+shuffle pipeline shared by
// handleSyncCodebergPublicWithFactory and handleSyncGitHubPublicWithFactory.
// Each caller has already fetched its platform-specific repo list; this type
// only handles the identical post-processing: reporting the fetch count,
// restricting to the configured allowlist, filtering out throttled repos on
// dry runs, shuffling sync order, printing the repo list, and either printing
// a dry-run summary or dispatching to the platform's sync function.
//
// Note: the original Codeberg handler had a redundant `if !flags.SyncGitHubPublic
// { return 0 }` inside its dry-run branch that always fell through to the same
// `return 0` either way (nothing observable happened in between) - that dead
// branch is dropped here since it had no behavioral effect.
type publicSyncPipeline struct {
	sourceLabel  string                       // e.g. "Codeberg"; used in log/dry-run messages
	targetLabel  string                       // e.g. "GitHub"; the mirror destination
	createTarget bool                         // flags.CreateGitHubRepos / flags.CreateCodebergRepos
	sync         func(repoNames []string) int // dispatches to syncCodebergRepos/syncGitHubRepos
}

func (p publicSyncPipeline) run(cfg *config.Config, flags *Flags, repoNames []string) int {
	fmt.Printf("Found %d public repositories on %s\n", len(repoNames), p.sourceLabel)

	if before := len(repoNames); before > 0 {
		repoNames = cfg.FilterSyncRepos(repoNames)
		if len(repoNames) < before {
			fmt.Printf("Restricted to %d configured repositories (allowlist)\n", len(repoNames))
		}
	}

	if len(repoNames) == 0 {
		fmt.Println("No public repositories found")
		return 0
	}

	if flags.DryRun {
		repoNames = filterDryRunRepoNames(repoNames, flags)
	}

	repoNames = shuffledRepoNames(repoNames)

	// Show the repositories that will be synced
	showReposToSync(repoNames)

	if flags.DryRun {
		fmt.Printf("\n[DRY RUN] Would sync %d repositories from %s to %s\n", len(repoNames), p.sourceLabel, p.targetLabel)
		if p.createTarget {
			fmt.Printf("Would create missing %s repositories\n", p.targetLabel)
		}
		return 0
	}

	return p.sync(repoNames)
}

// Helper functions

func createGitHubRepoIfNeeded(cfg *config.Config, repoName string, dryRun bool) error {
	return createGitHubRepoIfNeededWithResolver(cfg, repoName, dryRun, ForgeClientResolver{})
}

func createGitHubRepoIfNeededWithResolver(cfg *config.Config, repoName string, dryRun bool, resolver forgeClientResolver) error {
	if dryRun {
		return nil
	}
	githubOrg := cfg.FindGitHubOrg()
	if githubOrg == nil {
		return nil
	}

	fmt.Printf("Initializing GitHub client for organization: %s\n", githubOrg.Name)
	githubClient, ok := resolver.ClientFor(githubOrg)
	if !ok || !githubClient.HasToken() {
		fmt.Println("Warning: No GitHub token found. Cannot create repository.")
		return nil
	}

	fmt.Println("Checking/creating GitHub repository...")
	return githubClient.CreateRepo(repoName, fmt.Sprintf("Mirror of %s", repoName), false)
}

func createCodebergRepoIfNeeded(cfg *config.Config, repoName string, dryRun bool) error {
	return createCodebergRepoIfNeededWithResolver(cfg, repoName, dryRun, ForgeClientResolver{})
}

func createCodebergRepoIfNeededWithResolver(cfg *config.Config, repoName string, dryRun bool, resolver forgeClientResolver) error {
	if dryRun || !cfg.CodebergSyncEnabled() || !cfg.IsSyncRepo(repoName) {
		return nil
	}

	codebergOrg := cfg.FindCodebergOrg()
	if codebergOrg == nil {
		return nil
	}

	fmt.Printf("Initializing Codeberg client for organization: %s\n", codebergOrg.Name)
	codebergClient, ok := resolver.ClientFor(codebergOrg)
	if !ok || !codebergClient.HasToken() {
		fmt.Println("Warning: No Codeberg token found. Cannot create repository.")
		return nil
	}

	fmt.Println("Checking/creating Codeberg repository...")
	return codebergClient.CreateRepo(repoName, fmt.Sprintf("Mirror of %s", repoName), false)
}

func initGitHubClient(cfg *config.Config) forge.RepoClient {
	return initGitHubClientWithResolver(cfg, ForgeClientResolver{})
}

func initGitHubClientWithResolver(cfg *config.Config, resolver forgeClientResolver) forge.RepoClient {
	githubOrg := cfg.FindGitHubOrg()
	if githubOrg == nil {
		fmt.Println("Warning: --create-github-repos specified but no GitHub organization found in config")
		return nil
	}

	fmt.Printf("Initializing GitHub client for organization: %s\n", githubOrg.Name)
	githubClient, ok := resolver.ClientFor(githubOrg)
	if !ok {
		fmt.Println("Warning: GitHub client initialization returned nil")
		return nil
	}
	if !githubClient.HasToken() {
		fmt.Println("Warning: No GitHub token found. Cannot create repositories.")
		return nil
	}

	fmt.Println("GitHub client initialized successfully with token")
	return githubClient
}

func createRepoWithClient(client forge.RepoClient, repoName, description string) error {
	fmt.Printf("Checking/creating GitHub repository %s...\n", repoName)
	return client.CreateRepo(repoName, description, false)
}

func initCodebergClient(cfg *config.Config) forge.RepoClient {
	return initCodebergClientWithResolver(cfg, ForgeClientResolver{})
}

func initCodebergClientWithResolver(cfg *config.Config, resolver forgeClientResolver) forge.RepoClient {
	if !cfg.CodebergSyncEnabled() {
		fmt.Println("Warning: --create-codeberg-repos specified but Codeberg sync is disabled in config")
		return nil
	}

	codebergOrg := cfg.FindCodebergOrg()
	if codebergOrg == nil {
		fmt.Println("Warning: --create-codeberg-repos specified but no Codeberg organization found in config")
		return nil
	}

	fmt.Printf("Initializing Codeberg client for organization: %s\n", codebergOrg.Name)
	codebergClient, ok := resolver.ClientFor(codebergOrg)
	if !ok {
		fmt.Println("Warning: Codeberg client initialization returned nil")
		return nil
	}
	if !codebergClient.HasToken() {
		fmt.Println("Warning: No Codeberg token found. Cannot create repositories.")
		return nil
	}

	fmt.Println("Codeberg client initialized successfully with token")
	return codebergClient
}

func showReposToSync(repoNames []string) {
	fmt.Println("\nRepositories to sync:")
	for _, name := range repoNames {
		fmt.Printf("  - %s\n", name)
	}
}

func filterDryRunRepoNames(repoNames []string, flags *Flags) []string {
	_, syncState, err := loadSyncState(flags.WorkDir)
	if err != nil {
		fmt.Printf("Warning: Failed to load sync state: %v\n", err)
	}

	filtered := make([]string, 0, len(repoNames))
	for _, repoName := range repoNames {
		decision := evaluateSyncPolicy(repoName, flags.WorkDir, syncState, true, flags.Force, flags.Throttle)
		if decision.Message != "" {
			fmt.Println(decision.Message)
		}
		if decision.Skip {
			continue
		}
		filtered = append(filtered, repoName)
	}

	return filtered
}

func shuffledRepoNames(repoNames []string) []string {
	shuffled := append([]string(nil), repoNames...)
	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})
	return shuffled
}

func printFullSyncSeparator() {
	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("=== Continuing with GitHub to Codeberg sync ===")
	fmt.Println(strings.Repeat("=", 70) + "\n")
}

type syncExecution struct {
	syncer       *sync.Syncer
	descCache    map[string]string
	stateManager *state.Manager
	syncState    *state.State
	failedRepos  []string
}

func newSyncExecution(cfg *config.Config, flags *Flags) *syncExecution {
	execution := &syncExecution{
		descCache: loadDescriptionCache(flags.WorkDir),
		syncer:    sync.New(cfg, flags.WorkDir),
	}
	execution.syncer.SetBackupEnabled(shouldEnableBackupSync(flags))
	execution.syncer.SetDryRun(flags.DryRun)
	execution.syncer.SetForgejoBackupClientFactory(newForgejoBackupClient)

	manager, st, err := loadSyncState(flags.WorkDir)
	if err != nil {
		fmt.Printf("Warning: Failed to load sync state: %v\n", err)
	}
	execution.stateManager = manager
	execution.syncState = st

	return execution
}

func (e *syncExecution) maybeSkipRepo(repoName string, flags *Flags) bool {
	decision := evaluateSyncPolicy(repoName, flags.WorkDir, e.syncState, flags.DryRun, flags.Force, flags.Throttle)
	if decision.Message != "" {
		fmt.Println(decision.Message)
	}
	if decision.SetNextAllowed && e.stateManager != nil && !flags.DryRun {
		e.syncState.SetNextRepoSyncAllowed(repoName, decision.NextAllowed)
		if err := e.stateManager.Save(e.syncState); err != nil {
			fmt.Printf("Warning: Failed to save sync state: %v\n", err)
		}
	}

	return decision.Skip
}

func (e *syncExecution) markRepoSynced(repoName string, flags *Flags) {
	if e.stateManager == nil || flags.DryRun {
		return
	}

	recordRepoSync(repoName, e.syncState, flags.Throttle)
	if err := e.stateManager.Save(e.syncState); err != nil {
		fmt.Printf("Warning: Failed to save sync state: %v\n", err)
	}
}

// recordFailure logs a per-repo sync failure and keeps it for the end-of-run
// summary instead of aborting the batch - a single repo's transient error
// (network blip, one bad branch, etc.) shouldn't leave every other
// configured/discovered repo unsynced. Hard aborts are reserved for
// unrecoverable config/state errors detected before the loop starts.
func (e *syncExecution) recordFailure(repoName string, err error) {
	fmt.Printf("ERROR: Failed to sync %s: %v\n", repoName, err)
	e.failedRepos = append(e.failedRepos, repoName)
}

// printFailureSummary lists the repos recorded via recordFailure, if any.
func (e *syncExecution) printFailureSummary() {
	if len(e.failedRepos) == 0 {
		return
	}
	fmt.Printf("Failed to sync: %d repositories\n", len(e.failedRepos))
	for _, repo := range e.failedRepos {
		fmt.Printf("  - %s\n", repo)
	}
}

// exitCode returns 1 if any repo failed during this execution, 0 otherwise -
// the batch always runs to completion, but the process exit code still
// reflects whether everything actually succeeded.
func (e *syncExecution) exitCode() int {
	if len(e.failedRepos) > 0 {
		return 1
	}
	return 0
}

func (e *syncExecution) finishDiscoveredSync(successCount int, flags *Flags) {
	if !flags.DryRun {
		if err := saveDescriptionCache(flags.WorkDir, e.descCache); err != nil {
			fmt.Printf("Warning: Failed to save descriptions cache: %v\n", err)
		}
	}

	fmt.Printf("\n=== Summary ===\n")
	fmt.Printf("Successfully synced: %d repositories\n", successCount)
	e.printFailureSummary()

	if summary := e.syncer.GenerateAbandonedBranchSummary(); summary != "" {
		fmt.Print(summary)
	}

	if !flags.DryRun {
		printDeleteScript(e.syncer)
	}
}

func printDeleteScript(syncer *sync.Syncer) {
	if scriptPath, err := syncer.GenerateDeleteScript(); err != nil {
		fmt.Printf("\n⚠️  Failed to generate script: %v\n", err)
	} else if scriptPath != "" {
		fmt.Printf("\n")
		fmt.Print(strings.Repeat("=", 70))
		fmt.Printf("\n📋 ABANDONED BRANCH MANAGEMENT SCRIPT\n")
		fmt.Print(strings.Repeat("=", 70))
		fmt.Printf("\n")
		fmt.Printf("Generated script: %s\n", scriptPath)
		fmt.Printf("\n")
		fmt.Printf("Usage:\n")
		fmt.Printf("  bash %s --review       # Review diffs before deletion\n", scriptPath)
		fmt.Printf("  bash %s --review-full  # Review full diffs\n", scriptPath)
		fmt.Printf("  bash %s --dry-run      # Preview what will be deleted\n", scriptPath)
		fmt.Printf("  bash %s                # Delete branches (with confirmation)\n", scriptPath)
		fmt.Printf("\n")
		fmt.Printf("💡 Recommended workflow:\n")
		fmt.Printf("  1. Review branches:  bash %s --review\n", scriptPath)
		fmt.Printf("  2. Dry-run delete:   bash %s --dry-run\n", scriptPath)
		fmt.Printf("  3. Delete branches:  bash %s\n", scriptPath)
		fmt.Printf("\n")
		fmt.Printf("⚠️  WARNING: Review carefully before deleting branches!\n")
		fmt.Print(strings.Repeat("=", 70))
		fmt.Printf("\n")
	}
}

// forgeSyncSpec parametrizes syncDiscoveredRepos by which forge sourced the
// repo list and which forge (if any) mirror repos should be created on.
// syncCodebergRepos and syncGitHubRepos each build one of these and delegate
// the full per-repo sync loop (skip/throttle checks, optional mirror repo
// creation, SyncRepository, state recording, description sync) to the shared
// pipeline below.
type forgeSyncSpec struct {
	sourceLabel      string           // e.g. "Codeberg"; used in default mirror descriptions
	targetLabel      string           // e.g. "GitHub"; used in create-repo log lines
	createTarget     bool             // flags.CreateGitHubRepos / flags.CreateCodebergRepos
	targetClient     forge.RepoClient // nil unless createTarget && !flags.DryRun
	isCodebergSource bool             // selects which syncRepoDescriptions slot gets the discovered description

	// afterSync runs once the loop finishes. Codeberg's sync optionally
	// chains into a full GitHub sync and prints a separator; GitHub's sync
	// has no such follow-on and just returns 0.
	afterSync func(flags *Flags) int
}

// maybeCreateTargetRepo is the create-repo block that used to be duplicated
// (with only the client/label/description swapped) in syncCodebergRepos and
// syncGitHubRepos: if a mirror-creation client was configured, create the
// repo using its known description, falling back to a generated
// "Mirror of X from <source>" description when none is known.
func (spec forgeSyncSpec) maybeCreateTargetRepo(repoName, description string) {
	if spec.targetClient == nil || !spec.createTarget {
		return
	}
	if description == "" {
		description = fmt.Sprintf("Mirror of %s from %s", repoName, spec.sourceLabel)
	}

	fmt.Printf("Checking/creating %s repository %s...\n", spec.targetLabel, repoName)
	if err := spec.targetClient.CreateRepo(repoName, description, false); err != nil {
		fmt.Printf("Warning: Failed to create %s repo %s: %v\n", spec.targetLabel, repoName, err)
	}
}

// syncDescriptions replays the discovered description into whichever
// syncRepoDescriptions slot (knownCBDesc or knownGHDesc) matches the source
// forge, preserving the original precedence rules from description_sync.go.
func (spec forgeSyncSpec) syncDescriptions(cfg *config.Config, flags *Flags, execution *syncExecution, repoName, description string) {
	if spec.isCodebergSource {
		syncRepoDescriptions(cfg, flags.DryRun, execution.syncer.BackupActive, execution.syncer.DisableBackup, repoName, description, "", execution.descCache)
		return
	}
	syncRepoDescriptions(cfg, flags.DryRun, execution.syncer.BackupActive, execution.syncer.DisableBackup, repoName, "", description, execution.descCache)
}

// syncDiscoveredRepos is the shared sync loop for repos discovered via a
// public-repo listing (as opposed to the statically configured repos synced
// by HandleSyncAll). It is parametrized by spec for the handful of things
// that differ between the Codeberg->GitHub and GitHub->Codeberg directions:
// which forge to mirror into, the default description text, and what runs
// after the loop. repoDescriptions maps repo name to its known description on
// the source forge (missing entries yield "", matching the original map
// lookups which also treated a missing repo as an empty description).
func syncDiscoveredRepos(cfg *config.Config, flags *Flags, repoNames []string, repoDescriptions map[string]string, spec forgeSyncSpec) int {
	fmt.Printf("\nStarting sync of %d repositories...\n", len(repoNames))

	execution := newSyncExecution(cfg, flags)
	successCount := 0

	for i, repoName := range repoNames {
		fmt.Printf("\n[%d/%d] Syncing %s...\n", i+1, len(repoNames), repoName)

		if execution.maybeSkipRepo(repoName, flags) {
			continue
		}

		description := repoDescriptions[repoName]
		spec.maybeCreateTargetRepo(repoName, description)

		if err := execution.syncer.SyncRepository(repoName); err != nil {
			execution.recordFailure(repoName, err)
			continue
		}
		execution.markRepoSynced(repoName, flags)
		successCount++

		// After syncing, sync descriptions according to precedence
		spec.syncDescriptions(cfg, flags, execution, repoName, description)
	}

	execution.finishDiscoveredSync(successCount, flags)
	afterResult := spec.afterSync(flags)
	if execution.exitCode() != 0 {
		return execution.exitCode()
	}
	return afterResult
}

// syncCodebergRepos syncs repos discovered on Codeberg's public listing to
// their GitHub mirrors, optionally creating missing GitHub repos first. It is
// a thin wrapper around syncDiscoveredRepos; see forgeSyncSpec for what's
// parametrized.
func syncCodebergRepos(cfg *config.Config, flags *Flags, repos []forge.PublicRepo, repoNames []string, resolver forgeClientResolver) int {
	// Initialize GitHub client if needed
	var githubClient forge.RepoClient
	if flags.CreateGitHubRepos && !flags.DryRun {
		githubClient = initGitHubClientWithResolver(cfg, resolver)
	}

	spec := forgeSyncSpec{
		sourceLabel:      "Codeberg",
		targetLabel:      "GitHub",
		createTarget:     flags.CreateGitHubRepos,
		targetClient:     githubClient,
		isCodebergSource: true,
		afterSync: func(flags *Flags) int {
			if !flags.SyncGitHubPublic {
				return 0
			}
			// Print separator for full sync
			printFullSyncSeparator()
			return 0
		},
	}

	return syncDiscoveredRepos(cfg, flags, repoNames, publicRepoDescriptions(repos), spec)
}

// syncGitHubRepos syncs repos discovered on GitHub's public listing to their
// Codeberg mirrors, optionally creating missing Codeberg repos first. It is a
// thin wrapper around syncDiscoveredRepos; see forgeSyncSpec for what's
// parametrized.
func syncGitHubRepos(cfg *config.Config, flags *Flags, repos []forge.PublicRepo, repoNames []string, resolver forgeClientResolver) int {
	// Initialize Codeberg client if needed
	var codebergClient forge.RepoClient
	if flags.CreateCodebergRepos && !flags.DryRun {
		codebergClient = initCodebergClientWithResolver(cfg, resolver)
	}

	spec := forgeSyncSpec{
		sourceLabel:      "GitHub",
		targetLabel:      "Codeberg",
		createTarget:     flags.CreateCodebergRepos,
		targetClient:     codebergClient,
		isCodebergSource: false,
		afterSync:        func(flags *Flags) int { return 0 },
	}

	return syncDiscoveredRepos(cfg, flags, repoNames, publicRepoDescriptions(repos), spec)
}

// publicRepoDescriptions adapts a forge-agnostic public-repo listing into the
// plain name->description map that syncDiscoveredRepos operates on. Both
// Codeberg's and GitHub's public listings are converted to []forge.PublicRepo
// by their respective listers (see public_repo_lister.go) before reaching
// here, so a single helper now covers both forges - previously this needed
// two near-identical loops because codeberg.Repository and github.Repository
// were different concrete types.
func publicRepoDescriptions(repos []forge.PublicRepo) map[string]string {
	descriptions := make(map[string]string, len(repos))
	for _, repo := range repos {
		descriptions[repo.Name] = repo.Description
	}
	return descriptions
}

// publicRepoNames extracts just the repository names from a forge-agnostic
// public-repo listing, replacing the previous per-forge
// codeberg.GetRepoNames/github.GetRepoNames calls now that both listings
// share the forge.PublicRepo DTO.
func publicRepoNames(repos []forge.PublicRepo) []string {
	names := make([]string, len(repos))
	for i, repo := range repos {
		names[i] = repo.Name
	}
	return names
}
