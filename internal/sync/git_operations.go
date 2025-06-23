package sync

import (
	"fmt"
	"os/exec"
	"strings"
)

// checkForMergeConflicts checks if the repository has merge conflicts
func checkForMergeConflicts() (bool, string, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return false, "", err
	}
	
	statusStr := string(output)
	hasConflicts := strings.Contains(statusStr, "UU ") || 
	                strings.Contains(statusStr, "AA ") || 
	                strings.Contains(statusStr, "DD ")
	
	return hasConflicts, statusStr, nil
}

// stashChanges stashes uncommitted changes
func stashChanges() error {
	fmt.Println("  Stashing uncommitted changes...")
	return exec.Command("git", "stash", "push", "-m", "gitsyncer-auto-stash").Run()
}

// popStash attempts to pop the stash (used in defer)
func popStash() {
	exec.Command("git", "stash", "pop").Run()
}

// mergeBranch merges a branch from a remote
func mergeBranch(remoteName, branch string) error {
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
	
	return nil
}

// pushBranch pushes a branch to a remote
func pushBranch(remoteName, branch string, remoteHasBranch bool) error {
	cmd := exec.Command("git", "push", remoteName, branch)
	output, err := cmd.CombinedOutput()
	
	if err != nil {
		outputStr := string(output)
		// Check if it's because the repository doesn't exist
		if isRepositoryMissing(outputStr) {
			fmt.Printf("    Note: Remote repository %s does not exist - must be created manually\n", remoteName)
			fmt.Printf("    Skipping push to %s\n", remoteName)
			return nil // Not an error, just skip
		}
		
		// Check if it's because the branch doesn't exist on the remote
		if isBranchMissing(outputStr) {
			fmt.Printf("    Creating new branch on %s\n", remoteName)
			// Try again with -u flag to set upstream
			cmd = exec.Command("git", "push", "-u", remoteName, branch)
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to push to %s: %w", remoteName, err)
			}
			return nil
		}
		
		return fmt.Errorf("failed to push to %s: %w\n%s", remoteName, err, outputStr)
	}
	
	if !remoteHasBranch {
		fmt.Printf("    Successfully created branch %s on %s\n", branch, remoteName)
	}
	
	return nil
}

// isRepositoryMissing checks if the error indicates a missing repository
func isRepositoryMissing(output string) bool {
	return strings.Contains(output, "does not appear to be a git repository") ||
	       strings.Contains(output, "Could not read from remote repository")
}

// isBranchMissing checks if the error indicates a missing branch
func isBranchMissing(output string) bool {
	return strings.Contains(output, "error: src refspec")
}

// getRemotesList extracts unique remote names from git remote -v output
func getRemotesList() (map[string]bool, error) {
	cmd := exec.Command("git", "remote", "-v")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list remotes: %w", err)
	}

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
	
	return remotes, nil
}

// fetchRemote fetches from a single remote with error handling
func fetchRemote(remote string) error {
	fmt.Printf("Fetching %s\n", remote)
	cmd := exec.Command("git", "fetch", remote, "--prune")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if it's because the repository doesn't exist
		if isRepositoryMissing(string(output)) {
			fmt.Printf("  Warning: Remote repository %s does not exist yet\n", remote)
			return nil // Not an error, just skip
		}
		return fmt.Errorf("failed to fetch from %s: %w\n%s", remote, err, string(output))
	}
	return nil
}

// checkoutExistingBranch tries to checkout an existing branch
func checkoutExistingBranch(branch string) error {
	cmd := exec.Command("git", "checkout", branch)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("  Initial checkout failed: %s\n", strings.TrimSpace(string(output)))
		return err
	}
	return nil
}

// createTrackingBranch creates a new branch tracking a remote branch
func createTrackingBranch(branch, remoteName string) error {
	cmd := exec.Command("git", "checkout", "-b", branch, fmt.Sprintf("%s/%s", remoteName, branch))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create tracking branch: %s", string(output))
	}
	return nil
}

// getAllUniqueBranches extracts unique branch names from git branch -r output
func getAllUniqueBranches(output []byte) []string {
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

	return branches
}