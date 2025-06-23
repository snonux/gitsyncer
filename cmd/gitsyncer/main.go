package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"codeberg.org/snonux/gitsyncer/internal/codeberg"
	"codeberg.org/snonux/gitsyncer/internal/config"
	"codeberg.org/snonux/gitsyncer/internal/github"
	"codeberg.org/snonux/gitsyncer/internal/sync"
	"codeberg.org/snonux/gitsyncer/internal/version"
)

func main() {
	var (
		versionFlag        bool
		configPath         string
		listOrgs           bool
		listRepos          bool
		syncRepo           string
		syncAll            bool
		syncCodebergPublic bool
		syncGitHubPublic   bool
		fullSync           bool
		createGitHubRepos  bool
		createCodebergRepos bool
		dryRun             bool
		workDir            string
		testGitHubToken    bool
	)

	// Define command line flags
	flag.BoolVar(&versionFlag, "version", false, "print version information")
	flag.BoolVar(&versionFlag, "v", false, "print version information (short)")
	flag.StringVar(&configPath, "config", "", "path to configuration file")
	flag.StringVar(&configPath, "c", "", "path to configuration file (short)")
	flag.BoolVar(&listOrgs, "list-orgs", false, "list configured organizations")
	flag.BoolVar(&listRepos, "list-repos", false, "list configured repositories")
	flag.StringVar(&syncRepo, "sync", "", "repository name to sync")
	flag.BoolVar(&syncAll, "sync-all", false, "sync all configured repositories")
	flag.BoolVar(&syncCodebergPublic, "sync-codeberg-public", false, "sync all public Codeberg repositories to GitHub")
	flag.BoolVar(&syncGitHubPublic, "sync-github-public", false, "sync all public GitHub repositories to Codeberg")
	flag.BoolVar(&fullSync, "full", false, "full bidirectional sync (enables --sync-codeberg-public --sync-github-public --create-github-repos --create-codeberg-repos)")
	flag.BoolVar(&createGitHubRepos, "create-github-repos", false, "automatically create missing GitHub repositories")
	flag.BoolVar(&createCodebergRepos, "create-codeberg-repos", false, "automatically create missing Codeberg repositories")
	flag.BoolVar(&dryRun, "dry-run", false, "show what would be synced without actually syncing")
	flag.StringVar(&workDir, "work-dir", ".gitsyncer-work", "working directory for cloning repositories")
	flag.BoolVar(&testGitHubToken, "test-github-token", false, "test GitHub token authentication")
	flag.Parse()

	// Handle --full flag by enabling all sync operations
	if fullSync {
		syncCodebergPublic = true
		syncGitHubPublic = true
		createGitHubRepos = true
		createCodebergRepos = true
		fmt.Println("Full sync mode enabled:")
		fmt.Println("  - Sync all public Codeberg repos to GitHub")
		fmt.Println("  - Sync all public GitHub repos to Codeberg")
		fmt.Println("  - Create missing GitHub repositories")
		fmt.Println("  - Create missing Codeberg repositories (when implemented)")
		fmt.Println()
	}

	// Handle version flag
	if versionFlag {
		fmt.Println(version.GetVersion())
		os.Exit(0)
	}
	
	// Handle test GitHub token flag
	if testGitHubToken {
		fmt.Println("Testing GitHub token authentication...")
		client := github.NewClient("", "snonux") // Empty token to trigger loading from env/file
		if !client.HasToken() {
			fmt.Println("ERROR: No GitHub token found!")
			fmt.Println("Please set GITHUB_TOKEN environment variable or create ~/.gitsyncer_github_token file")
			os.Exit(1)
		}
		
		// Test the token by checking a known repo
		exists, err := client.RepoExists("gitsyncer")
		if err != nil {
			fmt.Printf("ERROR: Token test failed: %v\n", err)
			if strings.Contains(err.Error(), "401") {
				fmt.Println("\nThe token is invalid or expired. Please check:")
				fmt.Println("1. Token has not expired")
				fmt.Println("2. Token has 'repo' scope")
				fmt.Println("3. Token was not revoked")
			}
			os.Exit(1)
		}
		
		fmt.Printf("SUCCESS: Token is valid! Repository check returned: %v\n", exists)
		os.Exit(0)
	}

	// Determine config file path
	if configPath == "" {
		// Try default locations
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatal("Failed to get home directory:", err)
		}

		// Check common config locations
		configLocations := []string{
			filepath.Join(".", "gitsyncer.json"),
			filepath.Join(home, ".config", "gitsyncer", "config.json"),
			filepath.Join(home, ".gitsyncer.json"),
		}

		for _, loc := range configLocations {
			if _, err := os.Stat(loc); err == nil {
				configPath = loc
				break
			}
		}

		if configPath == "" {
			fmt.Println("No configuration file found. Please create one of:")
			for _, loc := range configLocations {
				fmt.Printf("  - %s\n", loc)
			}
			fmt.Println("\nOr specify a config file with --config flag")
			fmt.Println("\nExample configuration:")
			fmt.Println(`{
  "organizations": [
    {"host": "git@github.com", "name": "myorg"},
    {"host": "git@codeberg.org", "name": "myorg"}
  ],
  "repositories": [
    "repo1",
    "repo2"
  ]
}`)
			os.Exit(1)
		}
	}

	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatal("Failed to load configuration:", err)
	}

	fmt.Printf("Loaded configuration from: %s\n", configPath)

	// Handle list organizations flag
	if listOrgs {
		fmt.Println("\nConfigured organizations:")
		for _, org := range cfg.Organizations {
			fmt.Printf("  - %s\n", org.GetGitURL())
		}
		os.Exit(0)
	}

	// Handle list repositories flag
	if listRepos {
		fmt.Println("\nConfigured repositories:")
		if len(cfg.Repositories) == 0 {
			fmt.Println("  (none configured)")
		} else {
			for _, repo := range cfg.Repositories {
				fmt.Printf("  - %s\n", repo)
			}
		}
		os.Exit(0)
	}

	// Handle sync operation
	if syncRepo != "" {
		// If create-github-repos is enabled, create the repo if needed
		if createGitHubRepos {
			githubOrg := cfg.FindGitHubOrg()
			if githubOrg != nil {
				fmt.Printf("Initializing GitHub client for organization: %s\n", githubOrg.Name)
				githubClient := github.NewClient(githubOrg.GitHubToken, githubOrg.Name)
				if githubClient.HasToken() {
					fmt.Println("Checking/creating GitHub repository...")
					err := githubClient.CreateRepo(syncRepo, fmt.Sprintf("Mirror of %s", syncRepo), false)
					if err != nil {
						fmt.Printf("ERROR: Failed to create GitHub repo %s: %v\n", syncRepo, err)
						os.Exit(1)
					}
				} else {
					fmt.Println("Warning: No GitHub token found. Cannot create repository.")
				}
			}
		}
		
		syncer := sync.New(cfg, workDir)
		if err := syncer.SyncRepository(syncRepo); err != nil {
			log.Fatal("Sync failed:", err)
		}
		os.Exit(0)
	}

	// Handle sync all operation
	if syncAll {
		if len(cfg.Repositories) == 0 {
			fmt.Println("No repositories configured. Add repositories to the config file.")
			os.Exit(1)
		}

		// Initialize GitHub client if needed
		var githubClient *github.Client
		if createGitHubRepos {
			githubOrg := cfg.FindGitHubOrg()
			if githubOrg != nil {
				fmt.Printf("Initializing GitHub client for organization: %s\n", githubOrg.Name)
				githubClient = github.NewClient(githubOrg.GitHubToken, githubOrg.Name)
				if !githubClient.HasToken() {
					fmt.Println("Warning: No GitHub token found. Cannot create repositories.")
					githubClient = nil
				} else {
					fmt.Println("GitHub client initialized successfully with token")
				}
			}
		}

		syncer := sync.New(cfg, workDir)
		successCount := 0
		
		for i, repo := range cfg.Repositories {
			fmt.Printf("\n[%d/%d] Syncing %s...\n", i+1, len(cfg.Repositories), repo)
			
			// Create GitHub repo if needed
			if githubClient != nil {
				fmt.Printf("Checking/creating GitHub repository %s...\n", repo)
				err := githubClient.CreateRepo(repo, fmt.Sprintf("Mirror of %s", repo), false)
				if err != nil {
					fmt.Printf("ERROR: Failed to create GitHub repo %s: %v\n", repo, err)
					fmt.Printf("Stopping sync due to error.\n")
					os.Exit(1)
				}
			}
			
			if err := syncer.SyncRepository(repo); err != nil {
				fmt.Printf("ERROR: Failed to sync %s: %v\n", repo, err)
				fmt.Printf("Stopping sync due to error.\n")
				os.Exit(1)
			}
			successCount++
		}
		
		fmt.Printf("\nSuccessfully synced all %d repositories!\n", len(cfg.Repositories))
		os.Exit(0)
	}

	// Handle sync Codeberg public repos
	if syncCodebergPublic {
		codebergOrg := cfg.FindCodebergOrg()
		if codebergOrg == nil {
			fmt.Println("No Codeberg organization found in configuration")
			os.Exit(1)
		}

		fmt.Printf("Fetching public repositories from Codeberg user/org: %s...\n", codebergOrg.Name)
		
		client := codeberg.NewClient(codebergOrg.Name)
		
		// Try fetching as organization first
		repos, err := client.ListPublicRepos()
		if err != nil {
			// If that fails, try as user
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
			os.Exit(0)
		}

		// Show the repositories that will be synced
		fmt.Println("\nRepositories to sync:")
		for _, name := range repoNames {
			fmt.Printf("  - %s\n", name)
		}
		
		if dryRun {
			fmt.Printf("\n[DRY RUN] Would sync %d repositories from Codeberg to GitHub\n", len(repoNames))
			if createGitHubRepos {
				fmt.Println("Would create missing GitHub repositories")
			}
			if !syncGitHubPublic {
				os.Exit(0)
			}
		}
		
		if !dryRun {
			// If create-github-repos is enabled, pre-create repos on GitHub
			var githubClient *github.Client
		if createGitHubRepos {
			githubOrg := cfg.FindGitHubOrg()
			if githubOrg == nil {
				fmt.Println("Warning: --create-github-repos specified but no GitHub organization found in config")
			} else {
				fmt.Printf("Initializing GitHub client for organization: %s\n", githubOrg.Name)
				githubClient = github.NewClient(githubOrg.GitHubToken, githubOrg.Name)
				if !githubClient.HasToken() {
					fmt.Println("Warning: No GitHub token found. Set GITHUB_TOKEN env var or create ~/.gitsyncer_github_token")
					fmt.Println("         or add github_token to your config file")
					githubClient = nil
				} else {
					fmt.Println("GitHub client initialized successfully with token")
				}
			}
		}
		
		fmt.Printf("\nStarting sync of %d repositories...\n", len(repoNames))
		
		syncer := sync.New(cfg, workDir)
		successCount := 0
		
		// Get Codeberg repos for description
		codebergRepoMap := make(map[string]codeberg.Repository)
		for _, repo := range repos {
			codebergRepoMap[repo.Name] = repo
		}
		
		for i, repoName := range repoNames {
			fmt.Printf("\n[%d/%d] Syncing %s...\n", i+1, len(repoNames), repoName)
			
			// Create GitHub repo if needed
			if githubClient != nil && createGitHubRepos {
				codebergRepo := codebergRepoMap[repoName]
				description := codebergRepo.Description
				if description == "" {
					description = fmt.Sprintf("Mirror of %s from Codeberg", repoName)
				}
				
				fmt.Printf("Checking/creating GitHub repository %s...\n", repoName)
				err := githubClient.CreateRepo(repoName, description, false) // public repos
				if err != nil {
					fmt.Printf("ERROR: Failed to create GitHub repo %s: %v\n", repoName, err)
					fmt.Printf("Stopping sync due to error.\n")
					os.Exit(1)
				}
			}
			
			if err := syncer.SyncRepository(repoName); err != nil {
				fmt.Printf("ERROR: Failed to sync %s: %v\n", repoName, err)
				fmt.Printf("Stopping sync due to error.\n")
				os.Exit(1)
			} else {
				successCount++
			}
		}

		fmt.Printf("\n=== Summary ===\n")
		fmt.Printf("Successfully synced: %d repositories\n", successCount)
		} // End of if !dryRun
		
		if !syncGitHubPublic {
			os.Exit(0)
		}
		
		// Add separator when doing full sync
		fmt.Println("\n" + strings.Repeat("=", 70))
		fmt.Println("=== Continuing with GitHub to Codeberg sync ===")
		fmt.Println(strings.Repeat("=", 70) + "\n")
	}

	// Handle sync GitHub public repos
	if syncGitHubPublic {
		githubOrg := cfg.FindGitHubOrg()
		if githubOrg == nil {
			fmt.Println("No GitHub organization found in configuration")
			os.Exit(1)
		}

		fmt.Printf("Fetching public repositories from GitHub user/org: %s...\n", githubOrg.Name)
		
		client := github.NewClient(githubOrg.GitHubToken, githubOrg.Name)
		if !client.HasToken() {
			fmt.Println("ERROR: GitHub token required to list repositories")
			fmt.Println("Set GITHUB_TOKEN env var or create ~/.gitsyncer_github_token file")
			os.Exit(1)
		}
		
		repos, err := client.ListPublicRepos()
		if err != nil {
			log.Fatal("Failed to fetch repositories:", err)
		}

		repoNames := github.GetRepoNames(repos)
		fmt.Printf("Found %d public repositories on GitHub\n", len(repoNames))
		
		if len(repoNames) == 0 {
			fmt.Println("No public repositories found")
			os.Exit(0)
		}

		// Show the repositories that will be synced
		fmt.Println("\nRepositories to sync:")
		for _, name := range repoNames {
			fmt.Printf("  - %s\n", name)
		}
		
		if dryRun {
			fmt.Printf("\n[DRY RUN] Would sync %d repositories from GitHub to Codeberg\n", len(repoNames))
			if createCodebergRepos {
				fmt.Println("Would create missing Codeberg repositories")
			}
			os.Exit(0)
		}
		
		if !dryRun {
			// TODO: Add Codeberg API client for repo creation
			if createCodebergRepos {
			fmt.Println("WARNING: --create-codeberg-repos is not yet implemented")
			fmt.Println("         Repositories must exist on Codeberg before syncing")
		}
		
		fmt.Printf("\nStarting sync of %d repositories...\n", len(repoNames))
		
		syncer := sync.New(cfg, workDir)
		successCount := 0
		
		// Get GitHub repos for description
		githubRepoMap := make(map[string]github.Repository)
		for _, repo := range repos {
			githubRepoMap[repo.Name] = repo
		}
		
		for i, repoName := range repoNames {
			fmt.Printf("\n[%d/%d] Syncing %s...\n", i+1, len(repoNames), repoName)
			
			if err := syncer.SyncRepository(repoName); err != nil {
				fmt.Printf("ERROR: Failed to sync %s: %v\n", repoName, err)
				fmt.Printf("Stopping sync due to error.\n")
				os.Exit(1)
			} else {
				successCount++
			}
		}

		fmt.Printf("\n=== Summary ===\n")
		fmt.Printf("Successfully synced: %d repositories\n", successCount)
		} // End of if !dryRun
		
		os.Exit(0)
	}

	// Default: show usage
	fmt.Println("\ngitsyncer - Git repository synchronization tool")
	fmt.Printf("Configured with %d organization(s) and %d repository(ies)\n", 
		len(cfg.Organizations), len(cfg.Repositories))
	fmt.Println("\nUsage:")
	fmt.Println("  gitsyncer --sync <repo-name>        Sync a specific repository")
	fmt.Println("  gitsyncer --sync-all                Sync all configured repositories")
	fmt.Println("  gitsyncer --sync-codeberg-public    Sync all public Codeberg repositories to GitHub")
	fmt.Println("  gitsyncer --sync-github-public      Sync all public GitHub repositories to Codeberg")
	fmt.Println("  gitsyncer --full                    Full bidirectional sync of all public repos")
	fmt.Println("  gitsyncer --list-orgs               List configured organizations")
	fmt.Println("  gitsyncer --list-repos              List configured repositories")
	fmt.Println("  gitsyncer --test-github-token       Test GitHub token authentication")
	fmt.Println("  gitsyncer --version                 Show version information")
	fmt.Println("\nOptions:")
	fmt.Println("  --config <path>                     Path to configuration file")
	fmt.Println("  --work-dir <path>                   Working directory for operations (default: .gitsyncer-work)")
	fmt.Println("  --create-github-repos               Create missing GitHub repositories automatically")
	fmt.Println("  --create-codeberg-repos             Create missing Codeberg repositories (not yet implemented)")
	fmt.Println("  --dry-run                           Show what would be done without doing it")
	fmt.Println("\nGitHub Token:")
	fmt.Println("  Set via: config file, GITHUB_TOKEN env var, or ~/.gitsyncer_github_token file")
	
	// Exit with error code when no action was specified
	os.Exit(1)
}