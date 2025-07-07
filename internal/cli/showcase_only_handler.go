package cli

import (
	"fmt"
	"log"

	"codeberg.org/snonux/gitsyncer/internal/codeberg"
	"codeberg.org/snonux/gitsyncer/internal/config"
	"codeberg.org/snonux/gitsyncer/internal/github"
	"codeberg.org/snonux/gitsyncer/internal/showcase"
	"codeberg.org/snonux/gitsyncer/internal/sync"
)

// HandleShowcaseOnly handles showcase generation without syncing
// It will clone repositories if they don't exist locally, but won't sync changes
func HandleShowcaseOnly(cfg *config.Config, flags *Flags) int {
	// Get all repositories from all sources
	allRepos, err := getAllRepositories(cfg)
	if err != nil {
		log.Printf("ERROR: Failed to get repositories: %v\n", err)
		return 1
	}
	
	if len(allRepos) == 0 {
		fmt.Println("No repositories found")
		return 1
	}
	
	fmt.Printf("Found %d repositories total\n", len(allRepos))
	
	// Create a minimal syncer just for cloning
	syncer := sync.New(cfg, flags.WorkDir)
	syncer.SetBackupEnabled(false) // Never use backup in showcase-only mode
	
	// Ensure repositories are cloned (but not synced)
	fmt.Println("\nEnsuring repositories are cloned locally...")
	for _, repo := range allRepos {
		if err := syncer.EnsureRepositoryCloned(repo); err != nil {
			fmt.Printf("WARNING: Failed to clone %s: %v\n", repo, err)
			// Continue with other repos
		}
	}
	
	// Generate showcase for all repositories
	fmt.Println("\nGenerating showcase for all repositories...")
	generator := showcase.New(cfg, flags.WorkDir)
	
	// Pass empty filter to process all repos
	if err := generator.GenerateShowcase(nil, flags.Force); err != nil {
		log.Printf("ERROR: Failed to generate showcase: %v\n", err)
		return 1
	}
	
	fmt.Println("Showcase generation completed!")
	return 0
}

// getAllRepositories collects all unique repository names from all sources
func getAllRepositories(cfg *config.Config) ([]string, error) {
	repoMap := make(map[string]bool)
	
	// Add configured repositories
	for _, repo := range cfg.Repositories {
		repoMap[repo] = true
	}
	
	// Add Codeberg public repos if configured
	if codebergOrg := cfg.FindCodebergOrg(); codebergOrg != nil {
		fmt.Printf("Fetching public repositories from Codeberg user/org: %s...\n", codebergOrg.Name)
		client := codeberg.NewClient(codebergOrg.Name, codebergOrg.CodebergToken)
		
		repos, err := client.ListPublicRepos()
		if err != nil {
			// Try as user
			repos, err = client.ListUserPublicRepos()
			if err != nil {
				fmt.Printf("Warning: Failed to fetch Codeberg repos: %v\n", err)
			}
		}
		
		for _, repo := range repos {
			repoMap[repo.Name] = true
		}
	}
	
	// Add GitHub public repos if configured
	if githubOrg := cfg.FindGitHubOrg(); githubOrg != nil {
		fmt.Printf("Fetching public repositories from GitHub user/org: %s...\n", githubOrg.Name)
		client := github.NewClient(githubOrg.GitHubToken, githubOrg.Name)
		
		if client.HasToken() {
			repos, err := client.ListPublicRepos()
			if err != nil {
				fmt.Printf("Warning: Failed to fetch GitHub repos: %v\n", err)
			} else {
				for _, repo := range repos {
					repoMap[repo.Name] = true
				}
			}
		} else {
			fmt.Println("Warning: No GitHub token found, skipping GitHub repos")
		}
	}
	
	// Convert map to slice
	var allRepos []string
	for repo := range repoMap {
		allRepos = append(allRepos, repo)
	}
	
	return allRepos, nil
}