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
	abandonedReports map[string]*AbandonedBranchReport // Collects reports across repos
	branchFilter     *BranchFilter                     // Filter for excluding branches
	backupEnabled    bool                              // Whether to sync to backup locations
}

// CLAUDE: Is there a reason, we return a pointer to Syncer?
// New creates a new Syncer instance
func New(cfg *config.Config, workDir string) *Syncer {
	// Create branch filter
	branchFilter, err := NewBranchFilter(cfg.ExcludeBranches)
	if err != nil {
		// Log error but continue without filter
		fmt.Printf("Warning: Failed to create branch filter: %v\n", err)
		branchFilter = &BranchFilter{}
	}

	return &Syncer{
		config:  cfg,
		workDir: workDir,
		abandonedReports: make(map[string]*AbandonedBranchReport),
		branchFilter:     branchFilter,
		backupEnabled:    false, // Default to false, will be set via SetBackupEnabled
	}
}

// SetBackupEnabled enables or disables syncing to backup locations
func (s *Syncer) SetBackupEnabled(enabled bool) {
	s.backupEnabled = enabled
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
	allBranches, err := s.getAllBranches()
	if err != nil {
		return fmt.Errorf("failed to get branches: %w", err)
	}

	// Filter branches based on exclusion patterns
	branches := s.branchFilter.FilterBranches(allBranches)
	excludedBranches := s.branchFilter.GetExcludedBranches(allBranches)
	
	// Report excluded branches if any
	if exclusionReport := FormatExclusionReport(excludedBranches, s.config.ExcludeBranches); exclusionReport != "" {
		fmt.Print(exclusionReport)
	}

	// Get remotes map
	remotes := s.getRemotesMap()

	// Sync all branches
	if err := s.syncAllBranches(branches, remotes); err != nil {
		return err
	}

	// Analyze abandoned branches
	report, err := s.analyzeAbandonedBranches()
	if err != nil {
		// Don't fail sync, just log the error
		fmt.Printf("Warning: Failed to analyze abandoned branches: %v\n", err)
	} else {
		// Store the report for summary
		s.abandonedReports[repoName] = report
		// Print individual report if not empty
		if reportStr := formatAbandonedBranchReport(report, repoName); reportStr != "" {
			fmt.Print(reportStr)
		}
	}

	fmt.Printf("\nRepository %s synchronized successfully!\n", repoName)
	return nil
}

// EnsureRepositoryCloned ensures a repository is cloned locally without syncing
// This is used for showcase-only mode
func (s *Syncer) EnsureRepositoryCloned(repoName string) error {
	s.repoName = repoName
	
	// Create work directory if it doesn't exist
	if err := os.MkdirAll(s.workDir, 0755); err != nil {
		return fmt.Errorf("failed to create work directory: %w", err)
	}
	
	// Check if repository already exists
	repoPath := filepath.Join(s.workDir, repoName)
	if _, err := os.Stat(repoPath); err == nil {
		// Repository exists, nothing to do
		fmt.Printf("  Repository %s already exists locally\n", repoName)
		return nil
	}
	
	// Repository doesn't exist, clone it
	fmt.Printf("  Cloning %s...\n", repoName)
	
	// Find first non-backup organization to clone from
	var sourceOrg *config.Organization
	for i := range s.config.Organizations {
		if !s.config.Organizations[i].BackupLocation {
			sourceOrg = &s.config.Organizations[i]
			break
		}
	}
	
	if sourceOrg == nil {
		return fmt.Errorf("no non-backup organizations configured to clone from")
	}
	
	// Clone the repository
	if err := s.cloneRepository(sourceOrg, repoPath); err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}
	
	fmt.Printf("  Successfully cloned %s\n", repoName)
	return nil
}

// cloneRepository clones a repository from an organization
func (s *Syncer) cloneRepository(org *config.Organization, repoPath string) error {
	// Skip cloning from backup locations
	if org.BackupLocation {
		return fmt.Errorf("cannot clone from backup location %s", org.Host)
	}

	// For file:// URLs, we need special handling
	var cloneURL string
	if strings.HasPrefix(org.Host, "file://") {
		// For local file paths, the format is: file:///path/to/repo.git
		cloneURL = fmt.Sprintf("%s/%s.git", org.Host, s.repoName)
	} else if org.IsSSH() && org.Name == "" {
		// For SSH backup locations: user@host:path/repo.git
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
	} else if org.IsSSH() && org.Name == "" {
		// For SSH backup locations: user@host:path/repo.git
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

	// Check all organizations to identify backup locations
	// We need to check ALL orgs, not just active ones
	allOrgsMap := make(map[string]*config.Organization)
	for i := range s.config.Organizations {
		org := &s.config.Organizations[i]
		remoteName := s.getRemoteName(org)
		allOrgsMap[remoteName] = org
	}

	// Fetch from each remote
	for remote := range remotes {
		// Check if this remote is a backup location
		if org, exists := allOrgsMap[remote]; exists && org.BackupLocation {
			if !s.backupEnabled {
				// Silently skip - don't even print a message since backup is not enabled
				continue
			}
			// Even when backup is enabled, we don't fetch from backup locations
			fmt.Printf("Skipping fetch from backup location %s\n", remote)
			continue
		}

		fmt.Printf("Fetching %s\n", remote)
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

	// If backup is disabled, filter out branches from backup locations
	if !s.backupEnabled {
		filteredOutput := s.filterBackupBranches(output)
		return getAllUniqueBranches(filteredOutput), nil
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

// filterBackupBranches filters out branches from backup locations
func (s *Syncer) filterBackupBranches(output []byte) []byte {
	lines := strings.Split(string(output), "\n")
	var filtered []string
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		// Check if this branch is from a backup remote
		isBackup := false
		for i := range s.config.Organizations {
			org := &s.config.Organizations[i]
			if org.BackupLocation {
				remoteName := s.getRemoteName(org)
				if strings.HasPrefix(line, remoteName+"/") {
					isBackup = true
					break
				}
			}
		}
		
		if !isBackup {
			filtered = append(filtered, line)
		}
	}
	
	return []byte(strings.Join(filtered, "\n"))
}
