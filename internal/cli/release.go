package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codeberg.org/snonux/gitsyncer/internal/config"
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

type releaseTarget struct {
	name                  string
	owner                 string
	getReleases           func(owner, repo string) ([]string, error)
	createRelease         func(owner, repo, tag, releaseNotes string) error
	updateRelease         func(owner, repo, tag, releaseNotes string) error
	ensureReleasesEnabled func(owner, repo string) error
}

type releaseNotesGenerator interface {
	GenerateAIReleaseNotes(repoPath, repoName, tag string, allTags []string, commits []string) (string, error)
	GenerateReleaseNotes(repoPath, tag string, allTags []string) string
}

// HandleCheckReleasesForRepos checks for version tags without releases and creates them with confirmation
func HandleCheckReleasesForRepos(cfg *config.Config, flags *Flags, repositories []string) int {
	releaseManager := release.NewManager(flags.WorkDir)
	releaseManager.SetAITool(flags.AITool)

	// Load persistent AI release notes cache
	cacheFile := filepath.Join(flags.WorkDir, ".gitsyncer-ai-release-notes-cache.json")
	aiReleaseNotesCache := loadAIReleaseNotesCache(cacheFile)
	initialCacheSize := len(aiReleaseNotesCache)

	// Track failed AI generations
	failedAIGenerations := []string{}
	var releaseTargets []releaseTarget

	// Print summary at the end
	defer func() {
		if len(aiReleaseNotesCache) > initialCacheSize {
			fmt.Printf("\nAI release notes cache updated: %d new entries added (total: %d entries)\n",
				len(aiReleaseNotesCache)-initialCacheSize, len(aiReleaseNotesCache))
			fmt.Printf("Cache file: %s\n", cacheFile)
		}

		if len(failedAIGenerations) > 0 {
			fmt.Printf("\n⚠️  AI release notes generation failed for %d releases:\n", len(failedAIGenerations))
			for _, failed := range failedAIGenerations {
				fmt.Printf("  - %s\n", failed)
			}
			fmt.Println("\nThese releases were skipped. Their cache entries were cleared.")
			fmt.Println("Run again to retry generation for these releases.")
		}
	}()

	// Set tokens from config with fallback to environment variables and files
	githubOrg := cfg.FindGitHubOrg()
	if githubOrg != nil {
		fmt.Printf("Found GitHub org: %s\n", githubOrg.Name)

		// Try config token first, then fallback to env var and file
		token := githubOrg.GitHubToken
		if token == "" {
			// Try environment variable
			token = os.Getenv("GITHUB_TOKEN")
			if token == "" {
				// Try token file
				home, err := os.UserHomeDir()
				if err == nil {
					tokenFile := filepath.Join(home, ".gitsyncer_github_token")
					data, err := os.ReadFile(tokenFile)
					if err == nil {
						token = strings.TrimSpace(string(data))
					}
				}
			}
		}

		if token != "" {
			releaseManager.SetGitHubToken(token)
		} else {
			fmt.Println("WARNING: No GitHub token found - cannot create GitHub releases")
		}

		if githubOrg.Name != "" {
			releaseTargets = append(releaseTargets, releaseTarget{
				name:          "GitHub",
				owner:         githubOrg.Name,
				getReleases:   releaseManager.GetGitHubReleases,
				createRelease: releaseManager.CreateGitHubRelease,
				updateRelease: releaseManager.UpdateGitHubRelease,
			})
		}
	} else {
		fmt.Println("No GitHub organization found in config")
	}

	codebergOrg := cfg.FindCodebergOrg()
	if codebergOrg != nil {
		fmt.Printf("Found Codeberg org: %s\n", codebergOrg.Name)

		// Try config token first, then fallback to env var and file
		token := codebergOrg.CodebergToken
		if token == "" {
			// Try environment variable
			token = os.Getenv("CODEBERG_TOKEN")
			if token == "" {
				// Try token file
				home, err := os.UserHomeDir()
				if err == nil {
					tokenFile := filepath.Join(home, ".gitsyncer_codeberg_token")
					data, err := os.ReadFile(tokenFile)
					if err == nil {
						token = strings.TrimSpace(string(data))
					}
				}
			}
		}

		if token != "" {
			releaseManager.SetCodebergToken(token)
			fmt.Printf("  Codeberg token loaded (length: %d)\n", len(token))
		} else {
			fmt.Println("WARNING: No Codeberg token found - cannot create Codeberg releases")
		}

		if codebergOrg.Name != "" {
			releaseTargets = append(releaseTargets, releaseTarget{
				name:                  "Codeberg",
				owner:                 codebergOrg.Name,
				getReleases:           releaseManager.GetCodebergReleases,
				createRelease:         releaseManager.CreateCodebergRelease,
				updateRelease:         releaseManager.UpdateCodebergRelease,
				ensureReleasesEnabled: releaseManager.EnsureCodebergReleasesEnabled,
			})
		}
	} else {
		fmt.Println("No Codeberg organization found in config")
	}

	// Process the specified repositories
	for _, repoName := range repositories {
		fmt.Printf("\nChecking releases for repository: %s\n", repoName)

		// Check if the repository is cloned locally
		repoPath := filepath.Join(flags.WorkDir, repoName)
		if _, err := os.Stat(repoPath); os.IsNotExist(err) {
			fmt.Printf("  Repository not found locally at %s, skipping...\n", repoPath)
			continue
		}

		// Get local tags
		localTags, err := releaseManager.GetLocalTags(repoPath)
		if err != nil {
			fmt.Printf("  Error getting local tags: %v\n", err)
			continue
		}

		if len(localTags) == 0 {
			fmt.Println("  No version tags found")
			continue
		}

		fmt.Printf("  Found %d version tags: %s\n", len(localTags), strings.Join(localTags, ", "))
		// Log configured skip rules for this repo, if any
		if cfg.SkipReleases != nil {
			if skipTags, ok := cfg.SkipReleases[repoName]; ok && len(skipTags) > 0 {
				fmt.Printf("  Config skip_releases for %s: %s\n", repoName, strings.Join(skipTags, ", "))
			}
		}

		for _, target := range releaseTargets {
			missingReleases := getMissingReleasesForTarget(cfg, releaseManager, target, repoName, localTags)
			processCreateReleasesForTarget(
				cfg,
				flags,
				releaseManager,
				target,
				repoName,
				repoPath,
				localTags,
				missingReleases,
				cacheFile,
				aiReleaseNotesCache,
				&failedAIGenerations,
			)
		}

		// Update existing releases if requested
		if flags.UpdateReleases {
			for _, target := range releaseTargets {
				processUpdateReleasesForTarget(
					flags,
					releaseManager,
					target,
					repoName,
					repoPath,
					localTags,
					cacheFile,
					aiReleaseNotesCache,
					&failedAIGenerations,
				)
			}
		}
	}

	return 0
}

func getMissingReleasesForTarget(cfg *config.Config, releaseManager *release.Manager, target releaseTarget, repoName string, localTags []string) []string {
	releases, err := target.getReleases(target.owner, repoName)
	if err != nil {
		fmt.Printf("  Error checking %s releases: %v\n", target.name, err)
		return nil
	}

	missingReleases := releaseManager.FindMissingReleases(localTags, releases)
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
	releaseManager *release.Manager,
	target releaseTarget,
	repoName, repoPath string,
	localTags, missingReleases []string,
	cacheFile string,
	aiReleaseNotesCache map[string]string,
	failedAIGenerations *[]string,
) {
	if len(missingReleases) == 0 {
		return
	}

	if target.ensureReleasesEnabled != nil {
		if err := target.ensureReleasesEnabled(target.owner, repoName); err != nil {
			fmt.Printf("  Warning: Could not ensure %s releases are enabled: %v\n", target.name, err)
		}
	}

	for _, tag := range missingReleases {
		if cfg.ShouldSkipRelease(repoName, tag) {
			fmt.Printf("  Skipping %s release for %s:%s per config skip_releases\n", target.name, repoName, tag)
			continue
		}

		commits, err := releaseManager.GetCommitsSinceTag(repoPath, "", tag)
		if err != nil {
			commits = []string{}
		}

		releaseNotes, ok := resolveReleaseNotes(
			releaseManager,
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
			createRelease = release.PromptConfirmation(msg)
		}

		if createRelease {
			if err := target.createRelease(target.owner, repoName, tag, releaseNotes); err != nil {
				fmt.Printf("  Error creating %s release: %v\n", target.name, err)
			} else {
				fmt.Printf("  Created %s release for tag %s\n", target.name, tag)
			}
		}
	}
}

func processUpdateReleasesForTarget(
	flags *Flags,
	releaseManager *release.Manager,
	target releaseTarget,
	repoName, repoPath string,
	localTags []string,
	cacheFile string,
	aiReleaseNotesCache map[string]string,
	failedAIGenerations *[]string,
) {
	if !flags.AIReleaseNotes {
		return
	}

	existingReleases, err := target.getReleases(target.owner, repoName)
	if err != nil || len(existingReleases) == 0 {
		return
	}

	fmt.Printf("\n  Updating existing %s releases...\n", target.name)
	for _, tag := range existingReleases {
		if !version.IsVersionTag(tag) {
			continue
		}

		commits, err := releaseManager.GetCommitsSinceTag(repoPath, "", tag)
		if err != nil {
			commits = []string{}
		}

		releaseNotes, ok := resolveReleaseNotes(
			releaseManager,
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
			updateRelease = release.PromptConfirmation(msg)
		}

		if updateRelease {
			if err := target.updateRelease(target.owner, repoName, tag, releaseNotes); err != nil {
				fmt.Printf("  Error updating %s release: %v\n", target.name, err)
			} else {
				fmt.Printf("  Updated %s release for tag %s\n", target.name, tag)
			}
		}
	}
}

func resolveReleaseNotes(
	releaseManager releaseNotesGenerator,
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
		return releaseManager.GenerateReleaseNotes(repoPath, tag, localTags), true
	}

	cacheKey := fmt.Sprintf("%s:%s", repoName, tag)
	if cachedNotes, exists := aiReleaseNotesCache[cacheKey]; exists && !flags.Force {
		if mode == releaseNotesModeUpdate {
			fmt.Printf("  Using cached AI release notes for existing release %s\n", tag)
		} else {
			fmt.Printf("  Using cached AI release notes for %s\n", tag)
		}
		return cachedNotes, true
	}

	if flags.Force && aiReleaseNotesCache[cacheKey] != "" {
		if mode == releaseNotesModeUpdate {
			fmt.Printf("  Force regenerating AI release notes for existing release %s (ignoring cache)\n", tag)
		} else {
			fmt.Printf("  Force regenerating AI release notes for %s (ignoring cache)\n", tag)
		}
	} else {
		if mode == releaseNotesModeUpdate {
			fmt.Printf("  Generating AI release notes for existing release %s...\n", tag)
		} else {
			fmt.Printf("  Generating AI release notes for %s...\n", tag)
		}
	}

	aiNotes, err := releaseManager.GenerateAIReleaseNotes(repoPath, repoName, tag, localTags, commits)
	if err != nil {
		fmt.Printf("  Warning: Failed to generate AI release notes: %v\n", err)
		delete(aiReleaseNotesCache, cacheKey)
		*failedAIGenerations = append(*failedAIGenerations, failedTarget)
		_ = saveCache(cacheFile, aiReleaseNotesCache)

		if mode == releaseNotesModeCreate {
			fmt.Printf("  Falling back to standard release notes\n")
			return releaseManager.GenerateReleaseNotes(repoPath, tag, localTags), true
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
