package cli

import (
	"fmt"
	"math/rand"
	"strings"

	"github.com/snonux/gitsyncer/internal/config"
	"github.com/snonux/gitsyncer/internal/forge"
	"github.com/snonux/gitsyncer/internal/state"
	"github.com/snonux/gitsyncer/internal/sync"
)

func shouldEnableBackupSync(flags *Flags) bool {
	if flags == nil {
		return false
	}

	return flags.Backup || flags.FullSync || flags.SyncGitHubPublic
}

// HandleSync handles syncing a single repository. Naming a repo on the
// command line (`gitsyncer sync repo NAME`) always syncs it: the daily
// interval and --throttle windows only apply to batch commands, not to an
// explicit request.
func HandleSync(cfg *config.Config, flags *Flags) int {
	stateManager, syncState, err := loadSyncState(flags.WorkDir)
	if err != nil {
		fmt.Printf("Warning: Failed to load sync state: %v\n", err)
	}

	decision := evaluateSyncPolicy(flags.SyncRepo, flags.WorkDir, syncState, flags.DryRun, true, flags.Throttle)
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
	syncRepoDescriptions(cfg, flags.DryRun, syncer.BackupActive, syncer.DisableBackup, flags.SyncRepo, "", descCache)
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
	githubClient := configuredRepoCreationClient(cfg, flags)

	successCount := 0
	for i, repo := range repoNames {
		fmt.Printf("\n[%d/%d] Syncing %s...\n", i+1, len(repoNames), repo)

		if execution.maybeSkipRepo(repo, flags) {
			continue
		}

		createConfiguredGitHubRepo(repo, githubClient)

		if err := execution.syncer.SyncRepository(repo); err != nil {
			execution.recordFailure(repo, err)
			continue
		}
		execution.markRepoSynced(repo, flags)
		successCount++

		// Sync descriptions after repo sync
		syncRepoDescriptions(cfg, flags.DryRun, execution.syncer.BackupActive, execution.syncer.DisableBackup, repo, "", execution.descCache)
	}

	finishConfiguredSync(execution, successCount, flags)
	return execution.exitCode()
}

// configuredRepoCreationClient initializes the optional GitHub client used to
// create missing repositories before syncing configured repository names.
func configuredRepoCreationClient(cfg *config.Config, flags *Flags) forge.RepoClient {
	if flags.CreateGitHubRepos && !flags.DryRun {
		return initGitHubClient(cfg)
	}
	return nil
}

// createConfiguredGitHubRepo creates a missing GitHub repository before sync.
// A creation failure is a warning; synchronization still reports its own result.
func createConfiguredGitHubRepo(repo string, client forge.RepoClient) {
	if client == nil {
		return
	}
	if err := createRepoWithClient(client, repo, fmt.Sprintf("Mirror of %s", repo)); err != nil {
		fmt.Printf("Warning: Failed to create GitHub repo %s: %v\n", repo, err)
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

// HandleSyncGitHubPublic handles syncing all public GitHub repositories
func HandleSyncGitHubPublic(cfg *config.Config, flags *Flags) int {
	return handleSyncGitHubPublicWithDeps(cfg, flags, newGitHubPublicRepoLister, ForgeClientResolver{})
}

// handleSyncGitHubPublicWithDeps resolves the GitHub org, fetches its public
// repositories, then delegates post-processing to publicSyncPipeline.run.
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

	// A configured pushOnly mirror receives every public GitHub repository.
	pipeline := publicSyncPipeline{
		sourceLabel:    "GitHub",
		targetLabel:    githubSyncTargetLabel(cfg),
		applyAllowlist: !cfg.MirrorsAllGitHubRepos(),
		sync: func(repoNames []string) int {
			return syncGitHubRepos(cfg, flags, repos, repoNames)
		},
	}

	return pipeline.run(cfg, flags, publicRepoNames(repos))
}

// publicSyncPipeline reports the discovered repos, applies the configured
// allowlist when appropriate, and dispatches the sync loop.
type publicSyncPipeline struct {
	sourceLabel string // used in log and dry-run messages
	targetLabel string // destination label shown in dry-run output
	// applyAllowlist restricts the discovered repos to cfg.Repositories. It
	// is false for GitHub discovery when a pushOnly mirror must receive every
	// public repo (see Config.MirrorsAllGitHubRepos).
	applyAllowlist bool
	sync           func(repoNames []string) int
}

// githubSyncTargetLabel names the destination selected by the configured
// organizations for dry-run output.
func githubSyncTargetLabel(cfg *config.Config) string {
	if cfg.MirrorsAllGitHubRepos() {
		return "push-only Forgejo mirrors"
	}
	return "configured destinations"
}

func (p publicSyncPipeline) run(cfg *config.Config, flags *Flags, repoNames []string) int {
	fmt.Printf("Found %d public repositories on %s\n", len(repoNames), p.sourceLabel)

	if before := len(repoNames); before > 0 && p.applyAllowlist {
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

// syncGitHubRepos syncs repositories discovered on GitHub to the configured
// organizations and carries over their descriptions.
func syncGitHubRepos(cfg *config.Config, flags *Flags, repos []forge.PublicRepo, repoNames []string) int {
	execution := newSyncExecution(cfg, flags)
	descriptions := publicRepoDescriptions(repos)
	successCount := 0

	for i, repoName := range repoNames {
		fmt.Printf("\n[%d/%d] Syncing %s...\n", i+1, len(repoNames), repoName)
		if execution.maybeSkipRepo(repoName, flags) {
			continue
		}
		if err := execution.syncer.SyncRepository(repoName); err != nil {
			execution.recordFailure(repoName, err)
			continue
		}
		execution.markRepoSynced(repoName, flags)
		successCount++
		syncRepoDescriptions(cfg, flags.DryRun, execution.syncer.BackupActive, execution.syncer.DisableBackup, repoName, descriptions[repoName], execution.descCache)
	}

	execution.finishDiscoveredSync(successCount, flags)
	return execution.exitCode()
}

// publicRepoDescriptions adapts the public-repo listing into the map used by
// syncDiscoveredRepos.
func publicRepoDescriptions(repos []forge.PublicRepo) map[string]string {
	descriptions := make(map[string]string, len(repos))
	for _, repo := range repos {
		descriptions[repo.Name] = repo.Description
	}
	return descriptions
}

// publicRepoNames extracts repository names from a public-repo listing.
func publicRepoNames(repos []forge.PublicRepo) []string {
	names := make([]string, len(repos))
	for i, repo := range repos {
		names[i] = repo.Name
	}
	return names
}
