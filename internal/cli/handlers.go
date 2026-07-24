package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"codeberg.org/snonux/gitsyncer/internal/config"
	"codeberg.org/snonux/gitsyncer/internal/github"
)

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
		if org.IsCodeberg() && !cfg.CodebergSyncEnabled() {
			fmt.Printf("Skipping Codeberg org (sync_codeberg disabled): %s\n", org.Host)
			continue
		}
		client, supported := newRepoClientForOrg(org)
		if !supported {
			fmt.Printf("Skipping unsupported host: %s\n", org.Host)
			continue
		}
		exists, err := client.RepoExists(repoName)

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

		client, _ := newRepoClientForOrg(info.org)
		deleteErr := client.DeleteRepo(repoName)

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
