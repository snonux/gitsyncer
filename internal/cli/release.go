package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codeberg.org/snonux/gitsyncer/internal/codeberg"
	"codeberg.org/snonux/gitsyncer/internal/config"
	"codeberg.org/snonux/gitsyncer/internal/forge"
	"codeberg.org/snonux/gitsyncer/internal/github"
	"codeberg.org/snonux/gitsyncer/internal/localrepos"
	"codeberg.org/snonux/gitsyncer/internal/release"
	"codeberg.org/snonux/gitsyncer/internal/version"
)

// HandleCheckReleases checks for version tags without releases and creates them with confirmation
func HandleCheckReleases(cfg *config.Config, flags *Flags) int {
	// Get all repositories from work directory
	repositories, err := localrepos.ListLocalRepos(flags.WorkDir)
	if err != nil {
		fmt.Printf("Error reading work directory %s: %v\n", flags.WorkDir, err)
		return 1
	}

	if len(repositories) == 0 {
		fmt.Println("No repositories found in work directory")
		return 1
	}

	fmt.Printf("Found %d repositories in work directory\n", len(repositories))
	return HandleCheckReleasesForRepos(cfg, flags, repositories)
}

// HandleCheckReleasesForRepo checks releases for a specific repository
func HandleCheckReleasesForRepo(cfg *config.Config, flags *Flags, repoName string) int {
	// Check only the specified repository
	return HandleCheckReleasesForRepos(cfg, flags, []string{repoName})
}

type releaseNotesMode int

const (
	releaseNotesModeCreate releaseNotesMode = iota
	releaseNotesModeUpdate
)

// releaseTarget pairs a forge release client with the owner that releases
// should be created under. The client owns all HTTP/auth/404/409 handling; the
// CLI only orchestrates which tags to create or update.
type releaseTarget struct {
	name             string
	owner            string
	client           forge.ReleaseClient
	syncRepoRequired bool
}

// releaseNotesGenerator is the subset of the notes generator needed by the
// release pipeline. It is an interface so tests can substitute a fake.
type releaseNotesGenerator interface {
	GenerateAIReleaseNotes(repoPath, repoName, tag string, allTags []string, commits []string) (string, error)
	GenerateReleaseNotes(repoPath, tag string, allTags []string) string
}

// HandleCheckReleasesForRepos checks for version tags without releases and creates them with confirmation
func HandleCheckReleasesForRepos(cfg *config.Config, flags *Flags, repositories []string) int {
	if flags.DryRun {
		fmt.Println("[DRY RUN] Skipping post-sync release processing")
		return 0
	}

	// Git inspection and notes generation are split out of the old release
	// manager god-object: the inspector reads git state, the notes generator
	// turns commits/diffs into release notes, and per-forge clients handle the
	// release CRUD.
	inspector := release.NewGitInspector()
	notes := release.NewNotesGenerator(flags.AITool, inspector)

	// Load persistent AI release notes cache
	cacheFile := filepath.Join(flags.WorkDir, ".gitsyncer-ai-release-notes-cache.json")
	aiReleaseNotesCache := loadAIReleaseNotesCache(cacheFile)
	initialCacheSize := len(aiReleaseNotesCache)

	// Track failed AI generations
	failedAIGenerations := []string{}

	// Print summary at the end. Args are captured now, but aiReleaseNotesCache
	// (a map) and &failedAIGenerations (a pointer) stay live references, so
	// the deferred call still sees every entry added during the run below.
	defer printReleaseSummary(cacheFile, initialCacheSize, aiReleaseNotesCache, &failedAIGenerations)

	// Resolve GitHub/Codeberg tokens from config/env/file and build the list
	// of forges to publish releases to.
	releaseTargets := buildReleaseTargets(cfg)

	// Process the specified repositories
	for _, repoName := range repositories {
		processReleasesForRepo(
			cfg,
			flags,
			inspector,
			notes,
			releaseTargets,
			repoName,
			cacheFile,
			aiReleaseNotesCache,
			&failedAIGenerations,
		)
	}

	return 0
}

// printReleaseSummary prints the end-of-run summary: how many AI release
// notes cache entries were added, and which releases failed AI notes
// generation (if any). failedAIGenerations is a pointer because it is
// deferred before the run loop populates it.
func printReleaseSummary(cacheFile string, initialCacheSize int, aiReleaseNotesCache map[string]string, failedAIGenerations *[]string) {
	if len(aiReleaseNotesCache) > initialCacheSize {
		fmt.Printf("\nAI release notes cache updated: %d new entries added (total: %d entries)\n",
			len(aiReleaseNotesCache)-initialCacheSize, len(aiReleaseNotesCache))
		fmt.Printf("Cache file: %s\n", cacheFile)
	}

	if len(*failedAIGenerations) > 0 {
		fmt.Printf("\n⚠️  AI release notes generation failed for %d releases:\n", len(*failedAIGenerations))
		for _, failed := range *failedAIGenerations {
			fmt.Printf("  - %s\n", failed)
		}
		fmt.Println("\nThese releases were skipped. Their cache entries were cleared.")
		fmt.Println("Run again to retry generation for these releases.")
	}
}

// processReleasesForRepo checks release status for a single repository
// against every applicable release target, creating any missing releases and
// (when requested) updating existing ones. It is a no-op when the repository
// is not cloned locally or has no version tags.
func processReleasesForRepo(
	cfg *config.Config,
	flags *Flags,
	inspector *release.GitInspector,
	notes releaseNotesGenerator,
	releaseTargets []releaseTarget,
	repoName string,
	cacheFile string,
	aiReleaseNotesCache map[string]string,
	failedAIGenerations *[]string,
) {
	fmt.Printf("\nChecking releases for repository: %s\n", repoName)

	repoPath := filepath.Join(flags.WorkDir, repoName)
	localTags, ok := localVersionTags(cfg, inspector, repoName, repoPath)
	if !ok {
		return
	}

	createMissingReleasesForRepo(
		cfg, flags, inspector, notes, releaseTargets, repoName, repoPath, localTags,
		cacheFile, aiReleaseNotesCache, failedAIGenerations,
	)

	if flags.UpdateReleases {
		updateExistingReleasesForRepo(
			cfg, flags, inspector, notes, releaseTargets, repoName, repoPath, localTags,
			cacheFile, aiReleaseNotesCache, failedAIGenerations,
		)
	}
}

// localVersionTags checks that repoPath is cloned locally and has version
// tags, logging the reason and returning ok=false when it is not cloned, tag
// listing fails, or no tags exist. On success it also logs any configured
// skip_releases entries for repoName before returning the tags.
func localVersionTags(cfg *config.Config, inspector *release.GitInspector, repoName, repoPath string) ([]string, bool) {
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		fmt.Printf("  Repository not found locally at %s, skipping...\n", repoPath)
		return nil, false
	}

	localTags, err := inspector.GetLocalTags(repoPath)
	if err != nil {
		fmt.Printf("  Error getting local tags: %v\n", err)
		return nil, false
	}

	if len(localTags) == 0 {
		fmt.Println("  No version tags found")
		return nil, false
	}

	fmt.Printf("  Found %d version tags: %s\n", len(localTags), strings.Join(localTags, ", "))
	// Log configured skip rules for this repo, if any
	if cfg.SkipReleases != nil {
		if skipTags, ok := cfg.SkipReleases[repoName]; ok && len(skipTags) > 0 {
			fmt.Printf("  Config skip_releases for %s: %s\n", repoName, strings.Join(skipTags, ", "))
		}
	}

	return localTags, true
}

// createMissingReleasesForRepo creates, on every applicable release target,
// any release tags present locally but missing on the forge.
func createMissingReleasesForRepo(
	cfg *config.Config,
	flags *Flags,
	inspector *release.GitInspector,
	notes releaseNotesGenerator,
	releaseTargets []releaseTarget,
	repoName, repoPath string,
	localTags []string,
	cacheFile string,
	aiReleaseNotesCache map[string]string,
	failedAIGenerations *[]string,
) {
	for _, target := range releaseTargets {
		if !releaseTargetApplicable(target, cfg, repoName) {
			continue
		}
		missingReleases := getMissingReleasesForTarget(cfg, inspector, target, repoName, localTags)
		processCreateReleasesForTarget(
			cfg,
			flags,
			inspector,
			notes,
			target,
			repoName,
			repoPath,
			localTags,
			missingReleases,
			cacheFile,
			aiReleaseNotesCache,
			failedAIGenerations,
		)
	}
}

// updateExistingReleasesForRepo refreshes, on every applicable release
// target, releases that already exist on the forge. Only called when the
// caller has confirmed flags.UpdateReleases is set.
func updateExistingReleasesForRepo(
	cfg *config.Config,
	flags *Flags,
	inspector *release.GitInspector,
	notes releaseNotesGenerator,
	releaseTargets []releaseTarget,
	repoName, repoPath string,
	localTags []string,
	cacheFile string,
	aiReleaseNotesCache map[string]string,
	failedAIGenerations *[]string,
) {
	for _, target := range releaseTargets {
		if !releaseTargetApplicable(target, cfg, repoName) {
			continue
		}
		processUpdateReleasesForTarget(
			flags,
			inspector,
			notes,
			target,
			repoName,
			repoPath,
			localTags,
			cacheFile,
			aiReleaseNotesCache,
			failedAIGenerations,
		)
	}
}

// buildReleaseTargets resolves the GitHub and Codeberg tokens (from config,
// falling back to environment variables and token files) and returns the
// release targets releases should be published to. A forge is included only
// when its organization is configured in cfg; Codeberg is additionally
// skipped when Codeberg sync is disabled.
func buildReleaseTargets(cfg *config.Config) []releaseTarget {
	var targets []releaseTarget

	if target, ok := buildGitHubReleaseTarget(cfg); ok {
		targets = append(targets, target)
	}
	if target, ok := buildCodebergReleaseTarget(cfg); ok {
		targets = append(targets, target)
	}

	return targets
}

// buildGitHubReleaseTarget resolves the GitHub org and token from cfg and
// returns the corresponding release target. ok is false when no GitHub org
// is configured, or the configured org has no name.
func buildGitHubReleaseTarget(cfg *config.Config) (releaseTarget, bool) {
	githubOrg := cfg.FindGitHubOrg()
	if githubOrg == nil {
		fmt.Println("No GitHub organization found in config")
		return releaseTarget{}, false
	}
	fmt.Printf("Found GitHub org: %s\n", githubOrg.Name)

	// Try config token first, then fallback to env var and file
	token := resolveGitHubToken(githubOrg.GitHubToken)
	if token == "" {
		fmt.Println("WARNING: No GitHub token found - cannot create GitHub releases")
	}

	if githubOrg.Name == "" {
		return releaseTarget{}, false
	}
	return releaseTarget{
		name:   "GitHub",
		owner:  githubOrg.Name,
		client: github.NewClient(token, githubOrg.Name),
	}, true
}

// buildCodebergReleaseTarget resolves the Codeberg org and token from cfg and
// returns the corresponding release target. ok is false when no Codeberg org
// is configured, Codeberg sync is disabled, or the configured org has no
// name.
func buildCodebergReleaseTarget(cfg *config.Config) (releaseTarget, bool) {
	codebergOrg := cfg.FindCodebergOrg()
	if codebergOrg != nil && !cfg.CodebergSyncEnabled() {
		fmt.Println("Codeberg organization found in config but Codeberg sync is disabled (set \"sync_codeberg\": true to enable Codeberg releases)")
		codebergOrg = nil
	} else if codebergOrg == nil {
		fmt.Println("No Codeberg organization found in config")
	}
	// codebergOrg is nil here either because no Codeberg org is configured or
	// because Codeberg syncing is disabled in the config; the relevant
	// message has already been printed above.
	if codebergOrg == nil {
		return releaseTarget{}, false
	}
	fmt.Printf("Found Codeberg org: %s\n", codebergOrg.Name)

	// Try config token first, then fallback to env var and file
	token := resolveCodebergToken(codebergOrg.CodebergToken)
	if token != "" {
		fmt.Printf("  Codeberg token loaded (length: %d)\n", len(token))
	} else {
		fmt.Println("WARNING: No Codeberg token found - cannot create Codeberg releases")
	}

	if codebergOrg.Name == "" {
		return releaseTarget{}, false
	}
	return releaseTarget{
		name:             "Codeberg",
		owner:            codebergOrg.Name,
		client:           codeberg.NewClient(token, codebergOrg.Name),
		syncRepoRequired: true,
	}, true
}

// loadTokenWithFallback resolves a forge token from, in order: the config
// value, the named environment variable, or a dotfile of tokenFileName under
// the user's home directory. Config and env values are used as-is; only the
// token file contents are trimmed. Returns "" when no source has a token
// (including when the home directory or token file cannot be read). This is
// the single cascade shared by resolveGitHubToken and resolveCodebergToken so
// the config->env->file fallback logic exists in one place.
func loadTokenWithFallback(configToken, envVar, tokenFileName string) string {
	if configToken != "" {
		return configToken
	}
	if token := os.Getenv(envVar); token != "" {
		return token
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(home, tokenFileName))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// resolveGitHubToken loads the GitHub token from config, the GITHUB_TOKEN env
// var, or ~/.gitsyncer_github_token, in that order.
func resolveGitHubToken(configToken string) string {
	return loadTokenWithFallback(configToken, "GITHUB_TOKEN", ".gitsyncer_github_token")
}

// resolveCodebergToken loads the Codeberg token from config, the
// CODEBERG_TOKEN env var, or ~/.gitsyncer_codeberg_token, in that order.
func resolveCodebergToken(configToken string) string {
	return loadTokenWithFallback(configToken, "CODEBERG_TOKEN", ".gitsyncer_codeberg_token")
}

// releaseTargetApplicable reports whether a release target should run for the
// given repository. Targets marked syncRepoRequired (e.g. Codeberg) only run
// for repos in the configured sync allowlist, so releases are not created on
// Codeberg for repos that are not synced there.
func releaseTargetApplicable(target releaseTarget, cfg *config.Config, repoName string) bool {
	if !target.syncRepoRequired {
		return true
	}
	return cfg.IsSyncRepo(repoName)
}

// releasesEnabler returns the target's ReleasesEnabler implementation, or nil
// if the forge does not need per-repository release enabling (e.g. GitHub).
func releasesEnabler(target releaseTarget) forge.ReleasesEnabler {
	if enabler, ok := target.client.(forge.ReleasesEnabler); ok {
		return enabler
	}
	return nil
}

func getMissingReleasesForTarget(cfg *config.Config, inspector *release.GitInspector, target releaseTarget, repoName string, localTags []string) []string {
	releases, err := target.client.GetReleases(target.owner, repoName)
	if err != nil {
		fmt.Printf("  Error checking %s releases: %v\n", target.name, err)
		return nil
	}

	missingReleases := inspector.FindMissingReleases(localTags, releases)
	if len(missingReleases) == 0 {
		return missingReleases
	}

	var filtered []string
	var skipped []string
	for _, tag := range missingReleases {
		if cfg.ShouldSkipRelease(repoName, tag) {
			skipped = append(skipped, tag)
		} else {
			filtered = append(filtered, tag)
		}
	}
	if len(skipped) > 0 {
		fmt.Printf("  Skipping %s releases per config for tags: %s\n", target.name, strings.Join(skipped, ", "))
	}
	if len(filtered) > 0 {
		fmt.Printf("  Missing %s releases: %s\n", target.name, strings.Join(filtered, ", "))
	}

	return filtered
}

func processCreateReleasesForTarget(
	cfg *config.Config,
	flags *Flags,
	inspector *release.GitInspector,
	notes releaseNotesGenerator,
	target releaseTarget,
	repoName, repoPath string,
	localTags, missingReleases []string,
	cacheFile string,
	aiReleaseNotesCache map[string]string,
	failedAIGenerations *[]string,
) {
	if flags.DryRun || len(missingReleases) == 0 {
		return
	}

	if enabler := releasesEnabler(target); enabler != nil {
		if err := enabler.EnsureReleasesEnabled(target.owner, repoName); err != nil {
			fmt.Printf("  Warning: Could not ensure %s releases are enabled: %v\n", target.name, err)
		}
	}

	for _, tag := range missingReleases {
		if cfg.ShouldSkipRelease(repoName, tag) {
			fmt.Printf("  Skipping %s release for %s:%s per config skip_releases\n", target.name, repoName, tag)
			continue
		}

		commits, err := inspector.GetCommitsSinceTag(repoPath, "", tag)
		if err != nil {
			commits = []string{}
		}

		releaseNotes, ok := resolveReleaseNotes(
			notes,
			flags,
			repoPath,
			repoName,
			tag,
			localTags,
			commits,
			cacheFile,
			aiReleaseNotesCache,
			failedAIGenerations,
			fmt.Sprintf("%s/%s:%s", target.owner, repoName, tag),
			releaseNotesModeCreate,
			saveAIReleaseNotesCache,
		)
		if !ok {
			continue
		}

		fmt.Printf("\n%s\n", strings.Repeat("=", 70))
		fmt.Printf("Release Notes for %s/%s tag %s:\n", target.owner, repoName, tag)
		fmt.Printf("%s\n", strings.Repeat("-", 70))
		fmt.Println(releaseNotes)
		fmt.Printf("%s\n\n", strings.Repeat("=", 70))

		msg := fmt.Sprintf("Create %s release for %s/%s tag %s?", target.name, target.owner, repoName, tag)

		createRelease := false
		if flags.AutoCreateReleases {
			fmt.Printf("  Auto-creating %s release for %s/%s tag %s\n", target.name, target.owner, repoName, tag)
			createRelease = true
		} else {
			createRelease = promptConfirmation(msg)
		}

		if createRelease {
			if err := target.client.CreateRelease(target.owner, repoName, tag, releaseNotes); err != nil {
				fmt.Printf("  Error creating %s release: %v\n", target.name, err)
			} else {
				fmt.Printf("  Created %s release for tag %s\n", target.name, tag)
			}
		}
	}
}

func processUpdateReleasesForTarget(
	flags *Flags,
	inspector *release.GitInspector,
	notes releaseNotesGenerator,
	target releaseTarget,
	repoName, repoPath string,
	localTags []string,
	cacheFile string,
	aiReleaseNotesCache map[string]string,
	failedAIGenerations *[]string,
) {
	if flags.DryRun || !flags.AIReleaseNotes {
		return
	}

	existingReleases, err := target.client.GetReleases(target.owner, repoName)
	if err != nil || len(existingReleases) == 0 {
		return
	}

	fmt.Printf("\n  Updating existing %s releases...\n", target.name)
	for _, tag := range existingReleases {
		if !version.IsVersionTag(tag) {
			continue
		}

		commits, err := inspector.GetCommitsSinceTag(repoPath, "", tag)
		if err != nil {
			commits = []string{}
		}

		releaseNotes, ok := resolveReleaseNotes(
			notes,
			flags,
			repoPath,
			repoName,
			tag,
			localTags,
			commits,
			cacheFile,
			aiReleaseNotesCache,
			failedAIGenerations,
			fmt.Sprintf("%s/%s:%s", target.owner, repoName, tag),
			releaseNotesModeUpdate,
			saveAIReleaseNotesCache,
		)
		if !ok {
			continue
		}

		fmt.Printf("\n%s\n", strings.Repeat("=", 70))
		fmt.Printf("Updated Release Notes for %s/%s tag %s:\n", target.owner, repoName, tag)
		fmt.Printf("%s\n", strings.Repeat("-", 70))
		fmt.Println(releaseNotes)
		fmt.Printf("%s\n\n", strings.Repeat("=", 70))

		msg := fmt.Sprintf("Update %s release for %s/%s tag %s?", target.name, target.owner, repoName, tag)

		updateRelease := false
		if flags.AutoCreateReleases {
			fmt.Printf("  Auto-updating %s release for %s/%s tag %s\n", target.name, target.owner, repoName, tag)
			updateRelease = true
		} else {
			updateRelease = promptConfirmation(msg)
		}

		if updateRelease {
			if err := target.client.UpdateRelease(target.owner, repoName, tag, releaseNotes); err != nil {
				fmt.Printf("  Error updating %s release: %v\n", target.name, err)
			} else {
				fmt.Printf("  Updated %s release for tag %s\n", target.name, tag)
			}
		}
	}
}

func resolveReleaseNotes(
	notes releaseNotesGenerator,
	flags *Flags,
	repoPath, repoName, tag string,
	localTags, commits []string,
	cacheFile string,
	aiReleaseNotesCache map[string]string,
	failedAIGenerations *[]string,
	failedTarget string,
	mode releaseNotesMode,
	saveCache func(string, map[string]string) error,
) (string, bool) {
	if !flags.AIReleaseNotes {
		if mode == releaseNotesModeUpdate {
			return "", false
		}
		return notes.GenerateReleaseNotes(repoPath, tag, localTags), true
	}

	cacheKey := fmt.Sprintf("%s:%s", repoName, tag)
	// Force controls sync scheduling and must not invalidate release-note cache entries.
	if cachedNotes, exists := aiReleaseNotesCache[cacheKey]; exists {
		if mode == releaseNotesModeUpdate {
			fmt.Printf("  Using cached AI release notes for existing release %s\n", tag)
		} else {
			fmt.Printf("  Using cached AI release notes for %s\n", tag)
		}
		return cachedNotes, true
	}

	if mode == releaseNotesModeUpdate {
		fmt.Printf("  Generating AI release notes for existing release %s...\n", tag)
	} else {
		fmt.Printf("  Generating AI release notes for %s...\n", tag)
	}

	aiNotes, err := notes.GenerateAIReleaseNotes(repoPath, repoName, tag, localTags, commits)
	if err != nil {
		fmt.Printf("  Warning: Failed to generate AI release notes: %v\n", err)
		delete(aiReleaseNotesCache, cacheKey)
		*failedAIGenerations = append(*failedAIGenerations, failedTarget)
		_ = saveCache(cacheFile, aiReleaseNotesCache)

		if mode == releaseNotesModeCreate {
			fmt.Printf("  Falling back to standard release notes\n")
			return notes.GenerateReleaseNotes(repoPath, tag, localTags), true
		}
		return "", false
	}

	aiReleaseNotesCache[cacheKey] = aiNotes // Cache only on success
	if err := saveCache(cacheFile, aiReleaseNotesCache); err != nil {
		fmt.Printf("  Warning: Failed to save cache: %v\n", err)
	}
	if mode == releaseNotesModeCreate {
		fmt.Printf("  AI release notes generated successfully and cached\n")
	}

	return aiNotes, true
}

// loadAIReleaseNotesCache loads the AI release notes cache from disk
func loadAIReleaseNotesCache(cacheFile string) map[string]string {
	cache := make(map[string]string)

	data, err := os.ReadFile(cacheFile)
	if err != nil {
		// Cache file doesn't exist yet, return empty cache
		return cache
	}

	if err := json.Unmarshal(data, &cache); err != nil {
		fmt.Printf("Warning: Failed to parse AI release notes cache: %v\n", err)
		return make(map[string]string)
	}

	fmt.Printf("Loaded AI release notes cache with %d entries\n", len(cache))
	return cache
}

// saveAIReleaseNotesCache saves the AI release notes cache to disk
func saveAIReleaseNotesCache(cacheFile string, cache map[string]string) error {
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache: %w", err)
	}

	if err := os.WriteFile(cacheFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	// Don't print on every save since we save after each generation
	return nil
}
