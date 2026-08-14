package sync

import (
	"fmt"
	"sort"

	"codeberg.org/snonux/gitsyncer/internal/config"
)

// sortedRemoteNames returns the keys of a remote-keyed map in sorted
// (alphabetical) order.
//
// Go intentionally randomizes map iteration order (see "100 Go Mistakes"
// mistake #33: relying on map ordering). mergeFromRemotes, pushToAllRemotes,
// and fetchAll each drive a sequence of git operations (merge/push/fetch)
// keyed by remote name; ranging over such a map directly would run those
// operations in a different order on every invocation, making merge
// conflicts and partial-failure behavior non-deterministic and hard to
// reproduce. Sorting the remote names first makes every run process remotes
// in the same, predictable order.
func sortedRemoteNames[V any](remotes map[string]V) []string {
	names := make([]string, 0, len(remotes))
	for name := range remotes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

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
func mergeFromRemotes(repoPath, branch string, remotesWithBranch map[string]bool) error {
	if len(remotesWithBranch) == 0 {
		fmt.Printf("  Branch %s is local only, will push to all remotes\n", branch)
		return nil
	}

	// Merge changes from all remotes that have this branch, in sorted order
	// so the merge sequence (and any resulting conflicts) is deterministic
	// across runs. See sortedRemoteNames.
	for _, remoteName := range sortedRemoteNames(remotesWithBranch) {
		if err := mergeBranch(repoPath, remoteName, branch); err != nil {
			return err
		}
	}

	return nil
}

// handlePushError decides whether a push error should stop sync or only disable backup for this session.
func (s *Syncer) handlePushError(remoteName string, org *config.Organization, err error) error {
	if err == nil {
		return nil
	}

	if org != nil && org.BackupLocation {
		s.disableBackupForSession(remoteName, err)
		return nil
	}

	return err
}

// pushToAllRemotes pushes the branch to all configured remotes
func (s *Syncer) pushToAllRemotes(repoPath, branch string, remotes map[string]*config.Organization, remotesWithBranch map[string]bool) error {
	// Sorted so push order is deterministic across runs. See sortedRemoteNames.
	for _, remoteName := range sortedRemoteNames(remotes) {
		org := remotes[remoteName]
		if org.BackupLocation && !s.backupActive(remoteName) {
			continue
		}

		// Check if this remote has the branch
		remoteHasBranch := remotesWithBranch[remoteName]

		if !remoteHasBranch {
			fmt.Printf("  Creating branch on %s (%s)...\n", remoteName, org.Host)
		} else {
			fmt.Printf("  Pushing to %s (%s)...\n", remoteName, org.Host)
		}
		if s.dryRun {
			fmt.Printf("    [DRY RUN] Skipping push to %s\n", remoteName)
			continue
		}

		if err := s.handlePushError(remoteName, org, pushBranchWithBackupSupport(repoPath, remoteName, branch, remoteHasBranch, org)); err != nil {
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
