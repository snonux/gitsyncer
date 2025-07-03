package sync

import (
	"fmt"

	"codeberg.org/snonux/gitsyncer/internal/config"
)

// trackRemotesWithBranch finds which remotes have a specific branch
func (s *Syncer) trackRemotesWithBranch(branch string, remotes map[string]*config.Organization) map[string]bool {
	remotesWithBranch := make(map[string]bool)
	
	for remoteName, org := range remotes {
		// Skip checking backup locations as we don't sync from them
		if org.BackupLocation {
			continue
		}
		if s.remoteBranchExists(remoteName, branch) {
			remotesWithBranch[remoteName] = true
		}
	}
	
	return remotesWithBranch
}

// mergeFromRemotes merges changes from all remotes that have the branch
func mergeFromRemotes(branch string, remotesWithBranch map[string]bool) error {
	if len(remotesWithBranch) == 0 {
		fmt.Printf("  Branch %s is local only, will push to all remotes\n", branch)
		return nil
	}
	
	// Merge changes from all remotes that have this branch
	for remoteName := range remotesWithBranch {
		if err := mergeBranch(remoteName, branch); err != nil {
			return err
		}
	}
	
	return nil
}

// pushToAllRemotes pushes the branch to all configured remotes
func pushToAllRemotes(branch string, remotes map[string]*config.Organization, remotesWithBranch map[string]bool) error {
	for remoteName, org := range remotes {
		// Check if this remote has the branch
		remoteHasBranch := remotesWithBranch[remoteName]

		if !remoteHasBranch {
			fmt.Printf("  Creating branch on %s (%s)...\n", remoteName, org.Host)
		} else {
			fmt.Printf("  Pushing to %s (%s)...\n", remoteName, org.Host)
		}

		if err := pushBranchWithBackupSupport(remoteName, branch, remoteHasBranch, org); err != nil {
			return err
		}
	}
	
	return nil
}

// syncAllBranches synchronizes all branches across remotes
func (s *Syncer) syncAllBranches(branches []string, remotes map[string]*config.Organization) error {
	for _, branch := range branches {
		fmt.Printf("\nSyncing branch: %s\n", branch)
		if err := s.syncBranch(branch, remotes); err != nil {
			return fmt.Errorf("failed to sync branch %s: %w", branch, err)
		}
	}
	return nil
}