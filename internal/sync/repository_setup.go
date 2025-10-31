package sync

import (
	"fmt"
	"os"
	"os/exec"

	"codeberg.org/snonux/gitsyncer/internal/config"
)

// setupRepository ensures the repository exists and all remotes are configured
func (s *Syncer) setupRepository(repoPath string) error {
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		return s.setupNewRepository(repoPath)
	}
	return s.setupExistingRepository(repoPath)
}

// setupNewRepository clones and configures a new repository
func (s *Syncer) setupNewRepository(repoPath string) error {
	if len(s.config.Organizations) == 0 {
		return fmt.Errorf("no organizations configured")
	}

	// Find first non-backup organization to clone from
	var firstOrg *config.Organization
	var firstOrgIndex int
	for i := range s.config.Organizations {
		if !s.config.Organizations[i].BackupLocation {
			firstOrg = &s.config.Organizations[i]
			firstOrgIndex = i
			break
		}
	}

	if firstOrg == nil {
		return fmt.Errorf("no non-backup organizations configured to clone from")
	}

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
	for i := range s.config.Organizations {
		if i == firstOrgIndex {
			continue // Skip the first org we already cloned from
		}
		org := &s.config.Organizations[i]

		// Skip backup locations if backup is not enabled
		if org.BackupLocation && !s.backupEnabled {
			continue
		}

		if err := s.addRemote(repoPath, org); err != nil {
			return fmt.Errorf("failed to add remote %s: %w", s.getRemoteName(org), err)
		}
	}

	return nil
}

// setupExistingRepository ensures all remotes are configured for an existing repository
func (s *Syncer) setupExistingRepository(repoPath string) error {
	fmt.Printf("Using existing repository at %s\n", repoPath)

	// Check and add any missing remotes
	for i := range s.config.Organizations {
		org := &s.config.Organizations[i]

		// Skip backup locations if backup is not enabled
		if org.BackupLocation && !s.backupEnabled {
			continue
		}

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

	return nil
}

// changeToRepoDirectory changes to the repository directory and returns a function to restore the original directory
func changeToRepoDirectory(repoPath string) (func(), error) {
	originalDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get current directory: %w", err)
	}

	if err := os.Chdir(repoPath); err != nil {
		return nil, fmt.Errorf("failed to change to repository directory: %w", err)
	}

	return func() { os.Chdir(originalDir) }, nil
}

// getRemotesMap creates a map of remote names to organizations
func (s *Syncer) getRemotesMap() map[string]*config.Organization {
	remotes := make(map[string]*config.Organization)
	for i := range s.config.Organizations {
		org := &s.config.Organizations[i]

		// Skip backup locations if backup is not enabled
		if org.BackupLocation && !s.backupEnabled {
			continue
		}

		remoteName := s.getRemoteName(org)
		remotes[remoteName] = org
	}
	return remotes
}
