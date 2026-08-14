package cli

// Release-checking orchestration (finding version tags missing a release,
// generating/caching AI release notes, and creating or updating releases)
// used to live in this file. Task w01 moved that domain logic to
// internal/release (see orchestrator.go, targets.go, cache.go), leaving
// this file as a thin translation layer: it discovers which repositories to
// check, converts Flags into release.Options, and delegates to
// release.CheckReleasesForRepos with promptConfirmation injected as the
// confirmation callback.

import (
	"fmt"

	"codeberg.org/snonux/gitsyncer/internal/config"
	"codeberg.org/snonux/gitsyncer/internal/localrepos"
	"codeberg.org/snonux/gitsyncer/internal/release"
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

// HandleCheckReleasesForRepos checks for version tags without releases and
// creates them with confirmation. It translates Flags into
// release.Options and delegates the actual orchestration (target
// resolution, AI-notes caching, create/update pipeline) to
// release.CheckReleasesForRepos.
func HandleCheckReleasesForRepos(cfg *config.Config, flags *Flags, repositories []string) int {
	opts := release.Options{
		DryRun:             flags.DryRun,
		WorkDir:            flags.WorkDir,
		AutoCreateReleases: flags.AutoCreateReleases,
		AIReleaseNotes:     flags.AIReleaseNotes,
		UpdateReleases:     flags.UpdateReleases,
		AITool:             flags.AITool,
	}
	return release.CheckReleasesForRepos(cfg, opts, repositories, promptConfirmation)
}
