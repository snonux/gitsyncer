package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codeberg.org/snonux/gitsyncer/internal/codeberg"
	"codeberg.org/snonux/gitsyncer/internal/config"
	"codeberg.org/snonux/gitsyncer/internal/github"
	"codeberg.org/snonux/gitsyncer/internal/version"
)

// HandleVersion prints version information
func HandleVersion() int {
	fmt.Println(version.GetVersion())
	return 0
}

// HandleTestGitHubToken tests GitHub token authentication
func HandleTestGitHubToken() int {
	fmt.Println("Testing GitHub token authentication...")
	client := github.NewClient("", "snonux") // Empty token to trigger loading from env/file
	if !client.HasToken() {
		fmt.Println("ERROR: No GitHub token found!")
		fmt.Println("Please set GITHUB_TOKEN environment variable or create ~/.gitsyncer_github_token file")
		return 1
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
		return 1
	}
	
	fmt.Printf("SUCCESS: Token is valid! Repository check returned: %v\n", exists)
	return 0
}

// LoadConfig loads configuration from the specified path or default locations
func LoadConfig(configPath string) (*config.Config, error) {
	if configPath == "" {
		configPath = findDefaultConfigPath()
		if configPath == "" {
			return nil, fmt.Errorf("no configuration file found")
		}
	}
	
	fmt.Printf("Loaded configuration from: %s\n", configPath)
	return config.Load(configPath)
}

// findDefaultConfigPath searches for config file in default locations
func findDefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// Check common config locations
	configLocations := []string{
		filepath.Join(".", "gitsyncer.json"),
		filepath.Join(home, ".config", "gitsyncer", "config.json"),
		filepath.Join(home, ".gitsyncer.json"),
	}

	for _, loc := range configLocations {
		if _, err := os.Stat(loc); err == nil {
			return loc
		}
	}
	
	return ""
}

// ShowConfigHelp displays help for creating a configuration file
func ShowConfigHelp() {
	home, _ := os.UserHomeDir()
	
	fmt.Println("No configuration file found. Please create one of:")
	fmt.Printf("  - ./gitsyncer.json\n")
	fmt.Printf("  - %s/.config/gitsyncer/config.json\n", home)
	fmt.Printf("  - %s/.gitsyncer.json\n", home)
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
  ],
  "exclude_branches": [
    "^codex/",
    "^temp-",
    "-wip$"
  ],
  "work_dir": "~/git/gitsyncer-workdir"
}`)
}

// HandleListOrgs lists configured organizations
func HandleListOrgs(cfg *config.Config) int {
	fmt.Println("\nConfigured organizations:")
	for _, org := range cfg.Organizations {
		fmt.Printf("  - %s\n", org.GetGitURL())
	}
	return 0
}

// HandleListRepos lists configured repositories
func HandleListRepos(cfg *config.Config) int {
	fmt.Println("\nConfigured repositories:")
	if len(cfg.Repositories) == 0 {
		fmt.Println("  (none configured)")
	} else {
		for _, repo := range cfg.Repositories {
			fmt.Printf("  - %s\n", repo)
		}
	}
	return 0
}

// ShowUsage displays the usage information
func ShowUsage(cfg *config.Config) {
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
	fmt.Println("  gitsyncer --delete-repo <name>      Delete repository from all organizations")
	fmt.Println("  gitsyncer --version                 Show version information")
	fmt.Println("\nOptions:")
	fmt.Println("  --config <path>                     Path to configuration file")
	fmt.Println("  --work-dir <path>                   Working directory for operations (default: ~/git/gitsyncer-workdir)")
	fmt.Println("  --create-github-repos               Create missing GitHub repositories automatically")
	fmt.Println("  --create-codeberg-repos             Create missing Codeberg repositories (not yet implemented)")
	fmt.Println("  --dry-run                           Show what would be done without doing it")
	fmt.Println("\nGitHub Token:")
	fmt.Println("  Set via: config file, GITHUB_TOKEN env var, or ~/.gitsyncer_github_token file")
}

// HandleDeleteRepo handles the --delete-repo flag
func HandleDeleteRepo(cfg *config.Config, repoName string) int {
	if repoName == "" {
		fmt.Println("Error: Repository name is required for --delete-repo")
		return 1
	}

	fmt.Printf("\n⚠️  WARNING: This will permanently delete the repository '%s' from all configured organizations!\n\n", repoName)
	
	// Find organizations where the repo exists
	var orgsWithRepo []struct {
		org    config.Organization
		exists bool
		err    error
	}
	
	for _, org := range cfg.Organizations {
		var exists bool
		var err error
		
		switch org.Host {
		case "git@github.com":
			client := github.NewClient(org.GitHubToken, org.Name)
			exists, err = client.RepoExists(repoName)
		case "git@codeberg.org":
			client := codeberg.NewClient(org.Name, org.CodebergToken)
			exists, err = client.RepoExists(repoName)
		default:
			fmt.Printf("Skipping unsupported host: %s\n", org.Host)
			continue
		}
		
		orgsWithRepo = append(orgsWithRepo, struct {
			org    config.Organization
			exists bool
			err    error
		}{org, exists, err})
	}
	
	// Show summary of where the repo exists
	fmt.Println("Repository status:")
	foundAny := false
	for _, info := range orgsWithRepo {
		if info.err != nil {
			fmt.Printf("  ❌ %s: Error checking - %v\n", info.org.GetGitURL(), info.err)
		} else if info.exists {
			fmt.Printf("  ✅ %s: EXISTS - will be DELETED\n", info.org.GetGitURL())
			foundAny = true
		} else {
			fmt.Printf("  ⬜ %s: Not found\n", info.org.GetGitURL())
		}
	}
	
	if !foundAny {
		fmt.Printf("\nRepository '%s' not found in any configured organization.\n", repoName)
		return 0
	}
	
	// Confirm deletion
	fmt.Printf("\nAre you sure you want to delete '%s' from the above organizations? This action cannot be undone!\n", repoName)
	fmt.Print("Type 'yes' to confirm: ")
	
	reader := bufio.NewReader(os.Stdin)
	confirmation, _ := reader.ReadString('\n')
	confirmation = strings.TrimSpace(confirmation)
	
	if confirmation != "yes" {
		fmt.Println("Deletion cancelled.")
		return 0
	}
	
	// Perform deletions
	fmt.Println("\nDeleting repositories...")
	hasError := false
	
	for _, info := range orgsWithRepo {
		if !info.exists || info.err != nil {
			continue
		}
		
		fmt.Printf("  Deleting from %s... ", info.org.GetGitURL())
		
		var deleteErr error
		switch info.org.Host {
		case "git@github.com":
			client := github.NewClient(info.org.GitHubToken, info.org.Name)
			deleteErr = client.DeleteRepo(repoName)
		case "git@codeberg.org":
			client := codeberg.NewClient(info.org.Name, info.org.CodebergToken)
			deleteErr = client.DeleteRepo(repoName)
		}
		
		if deleteErr != nil {
			fmt.Printf("FAILED: %v\n", deleteErr)
			hasError = true
		} else {
			fmt.Println("SUCCESS")
		}
	}
	
	if hasError {
		fmt.Println("\n⚠️  Some deletions failed. Check the errors above.")
		return 1
	}
	
	fmt.Printf("\n✅ Repository '%s' has been successfully deleted from all organizations.\n", repoName)
	return 0
}