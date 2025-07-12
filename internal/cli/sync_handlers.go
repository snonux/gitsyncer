package cli

import (
	"fmt"
	"log"
	"strings"

	"codeberg.org/snonux/gitsyncer/internal/codeberg"
	"codeberg.org/snonux/gitsyncer/internal/config"
	"codeberg.org/snonux/gitsyncer/internal/github"
	"codeberg.org/snonux/gitsyncer/internal/sync"
)

// HandleSync handles syncing a single repository
func HandleSync(cfg *config.Config, flags *Flags) int {
	// If create-github-repos is enabled, create the repo if needed
	if flags.CreateGitHubRepos {
		if err := createGitHubRepoIfNeeded(cfg, flags.SyncRepo); err != nil {
			fmt.Printf("ERROR: %v\n", err)
			return 1
		}
	}
	
	syncer := sync.New(cfg, flags.WorkDir)
	syncer.SetBackupEnabled(flags.Backup)
	if err := syncer.SyncRepository(flags.SyncRepo); err != nil {
		log.Fatal("Sync failed:", err)
		return 1
	}
	return 0
}

// HandleSyncAll handles syncing all configured repositories
func HandleSyncAll(cfg *config.Config, flags *Flags) int {
	if len(cfg.Repositories) == 0 {
		fmt.Println("No repositories configured. Add repositories to the config file.")
		return 1
	}

	// Initialize GitHub client if needed
	var githubClient github.Client
	var hasGithubClient bool
	if flags.CreateGitHubRepos {
		if client := initGitHubClient(cfg); client != nil {
			githubClient = *client
			hasGithubClient = true
		}
	}

	syncer := sync.New(cfg, flags.WorkDir)
	syncer.SetBackupEnabled(flags.Backup)
	successCount := 0
	
	for i, repo := range cfg.Repositories {
		fmt.Printf("\n[%d/%d] Syncing %s...\n", i+1, len(cfg.Repositories), repo)
		
		// Create GitHub repo if needed
		if hasGithubClient {
			if err := createRepoWithClient(&githubClient, repo, fmt.Sprintf("Mirror of %s", repo)); err != nil {
				fmt.Printf("ERROR: Failed to create GitHub repo %s: %v\n", repo, err)
				fmt.Printf("Stopping sync due to error.\n")
				return 1
			}
		}
		
		if err := syncer.SyncRepository(repo); err != nil {
			fmt.Printf("ERROR: Failed to sync %s: %v\n", repo, err)
			fmt.Printf("Stopping sync due to error.\n")
			return 1
		}
		successCount++
	}
	
	fmt.Printf("\nSuccessfully synced all %d repositories!\n", successCount)
	
	// Print abandoned branches summary
	if summary := syncer.GenerateAbandonedBranchSummary(); summary != "" {
		fmt.Print(summary)
	}
	
	// Generate script for abandoned branches
	if scriptPath, err := syncer.GenerateDeleteScript(); err != nil {
		fmt.Printf("\n⚠️  Failed to generate script: %v\n", err)
	} else if scriptPath != "" {
		fmt.Printf("\n")
		fmt.Printf(strings.Repeat("=", 70))
		fmt.Printf("\n📋 ABANDONED BRANCH MANAGEMENT SCRIPT\n")
		fmt.Printf(strings.Repeat("=", 70))
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
		fmt.Printf(strings.Repeat("=", 70))
		fmt.Printf("\n")
	}
	
	return 0
}

// HandleSyncCodebergPublic handles syncing all public Codeberg repositories
func HandleSyncCodebergPublic(cfg *config.Config, flags *Flags) int {
	codebergOrg := cfg.FindCodebergOrg()
	if codebergOrg == nil {
		fmt.Println("No Codeberg organization found in configuration")
		return 1
	}

	fmt.Printf("Fetching public repositories from Codeberg user/org: %s...\n", codebergOrg.Name)
	
	client := codeberg.NewClient(codebergOrg.Name, codebergOrg.CodebergToken)
	
	// Try fetching as organization first, then as user
	repos, err := client.ListPublicRepos()
	if err != nil {
		fmt.Println("Trying as user account...")
		repos, err = client.ListUserPublicRepos()
		if err != nil {
			log.Fatal("Failed to fetch repositories:", err)
		}
	}

	repoNames := codeberg.GetRepoNames(repos)
	fmt.Printf("Found %d public repositories on Codeberg\n", len(repoNames))
	
	if len(repoNames) == 0 {
		fmt.Println("No public repositories found")
		return 0
	}

	// Show the repositories that will be synced
	showReposToSync(repoNames)
	
	if flags.DryRun {
		fmt.Printf("\n[DRY RUN] Would sync %d repositories from Codeberg to GitHub\n", len(repoNames))
		if flags.CreateGitHubRepos {
			fmt.Println("Would create missing GitHub repositories")
		}
		if !flags.SyncGitHubPublic {
			return 0
		}
	}
	
	if !flags.DryRun {
		return syncCodebergRepos(cfg, flags, repos, repoNames)
	}
	
	return 0
}

// HandleSyncGitHubPublic handles syncing all public GitHub repositories
func HandleSyncGitHubPublic(cfg *config.Config, flags *Flags) int {
	githubOrg := cfg.FindGitHubOrg()
	if githubOrg == nil {
		fmt.Println("No GitHub organization found in configuration")
		return 1
	}

	fmt.Printf("Fetching public repositories from GitHub user/org: %s...\n", githubOrg.Name)
	
	client := github.NewClient(githubOrg.GitHubToken, githubOrg.Name)
	if !client.HasToken() {
		fmt.Println("ERROR: GitHub token required to list repositories")
		fmt.Println("Set GITHUB_TOKEN env var or create ~/.gitsyncer_github_token file")
		return 1
	}
	
	repos, err := client.ListPublicRepos()
	if err != nil {
		log.Fatal("Failed to fetch repositories:", err)
	}

	repoNames := github.GetRepoNames(repos)
	fmt.Printf("Found %d public repositories on GitHub\n", len(repoNames))
	
	if len(repoNames) == 0 {
		fmt.Println("No public repositories found")
		return 0
	}

	// Show the repositories that will be synced
	showReposToSync(repoNames)
	
	if flags.DryRun {
		fmt.Printf("\n[DRY RUN] Would sync %d repositories from GitHub to Codeberg\n", len(repoNames))
		if flags.CreateCodebergRepos {
			fmt.Println("Would create missing Codeberg repositories")
		}
		return 0
	}
	
	if !flags.DryRun {
		return syncGitHubRepos(cfg, flags, repos, repoNames)
	}
	
	return 0
}

// Helper functions

func createGitHubRepoIfNeeded(cfg *config.Config, repoName string) error {
	githubOrg := cfg.FindGitHubOrg()
	if githubOrg == nil {
		return nil
	}
	
	fmt.Printf("Initializing GitHub client for organization: %s\n", githubOrg.Name)
	githubClient := github.NewClient(githubOrg.GitHubToken, githubOrg.Name)
	if !githubClient.HasToken() {
		fmt.Println("Warning: No GitHub token found. Cannot create repository.")
		return nil
	}
	
	fmt.Println("Checking/creating GitHub repository...")
	return githubClient.CreateRepo(repoName, fmt.Sprintf("Mirror of %s", repoName), false)
}

func initGitHubClient(cfg *config.Config) *github.Client {
	githubOrg := cfg.FindGitHubOrg()
	if githubOrg == nil {
		fmt.Println("Warning: --create-github-repos specified but no GitHub organization found in config")
		return nil
	}
	
	fmt.Printf("Initializing GitHub client for organization: %s\n", githubOrg.Name)
	githubClient := github.NewClient(githubOrg.GitHubToken, githubOrg.Name)
	if !githubClient.HasToken() {
		fmt.Println("Warning: No GitHub token found. Cannot create repositories.")
		return nil
	}
	
	fmt.Println("GitHub client initialized successfully with token")
	return &githubClient
}

func createRepoWithClient(client *github.Client, repoName, description string) error {
	fmt.Printf("Checking/creating GitHub repository %s...\n", repoName)
	return client.CreateRepo(repoName, description, false)
}

func initCodebergClient(cfg *config.Config) *codeberg.Client {
	codebergOrg := cfg.FindCodebergOrg()
	if codebergOrg == nil {
		fmt.Println("Warning: --create-codeberg-repos specified but no Codeberg organization found in config")
		return nil
	}

	fmt.Printf("Initializing Codeberg client for organization: %s\n", codebergOrg.Name)
	codebergClient := codeberg.NewClient(codebergOrg.Name, codebergOrg.CodebergToken)
	if !codebergClient.HasToken() {
		fmt.Println("Warning: No Codeberg token found. Cannot create repositories.")
		return nil
	}

	fmt.Println("Codeberg client initialized successfully with token")
	return &codebergClient
}

func showReposToSync(repoNames []string) {
	fmt.Println("\nRepositories to sync:")
	for _, name := range repoNames {
		fmt.Printf("  - %s\n", name)
	}
}

func printFullSyncSeparator() {
	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("=== Continuing with GitHub to Codeberg sync ===")
	fmt.Println(strings.Repeat("=", 70) + "\n")
}

func syncCodebergRepos(cfg *config.Config, flags *Flags, repos []codeberg.Repository, repoNames []string) int {
	// Initialize GitHub client if needed
	var githubClient github.Client
	var hasGithubClient bool
	if flags.CreateGitHubRepos {
		if client := initGitHubClient(cfg); client != nil {
			githubClient = *client
			hasGithubClient = true
		}
	}
	
	fmt.Printf("\nStarting sync of %d repositories...\n", len(repoNames))
	
	syncer := sync.New(cfg, flags.WorkDir)
	syncer.SetBackupEnabled(flags.Backup)
	successCount := 0
	
	// Create map for descriptions
	repoMap := make(map[string]codeberg.Repository)
	for _, repo := range repos {
		repoMap[repo.Name] = repo
	}
	
	for i, repoName := range repoNames {
		fmt.Printf("\n[%d/%d] Syncing %s...\n", i+1, len(repoNames), repoName)
		
		// Create GitHub repo if needed
		if hasGithubClient && flags.CreateGitHubRepos {
			codebergRepo := repoMap[repoName]
			description := codebergRepo.Description
			if description == "" {
				description = fmt.Sprintf("Mirror of %s from Codeberg", repoName)
			}
			
			fmt.Printf("Checking/creating GitHub repository %s...\n", repoName)
			err := githubClient.CreateRepo(repoName, description, false)
			if err != nil {
				fmt.Printf("Warning: Failed to create GitHub repo %s: %v\n", repoName, err)
			}
		}
		
		if err := syncer.SyncRepository(repoName); err != nil {
			fmt.Printf("ERROR: Failed to sync %s: %v\n", repoName, err)
			fmt.Printf("Stopping sync due to error.\n")
			return 1
		}
		successCount++
	}

	fmt.Printf("\n=== Summary ===\n")
	fmt.Printf("Successfully synced: %d repositories\n", successCount)
	
	// Print abandoned branches summary
	if summary := syncer.GenerateAbandonedBranchSummary(); summary != "" {
		fmt.Print(summary)
	}
	
	// Generate script for abandoned branches
	if scriptPath, err := syncer.GenerateDeleteScript(); err != nil {
		fmt.Printf("\n⚠️  Failed to generate script: %v\n", err)
	} else if scriptPath != "" {
		fmt.Printf("\n")
		fmt.Printf(strings.Repeat("=", 70))
		fmt.Printf("\n📋 ABANDONED BRANCH MANAGEMENT SCRIPT\n")
		fmt.Printf(strings.Repeat("=", 70))
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
		fmt.Printf(strings.Repeat("=", 70))
		fmt.Printf("\n")
	}
	
	if !flags.SyncGitHubPublic {
		return 0
	}
	
	// Print separator for full sync
	printFullSyncSeparator()
	return 0
}

func syncGitHubRepos(cfg *config.Config, flags *Flags, repos []github.Repository, repoNames []string) int {
	// Initialize Codeberg client if needed
	var codebergClient codeberg.Client
	var hasCodebergClient bool
	if flags.CreateCodebergRepos {
		if client := initCodebergClient(cfg); client != nil {
			codebergClient = *client
			hasCodebergClient = true
		}
	}

	fmt.Printf("\nStarting sync of %d repositories...\n", len(repoNames))

	syncer := sync.New(cfg, flags.WorkDir)
	syncer.SetBackupEnabled(flags.Backup)
	successCount := 0

	// Create map for descriptions
	repoMap := make(map[string]github.Repository)
	for _, repo := range repos {
		repoMap[repo.Name] = repo
	}

	for i, repoName := range repoNames {
		fmt.Printf("\n[%d/%d] Syncing %s...\n", i+1, len(repoNames), repoName)

		// Create Codeberg repo if needed
		if hasCodebergClient && flags.CreateCodebergRepos {
			githubRepo := repoMap[repoName]
			description := githubRepo.Description
			if description == "" {
				description = fmt.Sprintf("Mirror of %s from GitHub", repoName)
			}

			fmt.Printf("Checking/creating Codeberg repository %s...\n", repoName)
			err := codebergClient.CreateRepo(repoName, description, false)
			if err != nil {
				fmt.Printf("Warning: Failed to create Codeberg repo %s: %v\n", repoName, err)
			}
		}

		if err := syncer.SyncRepository(repoName); err != nil {
			fmt.Printf("ERROR: Failed to sync %s: %v\n", repoName, err)
			fmt.Printf("Stopping sync due to error.\n")
			return 1
		}
		successCount++
	}

	fmt.Printf("\n=== Summary ===\n")
	fmt.Printf("Successfully synced: %d repositories\n", successCount)

	// Print abandoned branches summary
	if summary := syncer.GenerateAbandonedBranchSummary(); summary != "" {
		fmt.Print(summary)
	}
	
	// Generate script for abandoned branches
	if scriptPath, err := syncer.GenerateDeleteScript(); err != nil {
		fmt.Printf("\n⚠️  Failed to generate script: %v\n", err)
	} else if scriptPath != "" {
		fmt.Printf("\n")
		fmt.Printf(strings.Repeat("=", 70))
		fmt.Printf("\n📋 ABANDONED BRANCH MANAGEMENT SCRIPT\n")
		fmt.Printf(strings.Repeat("=", 70))
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
		fmt.Printf(strings.Repeat("=", 70))
		fmt.Printf("\n")
	}

	return 0
}

// ShowFullSyncMessage displays the full sync mode message
func ShowFullSyncMessage() {
	fmt.Println("Full sync mode enabled:")
	fmt.Println("  - Sync all public Codeberg repos to GitHub")
	fmt.Println("  - Sync all public GitHub repos to Codeberg")
	fmt.Println("  - Create missing GitHub repositories")
	fmt.Println("  - Create missing Codeberg repositories (when implemented)")
	fmt.Println()
}