package sync

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/paul/gitsyncer/internal/config"
)

// Syncer handles repository synchronization between organizations
type Syncer struct {
	config   *config.Config
	workDir  string
	repoName string
}

// New creates a new Syncer instance
func New(cfg *config.Config, workDir string) *Syncer {
	return &Syncer{
		config:  cfg,
		workDir: workDir,
	}
}

// SyncRepository synchronizes a repository across all configured organizations
func (s *Syncer) SyncRepository(repoName string) error {
	s.repoName = repoName
	
	// Create work directory if it doesn't exist
	if err := os.MkdirAll(s.workDir, 0755); err != nil {
		return fmt.Errorf("failed to create work directory: %w", err)
	}

	// Get all remotes
	remotes := make(map[string]*config.Organization)
	for i := range s.config.Organizations {
		org := &s.config.Organizations[i]
		remoteName := s.getRemoteName(org)
		remotes[remoteName] = org
	}

	// Clone or update the repository
	repoPath := filepath.Join(s.workDir, repoName)
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		// Clone from the first organization
		if len(s.config.Organizations) == 0 {
			return fmt.Errorf("no organizations configured")
		}
		
		firstOrg := &s.config.Organizations[0]
		if err := s.cloneRepository(firstOrg, repoPath); err != nil {
			return fmt.Errorf("failed to clone repository: %w", err)
		}
		
		// Rename origin to the proper remote name
		firstRemoteName := s.getRemoteName(firstOrg)
		cmd := exec.Command("git", "-C", repoPath, "remote", "rename", "origin", firstRemoteName)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to rename origin remote: %w", err)
		}
		
		// Add other organizations as remotes
		for i := 1; i < len(s.config.Organizations); i++ {
			org := &s.config.Organizations[i]
			if err := s.addRemote(repoPath, org); err != nil {
				return fmt.Errorf("failed to add remote %s: %w", s.getRemoteName(org), err)
			}
		}
	}

	// Change to repository directory
	originalDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}
	defer os.Chdir(originalDir)
	
	if err := os.Chdir(repoPath); err != nil {
		return fmt.Errorf("failed to change to repository directory: %w", err)
	}

	// Fetch all remotes
	fmt.Printf("Fetching updates from all remotes...\n")
	if err := s.fetchAll(); err != nil {
		return fmt.Errorf("failed to fetch remotes: %w", err)
	}

	// Get all branches
	branches, err := s.getAllBranches()
	if err != nil {
		return fmt.Errorf("failed to get branches: %w", err)
	}

	// Sync each branch
	for _, branch := range branches {
		fmt.Printf("\nSyncing branch: %s\n", branch)
		if err := s.syncBranch(branch, remotes); err != nil {
			return fmt.Errorf("failed to sync branch %s: %w", branch, err)
		}
	}

	fmt.Printf("\nRepository %s synchronized successfully!\n", repoName)
	return nil
}

// cloneRepository clones a repository from an organization
func (s *Syncer) cloneRepository(org *config.Organization, repoPath string) error {
	// For file:// URLs, we need special handling
	var cloneURL string
	if strings.HasPrefix(org.Host, "file://") {
		// For local file paths, the format is: file:///path/to/repo.git
		cloneURL = fmt.Sprintf("%s/%s.git", org.Host, s.repoName)
	} else {
		// For SSH URLs, the format is: git@host:org/repo.git
		cloneURL = fmt.Sprintf("%s/%s.git", org.GetGitURL(), s.repoName)
	}
	
	fmt.Printf("Cloning from %s...\n", cloneURL)
	
	cmd := exec.Command("git", "clone", cloneURL, repoPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	if err := cmd.Run(); err != nil {
		return err
	}
	
	return nil
}

// addRemote adds a remote to the repository
func (s *Syncer) addRemote(repoPath string, org *config.Organization) error {
	remoteName := s.getRemoteName(org)
	
	// For file:// URLs, we need special handling
	var remoteURL string
	if strings.HasPrefix(org.Host, "file://") {
		remoteURL = fmt.Sprintf("%s/%s.git", org.Host, s.repoName)
	} else {
		remoteURL = fmt.Sprintf("%s/%s.git", org.GetGitURL(), s.repoName)
	}
	
	fmt.Printf("Adding remote %s: %s\n", remoteName, remoteURL)
	
	cmd := exec.Command("git", "-C", repoPath, "remote", "add", remoteName, remoteURL)
	if err := cmd.Run(); err != nil {
		return err
	}
	
	return nil
}

// fetchAll fetches from all remotes
func (s *Syncer) fetchAll() error {
	cmd := exec.Command("git", "fetch", "--all", "--prune")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// getAllBranches gets all unique branches from all remotes
func (s *Syncer) getAllBranches() ([]string, error) {
	cmd := exec.Command("git", "branch", "-r")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	
	branchMap := make(map[string]bool)
	lines := strings.Split(string(output), "\n")
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "->") {
			continue
		}
		
		// Extract branch name from remote/branch format
		parts := strings.SplitN(line, "/", 2)
		if len(parts) == 2 {
			branch := parts[1]
			branchMap[branch] = true
		}
	}
	
	// Convert map to slice
	branches := make([]string, 0, len(branchMap))
	for branch := range branchMap {
		branches = append(branches, branch)
	}
	
	return branches, nil
}

// syncBranch synchronizes a specific branch across all remotes
func (s *Syncer) syncBranch(branch string, remotes map[string]*config.Organization) error {
	// Create or checkout the branch
	if err := s.checkoutBranch(branch); err != nil {
		return fmt.Errorf("failed to checkout branch %s: %w", branch, err)
	}

	// Track which remotes have this branch
	remotesWithBranch := make(map[string]bool)
	
	// Check which remotes have this branch
	for remoteName := range remotes {
		if s.remoteBranchExists(remoteName, branch) {
			remotesWithBranch[remoteName] = true
		}
	}

	// If no remotes have this branch, skip it
	if len(remotesWithBranch) == 0 {
		fmt.Printf("  Branch %s not found on any remote, skipping\n", branch)
		return nil
	}

	// Merge changes from all remotes that have this branch
	for remoteName := range remotesWithBranch {
		fmt.Printf("  Merging from %s/%s...\n", remoteName, branch)
		
		cmd := exec.Command("git", "merge", fmt.Sprintf("%s/%s", remoteName, branch), "--no-edit")
		output, err := cmd.CombinedOutput()
		
		if err != nil {
			// Check if it's a merge conflict
			if strings.Contains(string(output), "CONFLICT") {
				return fmt.Errorf("merge conflict detected when merging %s/%s. Please resolve manually", remoteName, branch)
			}
			return fmt.Errorf("failed to merge %s/%s: %w\n%s", remoteName, branch, err, string(output))
		}
	}

	// Push to all remotes
	for remoteName, org := range remotes {
		fmt.Printf("  Pushing to %s (%s)...\n", remoteName, org.Host)
		
		cmd := exec.Command("git", "push", remoteName, branch)
		output, err := cmd.CombinedOutput()
		
		if err != nil {
			// Check if it's because the branch doesn't exist on the remote
			if strings.Contains(string(output), "error: src refspec") {
				fmt.Printf("    Creating new branch on %s\n", remoteName)
				// Try again with -u flag to set upstream
				cmd = exec.Command("git", "push", "-u", remoteName, branch)
				if err := cmd.Run(); err != nil {
					return fmt.Errorf("failed to push to %s: %w", remoteName, err)
				}
			} else {
				return fmt.Errorf("failed to push to %s: %w\n%s", remoteName, err, string(output))
			}
		}
	}

	return nil
}

// checkoutBranch checks out a branch, creating it if necessary
func (s *Syncer) checkoutBranch(branch string) error {
	// First try to checkout existing branch
	cmd := exec.Command("git", "checkout", branch)
	if err := cmd.Run(); err == nil {
		return nil
	}
	
	// If that fails, create a new branch tracking the first remote that has it
	for i := range s.config.Organizations {
		org := &s.config.Organizations[i]
		remoteName := s.getRemoteName(org)
		
		if s.remoteBranchExists(remoteName, branch) {
			cmd = exec.Command("git", "checkout", "-b", branch, fmt.Sprintf("%s/%s", remoteName, branch))
			return cmd.Run()
		}
	}
	
	return fmt.Errorf("branch %s not found on any remote", branch)
}

// remoteBranchExists checks if a branch exists on a remote
func (s *Syncer) remoteBranchExists(remoteName, branch string) bool {
	cmd := exec.Command("git", "branch", "-r", "--list", fmt.Sprintf("%s/%s", remoteName, branch))
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) != ""
}

// getRemoteName generates a remote name for an organization
func (s *Syncer) getRemoteName(org *config.Organization) string {
	// Use the host without git@ or file:// prefix as remote name
	host := org.Host
	host = strings.TrimPrefix(host, "git@")
	host = strings.TrimPrefix(host, "file://")
	host = strings.ReplaceAll(host, ":", "_")
	host = strings.ReplaceAll(host, ".", "_")
	host = strings.ReplaceAll(host, "/", "_")
	
	// For file URLs, create a simpler name
	if strings.HasPrefix(org.Host, "file://") {
		// Get the last part of the path
		parts := strings.Split(strings.TrimPrefix(org.Host, "file://"), "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}
	
	return host
}