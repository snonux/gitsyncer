package release

// Release-checking orchestration: given a set of locally cloned
// repositories, find version tags missing a release on each configured
// forge, generate (optionally AI-assisted, cached) release notes, and create
// or update the releases with the caller's confirmation. This was
// internal/cli/release.go's HandleCheckReleasesForRepos and its helpers
// before task w01 moved the orchestration logic that belongs in this
// package out of the CLI layer; internal/cli/release.go now only translates
// Flags into Options and delegates here.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codeberg.org/snonux/gitsyncer/internal/config"
	"codeberg.org/snonux/gitsyncer/internal/version"
)

// Options carries the subset of internal/cli's Flags that release
// orchestration needs. It is a separate type (rather than internal/release
// importing internal/cli.Flags directly) because internal/cli already
// imports internal/release for GitInspector/NotesGenerator; importing back
// would be a cycle, and this also keeps the package's public API independent
// of the CLI's flag layout.
type Options struct {
	DryRun             bool
	WorkDir            string
	AutoCreateReleases bool
	AIReleaseNotes     bool
	UpdateReleases     bool
	AITool             string
}

// ConfirmFunc asks the user to confirm an action (e.g. "Create release for
// X?") and reports their answer. internal/cli injects its interactive
// console prompt (promptConfirmation); tests inject a fake.
type ConfirmFunc func(message string) bool

// releaseNotesSource is the subset of *NotesGenerator the release pipeline
// needs. It is an interface (distinct from the concrete NotesGenerator type
// above) so tests can substitute a fake instead of shelling out to a real AI
// tool.
type releaseNotesSource interface {
	GenerateAIReleaseNotes(repoPath, repoName, tag string, allTags []string, commits []string) (string, error)
	GenerateReleaseNotes(repoPath, tag string, allTags []string) string
}

type releaseNotesMode int

const (
	releaseNotesModeCreate releaseNotesMode = iota
	releaseNotesModeUpdate
)

// CheckReleasesForRepos checks for version tags without releases and creates
// them with confirmation, across every repository in repositories. It is the
// single entry point internal/cli.HandleCheckReleasesForRepos delegates to.
func CheckReleasesForRepos(cfg *config.Config, opts Options, repositories []string, confirm ConfirmFunc) int {
	if opts.DryRun {
		fmt.Println("[DRY RUN] Skipping post-sync release processing")
		return 0
	}

	// Git inspection and notes generation are split out of the old release
	// manager god-object: the inspector reads git state, the notes generator
	// turns commits/diffs into release notes, and per-forge clients handle the
	// release CRUD.
	inspector := NewGitInspector()
	notes := NewNotesGenerator(opts.AITool, inspector)

	// Load persistent AI release notes cache
	cacheFile := filepath.Join(opts.WorkDir, ".gitsyncer-ai-release-notes-cache.json")
	aiReleaseNotesCache := LoadAIReleaseNotesCache(cacheFile)
	initialCacheSize := len(aiReleaseNotesCache)

	// Track failed AI generations
	failedAIGenerations := []string{}

	// Print summary at the end. Args are captured now, but aiReleaseNotesCache
	// (a map) and &failedAIGenerations (a pointer) stay live references, so
	// the deferred call still sees every entry added during the run below.
	defer printReleaseSummary(cacheFile, initialCacheSize, aiReleaseNotesCache, &failedAIGenerations)

	// Resolve GitHub/Codeberg tokens from config/env/file and build the list
	// of forges to publish releases to.
	releaseTargets := BuildReleaseTargets(cfg)

	// Process the specified repositories
	for _, repoName := range repositories {
		processReleasesForRepo(
			cfg,
			opts,
			confirm,
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
	opts Options,
	confirm ConfirmFunc,
	inspector *GitInspector,
	notes releaseNotesSource,
	releaseTargets []Target,
	repoName string,
	cacheFile string,
	aiReleaseNotesCache map[string]string,
	failedAIGenerations *[]string,
) {
	fmt.Printf("\nChecking releases for repository: %s\n", repoName)

	repoPath := filepath.Join(opts.WorkDir, repoName)
	localTags, ok := localVersionTags(cfg, inspector, repoName, repoPath)
	if !ok {
		return
	}

	createMissingReleasesForRepo(
		cfg, opts, confirm, inspector, notes, releaseTargets, repoName, repoPath, localTags,
		cacheFile, aiReleaseNotesCache, failedAIGenerations,
	)

	if opts.UpdateReleases {
		updateExistingReleasesForRepo(
			cfg, opts, confirm, inspector, notes, releaseTargets, repoName, repoPath, localTags,
			cacheFile, aiReleaseNotesCache, failedAIGenerations,
		)
	}
}

// localVersionTags checks that repoPath is cloned locally and has version
// tags, logging the reason and returning ok=false when it is not cloned, tag
// listing fails, or no tags exist. On success it also logs any configured
// skip_releases entries for repoName before returning the tags.
func localVersionTags(cfg *config.Config, inspector *GitInspector, repoName, repoPath string) ([]string, bool) {
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
	opts Options,
	confirm ConfirmFunc,
	inspector *GitInspector,
	notes releaseNotesSource,
	releaseTargets []Target,
	repoName, repoPath string,
	localTags []string,
	cacheFile string,
	aiReleaseNotesCache map[string]string,
	failedAIGenerations *[]string,
) {
	for _, target := range releaseTargets {
		if !TargetApplicable(target, cfg, repoName) {
			continue
		}
		missingReleases := getMissingReleasesForTarget(cfg, inspector, target, repoName, localTags)
		processCreateReleasesForTarget(
			cfg,
			opts,
			confirm,
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
// caller has confirmed opts.UpdateReleases is set.
func updateExistingReleasesForRepo(
	cfg *config.Config,
	opts Options,
	confirm ConfirmFunc,
	inspector *GitInspector,
	notes releaseNotesSource,
	releaseTargets []Target,
	repoName, repoPath string,
	localTags []string,
	cacheFile string,
	aiReleaseNotesCache map[string]string,
	failedAIGenerations *[]string,
) {
	for _, target := range releaseTargets {
		if !TargetApplicable(target, cfg, repoName) {
			continue
		}
		processUpdateReleasesForTarget(
			opts,
			confirm,
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

func getMissingReleasesForTarget(cfg *config.Config, inspector *GitInspector, target Target, repoName string, localTags []string) []string {
	releases, err := target.Client.GetReleases(target.Owner, repoName)
	if err != nil {
		fmt.Printf("  Error checking %s releases: %v\n", target.Name, err)
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
		fmt.Printf("  Skipping %s releases per config for tags: %s\n", target.Name, strings.Join(skipped, ", "))
	}
	if len(filtered) > 0 {
		fmt.Printf("  Missing %s releases: %s\n", target.Name, strings.Join(filtered, ", "))
	}

	return filtered
}

func processCreateReleasesForTarget(
	cfg *config.Config,
	opts Options,
	confirm ConfirmFunc,
	inspector *GitInspector,
	notes releaseNotesSource,
	target Target,
	repoName, repoPath string,
	localTags, missingReleases []string,
	cacheFile string,
	aiReleaseNotesCache map[string]string,
	failedAIGenerations *[]string,
) {
	if opts.DryRun || len(missingReleases) == 0 {
		return
	}

	ensureReleasesEnabledForTarget(target, repoName)

	for _, tag := range missingReleases {
		if cfg.ShouldSkipRelease(repoName, tag) {
			fmt.Printf("  Skipping %s release for %s:%s per config skip_releases\n", target.Name, repoName, tag)
			continue
		}

		createReleaseForTag(
			opts, confirm, inspector, notes, target, repoName, repoPath, tag,
			localTags, cacheFile, aiReleaseNotesCache, failedAIGenerations,
		)
	}
}

// ensureReleasesEnabledForTarget calls the target's ReleasesEnabler (if the
// forge implements one) before attempting to create releases, warning
// rather than failing if it errors. Forges without this requirement (e.g.
// GitHub) leave releasesEnabler nil and this is a no-op.
func ensureReleasesEnabledForTarget(target Target, repoName string) {
	enabler := releasesEnabler(target)
	if enabler == nil {
		return
	}
	if err := enabler.EnsureReleasesEnabled(target.Owner, repoName); err != nil {
		fmt.Printf("  Warning: Could not ensure %s releases are enabled: %v\n", target.Name, err)
	}
}

// createReleaseForTag resolves release notes for one missing tag, prints
// them for review, confirms (or auto-confirms per opts.AutoCreateReleases),
// and creates the release on target.
func createReleaseForTag(
	opts Options,
	confirm ConfirmFunc,
	inspector *GitInspector,
	notes releaseNotesSource,
	target Target,
	repoName, repoPath, tag string,
	localTags []string,
	cacheFile string,
	aiReleaseNotesCache map[string]string,
	failedAIGenerations *[]string,
) {
	commits, err := inspector.GetCommitsSinceTag(repoPath, "", tag)
	if err != nil {
		commits = []string{}
	}

	releaseNotes, ok := resolveReleaseNotes(
		notes,
		opts,
		repoPath,
		repoName,
		tag,
		localTags,
		commits,
		cacheFile,
		aiReleaseNotesCache,
		failedAIGenerations,
		fmt.Sprintf("%s/%s:%s", target.Owner, repoName, tag),
		releaseNotesModeCreate,
		SaveAIReleaseNotesCache,
	)
	if !ok {
		return
	}

	printReleaseNotesForReview("Release Notes", target.Owner, repoName, tag, releaseNotes)

	msg := fmt.Sprintf("Create %s release for %s/%s tag %s?", target.Name, target.Owner, repoName, tag)
	if !decideReleaseAction(opts, confirm, "creating", target, repoName, tag, msg) {
		return
	}

	if err := target.Client.CreateRelease(target.Owner, repoName, tag, releaseNotes); err != nil {
		fmt.Printf("  Error creating %s release: %v\n", target.Name, err)
	} else {
		fmt.Printf("  Created %s release for tag %s\n", target.Name, tag)
	}
}

// decideReleaseAction reports whether a create/update mutation should
// proceed: true automatically when opts.AutoCreateReleases is set (printing
// an "Auto-<verb>ing" notice), otherwise whatever confirm(msg) returns.
// actionVerb is "creating" or "updating", matching createReleaseForTag's and
// updateReleaseForTag's respective log wording.
func decideReleaseAction(opts Options, confirm ConfirmFunc, actionVerb string, target Target, repoName, tag, msg string) bool {
	if opts.AutoCreateReleases {
		fmt.Printf("  Auto-%s %s release for %s/%s tag %s\n", actionVerb, target.Name, target.Owner, repoName, tag)
		return true
	}
	return confirm(msg)
}

// printReleaseNotesForReview prints notes framed by "====...===="/"----...--"
// separators, shared by the create and update paths (which only differ in
// the header text: "Release Notes" vs "Updated Release Notes").
func printReleaseNotesForReview(header, owner, repoName, tag, notes string) {
	fmt.Printf("\n%s\n", strings.Repeat("=", 70))
	fmt.Printf("%s for %s/%s tag %s:\n", header, owner, repoName, tag)
	fmt.Printf("%s\n", strings.Repeat("-", 70))
	fmt.Println(notes)
	fmt.Printf("%s\n\n", strings.Repeat("=", 70))
}

func processUpdateReleasesForTarget(
	opts Options,
	confirm ConfirmFunc,
	inspector *GitInspector,
	notes releaseNotesSource,
	target Target,
	repoName, repoPath string,
	localTags []string,
	cacheFile string,
	aiReleaseNotesCache map[string]string,
	failedAIGenerations *[]string,
) {
	if opts.DryRun || !opts.AIReleaseNotes {
		return
	}

	existingReleases, err := target.Client.GetReleases(target.Owner, repoName)
	if err != nil || len(existingReleases) == 0 {
		return
	}

	fmt.Printf("\n  Updating existing %s releases...\n", target.Name)
	for _, tag := range existingReleases {
		if !version.IsVersionTag(tag) {
			continue
		}

		updateReleaseForTag(
			opts, confirm, inspector, notes, target, repoName, repoPath, tag,
			localTags, cacheFile, aiReleaseNotesCache, failedAIGenerations,
		)
	}
}

// updateReleaseForTag resolves (regenerating or reusing cached AI) release
// notes for one existing tag, prints them for review, confirms (or
// auto-confirms per opts.AutoCreateReleases), and updates the release on
// target.
func updateReleaseForTag(
	opts Options,
	confirm ConfirmFunc,
	inspector *GitInspector,
	notes releaseNotesSource,
	target Target,
	repoName, repoPath, tag string,
	localTags []string,
	cacheFile string,
	aiReleaseNotesCache map[string]string,
	failedAIGenerations *[]string,
) {
	commits, err := inspector.GetCommitsSinceTag(repoPath, "", tag)
	if err != nil {
		commits = []string{}
	}

	releaseNotes, ok := resolveReleaseNotes(
		notes,
		opts,
		repoPath,
		repoName,
		tag,
		localTags,
		commits,
		cacheFile,
		aiReleaseNotesCache,
		failedAIGenerations,
		fmt.Sprintf("%s/%s:%s", target.Owner, repoName, tag),
		releaseNotesModeUpdate,
		SaveAIReleaseNotesCache,
	)
	if !ok {
		return
	}

	printReleaseNotesForReview("Updated Release Notes", target.Owner, repoName, tag, releaseNotes)

	msg := fmt.Sprintf("Update %s release for %s/%s tag %s?", target.Name, target.Owner, repoName, tag)
	if !decideReleaseAction(opts, confirm, "updating", target, repoName, tag, msg) {
		return
	}

	if err := target.Client.UpdateRelease(target.Owner, repoName, tag, releaseNotes); err != nil {
		fmt.Printf("  Error updating %s release: %v\n", target.Name, err)
	} else {
		fmt.Printf("  Updated %s release for tag %s\n", target.Name, tag)
	}
}

// resolveReleaseNotes picks the release notes to use for tag: standard
// (non-AI) notes when AI notes are disabled, a cached AI notes entry when
// one exists, or freshly generated AI notes otherwise (see
// generateAIReleaseNotes). Force controls sync scheduling and must not
// invalidate release-note cache entries, so no force/refresh path exists
// here.
func resolveReleaseNotes(
	notes releaseNotesSource,
	opts Options,
	repoPath, repoName, tag string,
	localTags, commits []string,
	cacheFile string,
	aiReleaseNotesCache map[string]string,
	failedAIGenerations *[]string,
	failedTarget string,
	mode releaseNotesMode,
	saveCache func(string, map[string]string) error,
) (string, bool) {
	if !opts.AIReleaseNotes {
		if mode == releaseNotesModeUpdate {
			return "", false
		}
		return notes.GenerateReleaseNotes(repoPath, tag, localTags), true
	}

	cacheKey := fmt.Sprintf("%s:%s", repoName, tag)
	if cachedNotes, exists := aiReleaseNotesCache[cacheKey]; exists {
		logCachedAIReleaseNotesUsed(tag, mode)
		return cachedNotes, true
	}

	return generateAIReleaseNotes(
		notes, opts, repoPath, repoName, tag, localTags, commits, cacheFile,
		aiReleaseNotesCache, failedAIGenerations, failedTarget, mode, saveCache,
	)
}

// logCachedAIReleaseNotesUsed prints the "using cached notes" message, with
// wording that differs slightly between the create and update pipelines.
func logCachedAIReleaseNotesUsed(tag string, mode releaseNotesMode) {
	if mode == releaseNotesModeUpdate {
		fmt.Printf("  Using cached AI release notes for existing release %s\n", tag)
	} else {
		fmt.Printf("  Using cached AI release notes for %s\n", tag)
	}
}

// generateAIReleaseNotes calls the AI notes generator for a tag not already
// in the cache. On success it caches the result; on failure it clears any
// stale cache entry, records the failure in failedAIGenerations, and - for
// new releases only (releaseNotesModeCreate) - falls back to standard
// (non-AI) release notes so creation can still proceed. An update
// (releaseNotesModeUpdate) has no such fallback: ok is false and the caller
// skips that release.
func generateAIReleaseNotes(
	notes releaseNotesSource,
	opts Options,
	repoPath, repoName, tag string,
	localTags, commits []string,
	cacheFile string,
	aiReleaseNotesCache map[string]string,
	failedAIGenerations *[]string,
	failedTarget string,
	mode releaseNotesMode,
	saveCache func(string, map[string]string) error,
) (string, bool) {
	cacheKey := fmt.Sprintf("%s:%s", repoName, tag)
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
