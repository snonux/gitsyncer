package sync

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"codeberg.org/snonux/gitsyncer/internal/config"
)

// Syncer handles repository synchronization between organizations
type Syncer struct {
	config   *config.Config
	workDir  string
	repoName string
}

// CLAUDE: Is there a reason, we return a pointer to Syncer?
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
	} else {
		// Repository exists, ensure all remotes are configured
		fmt.Printf("Using existing repository at %s\n", repoPath)

		// Check and add any missing remotes
		for i := range s.config.Organizations {
			org := &s.config.Organizations[i]
			remoteName := s.getRemoteName(org)

			// Check if remote exists
			cmd := exec.Command("git", "-C", repoPath, "remote", "get-url", remoteName)
			if err := cmd.Run(); err != nil {
				// Remote doesn't exist, add it
				if err := s.addRemote(repoPath, org); err != nil {
					return fmt.Errorf("failed to add remote %s: %w", remoteName, err)
				}
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
// Note: We use individual fetches instead of --all to handle missing repositories gracefully
func (s *Syncer) fetchAll() error {
	// First, check which remotes actually exist
	cmd := exec.Command("git", "remote", "-v")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list remotes: %w", err)
	}

	// Try to fetch from each remote individually to handle missing repos
	remotes := make(map[string]bool)
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 1 {
			remotes[parts[0]] = true
		}
	}

	// Fetch from each remote
	for remote := range remotes {
		fmt.Printf("Fetching %s\n", remote)
		cmd := exec.Command("git", "fetch", remote, "--prune")
		output, err := cmd.CombinedOutput()
		if err != nil {
			// Check if it's because the repository doesn't exist
			if strings.Contains(string(output), "does not appear to be a git repository") ||
				strings.Contains(string(output), "Could not read from remote repository") {
				fmt.Printf("  Warning: Remote repository %s does not exist yet\n", remote)
				continue
			}
			return fmt.Errorf("failed to fetch from %s: %w\n%s", remote, err, string(output))
		}
	}

	return nil
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
	// First check if we have unresolved merge conflicts
	cmd := exec.Command("git", "status", "--porcelain")
	output, err := cmd.Output()
	if err == nil && len(output) > 0 {
		// Check for merge conflicts
		statusStr := string(output)
		if strings.Contains(statusStr, "UU ") || strings.Contains(statusStr, "AA ") || strings.Contains(statusStr, "DD ") {
			// Get absolute path for clarity
			absPath, err := filepath.Abs(s.workDir)
			if err != nil {
				absPath = s.workDir
			}
			return fmt.Errorf("repository has unresolved merge conflicts\nPlease resolve conflicts in: %s\nOr delete the directory to start fresh: rm -rf %s", absPath, absPath)
		}
		// If we have uncommitted changes but no conflicts, try to stash them
		fmt.Println("  Stashing uncommitted changes...")
		if err := exec.Command("git", "stash", "push", "-m", "gitsyncer-auto-stash").Run(); err != nil {
			return fmt.Errorf("failed to stash changes: %w", err)
		}
		defer func() {
			// Try to pop the stash at the end
			exec.Command("git", "stash", "pop").Run()
		}()
	}
	
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

	// If no remotes have this branch, it means it's a local branch that needs to be pushed
	if len(remotesWithBranch) == 0 {
		fmt.Printf("  Branch %s is local only, will push to all remotes\n", branch)
	} else {
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
	}

	// Push to all remotes
	for remoteName, org := range remotes {
		// Check if this remote has the branch
		remoteHasBranch := remotesWithBranch[remoteName]

		if !remoteHasBranch {
			fmt.Printf("  Creating branch on %s (%s)...\n", remoteName, org.Host)
		} else {
			fmt.Printf("  Pushing to %s (%s)...\n", remoteName, org.Host)
		}

		cmd := exec.Command("git", "push", remoteName, branch)
		output, err := cmd.CombinedOutput()

		if err != nil {
			outputStr := string(output)
			// Check if it's because the repository doesn't exist
			if strings.Contains(outputStr, "does not appear to be a git repository") ||
				strings.Contains(outputStr, "Could not read from remote repository") {
				fmt.Printf("    Note: Remote repository %s does not exist - must be created manually\n", remoteName)
				fmt.Printf("    Skipping push to %s\n", remoteName)
				continue
			}
			// Check if it's because the branch doesn't exist on the remote
			// This shouldn't happen with our logic, but keep it as a fallback
			if strings.Contains(outputStr, "error: src refspec") {
				fmt.Printf("    Creating new branch on %s\n", remoteName)
				// Try again with -u flag to set upstream
				cmd = exec.Command("git", "push", "-u", remoteName, branch)
				if err := cmd.Run(); err != nil {
					return fmt.Errorf("failed to push to %s: %w", remoteName, err)
				}
			} else {
				return fmt.Errorf("failed to push to %s: %w\n%s", remoteName, err, outputStr)
			}
		} else if !remoteHasBranch {
			fmt.Printf("    Successfully created branch %s on %s\n", branch, remoteName)
		}
	}

	return nil
}

// checkoutBranch checks out a branch, creating it if necessary
func (s *Syncer) checkoutBranch(branch string) error {
	// First try to checkout existing branch
	cmd := exec.Command("git", "checkout", branch)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	
	// If checkout failed, check the error
	outputStr := string(output)
	fmt.Printf("  Initial checkout failed: %s\n", strings.TrimSpace(outputStr))

	// If that fails, create a new branch tracking the first remote that has it
	for i := range s.config.Organizations {
		org := &s.config.Organizations[i]
		remoteName := s.getRemoteName(org)

		if s.remoteBranchExists(remoteName, branch) {
			cmd = exec.Command("git", "checkout", "-b", branch, fmt.Sprintf("%s/%s", remoteName, branch))
			output, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("failed to create tracking branch: %s", string(output))
			}
			return nil
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
