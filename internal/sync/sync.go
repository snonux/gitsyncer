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

	// Setup repository (clone or ensure remotes are configured)
	repoPath := filepath.Join(s.workDir, repoName)
	if err := s.setupRepository(repoPath); err != nil {
		return err
	}

	// Change to repository directory
	restoreDir, err := changeToRepoDirectory(repoPath)
	if err != nil {
		return err
	}
	defer restoreDir()

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

	// Get remotes map
	remotes := s.getRemotesMap()

	// Sync all branches
	if err := s.syncAllBranches(branches, remotes); err != nil {
		return err
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
	// Get list of remotes
	remotes, err := getRemotesList()
	if err != nil {
		return err
	}

	// Fetch from each remote
	for remote := range remotes {
		if err := fetchRemote(remote); err != nil {
			return err
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

	return getAllUniqueBranches(output), nil
}

// syncBranch synchronizes a specific branch across all remotes
func (s *Syncer) syncBranch(branch string, remotes map[string]*config.Organization) error {
	// Handle merge conflicts and uncommitted changes
	stashed, err := s.handleWorkingDirectoryState()
	if err != nil {
		return err
	}
	if stashed {
		defer popStash()
	}
	
	// Create or checkout the branch
	if err := s.checkoutBranch(branch); err != nil {
		return fmt.Errorf("failed to checkout branch %s: %w", branch, err)
	}

	// Track which remotes have this branch
	remotesWithBranch := s.trackRemotesWithBranch(branch, remotes)

	// Merge changes from remotes
	if err := mergeFromRemotes(branch, remotesWithBranch); err != nil {
		return err
	}

	// Push to all remotes
	return pushToAllRemotes(branch, remotes, remotesWithBranch)
}

// handleWorkingDirectoryState checks for conflicts and stashes changes if needed
// Returns true if changes were stashed
func (s *Syncer) handleWorkingDirectoryState() (bool, error) {
	hasConflicts, statusStr, err := checkForMergeConflicts()
	if err != nil || statusStr == "" {
		return false, nil
	}
	
	if hasConflicts {
		// Get absolute path for clarity
		absPath, err := filepath.Abs(s.workDir)
		if err != nil {
			absPath = s.workDir
		}
		return false, fmt.Errorf("repository has unresolved merge conflicts\nPlease resolve conflicts in: %s\nOr delete the directory to start fresh: rm -rf %s", absPath, absPath)
	}
	
	// If we have uncommitted changes but no conflicts, try to stash them
	if err := stashChanges(); err != nil {
		return false, fmt.Errorf("failed to stash changes: %w", err)
	}
	return true, nil
}

// checkoutBranch checks out a branch, creating it if necessary
func (s *Syncer) checkoutBranch(branch string) error {
	// First try to checkout existing branch
	if err := checkoutExistingBranch(branch); err == nil {
		return nil
	}

	// If that fails, create a new branch tracking the first remote that has it
	for i := range s.config.Organizations {
		org := &s.config.Organizations[i]
		remoteName := s.getRemoteName(org)

		if s.remoteBranchExists(remoteName, branch) {
			return createTrackingBranch(branch, remoteName)
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
