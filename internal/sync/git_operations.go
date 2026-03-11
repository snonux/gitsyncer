package sync

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"codeberg.org/snonux/gitsyncer/internal/config"
)

func gitCommand(repoPath string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	if repoPath != "" {
		cmd.Dir = repoPath
	}
	return cmd
}

// checkForMergeConflicts checks if the repository has merge conflicts
func checkForMergeConflicts(repoPath string) (bool, string, error) {
	cmd := gitCommand(repoPath, "status", "--porcelain")
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
func stashChanges(repoPath string) error {
	fmt.Println("  Stashing uncommitted changes...")
	return gitCommand(repoPath, "stash", "push", "-m", "gitsyncer-auto-stash").Run()
}

// popStash attempts to pop the stash (used in defer)
func popStash(repoPath string) {
	gitCommand(repoPath, "stash", "pop").Run()
}

// mergeBranch merges a branch from a remote
func mergeBranch(repoPath, remoteName, branch string) error {
	fmt.Printf("  Merging from %s/%s...\n", remoteName, branch)

	cmd := gitCommand(repoPath, "merge", fmt.Sprintf("%s/%s", remoteName, branch), "--no-edit")
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
func pushBranch(repoPath, remoteName, branch string, remoteHasBranch bool) error {
	cmd := gitCommand(repoPath, "push", remoteName, branch, "--tags")
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
			cmd = gitCommand(repoPath, "push", "-u", remoteName, branch, "--tags")
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
func getRemotesList(repoPath string) (map[string]bool, error) {
	cmd := gitCommand(repoPath, "remote", "-v")
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
func fetchRemote(repoPath, remote string) error {
	cmd := gitCommand(repoPath, "fetch", remote, "--prune", "--tags")
	output, err := cmd.CombinedOutput()

	if err != nil {
		// Check if it's a tag conflict error
		if bytes.Contains(output, []byte("would clobber existing tag")) {
			return handleTagConflict(repoPath, remote, output)
		}

		// Check if it's because the repository doesn't exist
		if isRepositoryMissing(string(output)) {
			fmt.Printf("  Warning: Remote repository %s does not exist yet\n", remote)
			return nil // Not an error, just skip
		}
		return fmt.Errorf("failed to fetch from %s: %w\n%s", remote, err, string(output))
	}
	return nil
}

// handleTagConflict provides a detailed error message for tag conflicts.
func handleTagConflict(repoPath, remote string, output []byte) error {
	var conflictDetails strings.Builder
	conflictDetails.WriteString("tag conflict detected while fetching from remote: ")
	conflictDetails.WriteString(remote)

	// Regex to find tag names from error output
	re := regexp.MustCompile(`! \[rejected\]\s+([^\s]+)`)
	matches := re.FindAllSubmatch(output, -1)

	for _, match := range matches {
		if len(match) > 1 {
			tag := string(match[1])
			localHash, _ := getTagCommitHash(repoPath, tag, "local")
			remoteHash, _ := getTagCommitHash(repoPath, tag, remote)
			conflictDetails.WriteString(fmt.Sprintf("\n  - Tag: %s\n    Local:  %s\n    Remote: %s", tag, localHash, remoteHash))
		}
	}

	return errors.New(conflictDetails.String())
}

// getTagCommitHash retrieves the commit hash for a given tag, either locally or from a remote.
func getTagCommitHash(repoPath, tag, source string) (string, error) {
	var cmd *exec.Cmd
	if source == "local" {
		cmd = gitCommand(repoPath, "rev-parse", tag+"^{\\}")
	} else {
		cmd = gitCommand(repoPath, "ls-remote", "--tags", source, tag)
	}

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	hash := strings.Fields(string(output))[0]
	return hash, nil
}

// checkoutExistingBranch tries to checkout an existing branch
func checkoutExistingBranch(repoPath, branch string) error {
	cmd := gitCommand(repoPath, "checkout", branch)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("  Initial checkout failed: %s\n", strings.TrimSpace(string(output)))
		return err
	}
	return nil
}

// createTrackingBranch creates a new branch tracking a remote branch
func createTrackingBranch(repoPath, branch, remoteName string) error {
	cmd := gitCommand(repoPath, "checkout", "-b", branch, fmt.Sprintf("%s/%s", remoteName, branch))
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

// createSSHBareRepository creates a bare repository on an SSH server
func createSSHBareRepository(sshHost, repoPath string) error {
	// Extract user@host and path components
	parts := strings.Split(sshHost, ":")
	if len(parts) != 2 {
		return fmt.Errorf("invalid SSH host format: %s", sshHost)
	}

	userHost := parts[0]
	basePath := parts[1]

	// Full path to the repository
	fullRepoPath := fmt.Sprintf("%s/%s.git", basePath, repoPath)

	fmt.Printf("Creating bare repository at %s:%s\n", userHost, fullRepoPath)

	// Create the repository directory and initialize as bare
	commands := fmt.Sprintf("mkdir -p %s && cd %s && git init --bare", fullRepoPath, fullRepoPath)
	cmd := exec.Command("ssh", userHost, commands)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf("failed to create bare repository: %w\n%s", err, string(output))
	}

	fmt.Printf("Successfully created bare repository at %s:%s\n", userHost, fullRepoPath)
	return nil
}

// pushBranchWithBackupSupport pushes a branch to a remote, creating SSH repos if needed
func pushBranchWithBackupSupport(repoPath, remoteName, branch string, remoteHasBranch bool, org *config.Organization) error {
	cmd := gitCommand(repoPath, "push", remoteName, branch, "--tags")
	output, err := cmd.CombinedOutput()

	if err != nil {
		outputStr := string(output)
		// Check if it's because the repository doesn't exist
		if isRepositoryMissing(outputStr) {
			// If it's an SSH backup location, try to create the repository
			if org.BackupLocation && org.IsSSH() {
				// Get the repository name from the remote URL
				remoteURL, err := getRemoteURL(repoPath, remoteName)
				if err != nil {
					return fmt.Errorf("failed to get remote URL: %w", err)
				}

				// Extract repo name from URL
				repoName := extractRepoName(remoteURL)
				if repoName == "" {
					return fmt.Errorf("failed to extract repository name from URL: %s", remoteURL)
				}

				// Create the bare repository
				if err := createSSHBareRepository(org.Host, repoName); err != nil {
					return fmt.Errorf("failed to create SSH repository: %w", err)
				}

				// Try pushing again
				cmd = gitCommand(repoPath, "push", remoteName, branch, "--tags")
				if err := cmd.Run(); err != nil {
					return fmt.Errorf("failed to push after creating repository: %w", err)
				}
				fmt.Printf("    Successfully pushed to newly created backup repository\n")
				return nil
			}

			fmt.Printf("    Note: Remote repository %s does not exist - must be created manually\n", remoteName)
			fmt.Printf("    Skipping push to %s\n", remoteName)
			return nil // Not an error, just skip
		}

		// Check if it's because the branch doesn't exist on the remote
		if isBranchMissing(outputStr) {
			fmt.Printf("    Creating new branch on %s\n", remoteName)
			// Try again with -u flag to set upstream
			cmd = gitCommand(repoPath, "push", "-u", remoteName, branch, "--tags")
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

// getRemoteURL gets the URL for a given remote
func getRemoteURL(repoPath, remoteName string) (string, error) {
	cmd := gitCommand(repoPath, "remote", "get-url", remoteName)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// extractRepoName extracts the repository name from a git URL
func extractRepoName(url string) string {
	// Remove .git suffix if present
	url = strings.TrimSuffix(url, ".git")

	// Extract the last component of the path
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}
