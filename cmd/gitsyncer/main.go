package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/paul/gitsyncer/internal/config"
	"github.com/paul/gitsyncer/internal/sync"
	"github.com/paul/gitsyncer/internal/version"
)

func main() {
	var (
		versionFlag bool
		configPath  string
		listOrgs    bool
		listRepos   bool
		syncRepo    string
		syncAll     bool
		workDir     string
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
	flag.StringVar(&workDir, "work-dir", ".gitsyncer-work", "working directory for cloning repositories")
	flag.Parse()

	// Handle version flag
	if versionFlag {
		fmt.Println(version.GetVersion())
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

		syncer := sync.New(cfg, workDir)
		failedRepos := []string{}
		
		for i, repo := range cfg.Repositories {
			fmt.Printf("\n[%d/%d] Syncing %s...\n", i+1, len(cfg.Repositories), repo)
			if err := syncer.SyncRepository(repo); err != nil {
				fmt.Printf("Failed to sync %s: %v\n", repo, err)
				failedRepos = append(failedRepos, repo)
			}
		}

		if len(failedRepos) > 0 {
			fmt.Printf("\nFailed to sync %d repository(ies):\n", len(failedRepos))
			for _, repo := range failedRepos {
				fmt.Printf("  - %s\n", repo)
			}
			os.Exit(1)
		}
		
		fmt.Printf("\nSuccessfully synced all %d repositories!\n", len(cfg.Repositories))
		os.Exit(0)
	}

	// Default: show usage
	fmt.Println("\ngitsyncer - Git repository synchronization tool")
	fmt.Printf("Configured with %d organization(s) and %d repository(ies)\n", 
		len(cfg.Organizations), len(cfg.Repositories))
	fmt.Println("\nUsage:")
	fmt.Println("  gitsyncer --sync <repo-name>     Sync a specific repository")
	fmt.Println("  gitsyncer --sync-all             Sync all configured repositories")
	fmt.Println("  gitsyncer --list-orgs            List configured organizations")
	fmt.Println("  gitsyncer --list-repos           List configured repositories")
	fmt.Println("  gitsyncer --version              Show version information")
	fmt.Println("\nOptions:")
	fmt.Println("  --config <path>                  Path to configuration file")
	fmt.Println("  --work-dir <path>                Working directory for operations (default: .gitsyncer-work)")
}