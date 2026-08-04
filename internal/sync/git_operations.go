package sync

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"codeberg.org/snonux/gitsyncer/internal/config"
)

// sshBackupRepoPushRetries and sshBackupRepoPushBackoff govern how a push is
// retried immediately after createSSHBareRepository creates a missing
// backup repository. See createAndPushSSHBackupRepo for why the retry
// exists.
const (
	sshBackupRepoPushRetries = 3
	sshBackupRepoPushBackoff = 2 * time.Second
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

// stashChanges stashes uncommitted changes.
// Returns true only when a stash entry was created.
func stashChanges(repoPath string) (bool, error) {
	fmt.Println("  Stashing uncommitted changes...")
	output, err := gitCommand(repoPath, "stash", "push", "-m", "gitsyncer-auto-stash").CombinedOutput()
	if err != nil {
		return false, err
	}

	if strings.Contains(string(output), "No local changes to save") {
		return false, nil
	}

	return true, nil
}

// popStash attempts to pop the stash (used in defer)
func popStash(repoPath string) error {
	output, err := gitCommand(repoPath, "stash", "pop").CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to restore stashed changes: %w\n%s", err, strings.TrimSpace(string(output)))
	}

	return nil
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
		cmd = gitCommand(repoPath, "rev-parse", tag+"^{}")
	} else {
		cmd = gitCommand(repoPath, "ls-remote", "--tags", source, tag)
	}

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return parseTagHashOutput(output, tag, source)
}

// parseTagHashOutput extracts the first whitespace-separated field from output
// as the commit hash for a tag, returning an error when output is empty.
func parseTagHashOutput(output []byte, tag, source string) (string, error) {
	fields := strings.Fields(string(output))
	if len(fields) == 0 {
		return "", fmt.Errorf("no hash found for tag %s from %s", tag, source)
	}
	return fields[0], nil
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

// createSSHBareRepository creates a bare repository on an SSH server.
func createSSHBareRepository(org *config.Organization, repoPath string) error {
	userHost, sshArgs, basePath, err := repositoryCreationLocation(org)
	if err != nil {
		return err
	}

	// Full path to the repository
	fullRepoPath := strings.TrimRight(basePath, "/") + "/" + repoPath + ".git"

	fmt.Printf("Creating bare repository at %s:%s\n", userHost, fullRepoPath)

	// Create the repository directory and initialize as bare
	commands := bareRepoInitCommand(fullRepoPath)
	cmd := exec.Command("ssh", append(sshArgs, commands)...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf("failed to create bare repository: %w\n%s", err, string(output))
	}

	fmt.Printf("Successfully created bare repository at %s:%s\n", userHost, fullRepoPath)
	return nil
}

// bareRepoInitCommand builds the remote shell command used to create a bare
// git repository at fullRepoPath.
//
// The repository is initialized with "--shared=group" so the resulting
// directory tree is group-writable (mode 2775) and git sets
// core.sharedRepository=group in its config. This matters because
// repository creation for SSH backup locations (e.g. the r0 git-server,
// see repositoryCreationLocation/DescriptionSyncHost) runs "git init" over
// a root SSH session, while pushes into that same repository arrive later
// through the git-server pod's SSH endpoint as a different, non-root UID
// (UID 1001, GID 33/www-data - see the git-server helm chart's
// docker-image/Dockerfile). A plain "git init --bare" creates the directory
// with the root session's default umask (0755, effectively owner-only
// writable), so the very next push from the non-root git-server user fails
// with "unable to create temporary object directory". "--shared=group"
// makes the freshly created repository writable by any member of its
// owning group immediately, matching how the already-working repositories
// on the backup host are provisioned (mode 2775).
func bareRepoInitCommand(fullRepoPath string) string {
	return fmt.Sprintf("mkdir -p %q && cd %q && git init --bare --shared=group", fullRepoPath, fullRepoPath)
}

func repositoryCreationLocation(org *config.Organization) (string, []string, string, error) {
	if org == nil {
		return "", nil, "", fmt.Errorf("backup organization is required")
	}

	if org.DescriptionSyncHost != "" && org.DescriptionSyncRoot != "" {
		return org.DescriptionSyncHost, []string{org.DescriptionSyncHost}, org.DescriptionSyncRoot, nil
	}

	return parseSSHLocation(org.Host)
}

func parseSSHLocation(sshHost string) (string, []string, string, error) {
	if strings.HasPrefix(sshHost, "ssh://") {
		parsed, err := url.Parse(sshHost)
		if err != nil {
			return "", nil, "", fmt.Errorf("invalid SSH host format: %w", err)
		}

		host := parsed.Hostname()
		if host == "" || parsed.Path == "" {
			return "", nil, "", fmt.Errorf("invalid SSH host format: %s", sshHost)
		}

		userHost := host
		if parsed.User != nil && parsed.User.Username() != "" {
			userHost = parsed.User.Username() + "@" + host
		}

		sshArgs := make([]string, 0, 3)
		if port := parsed.Port(); port != "" {
			sshArgs = append(sshArgs, "-p", port)
		}
		sshArgs = append(sshArgs, userHost)

		return userHost, sshArgs, parsed.Path, nil
	}

	parts := strings.SplitN(sshHost, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", nil, "", fmt.Errorf("invalid SSH host format: %s", sshHost)
	}

	userHost := parts[0]
	return userHost, []string{userHost}, parts[1], nil
}

// pushBranchWithBackupSupport pushes a branch to a remote, creating SSH repos if needed
func pushBranchWithBackupSupport(repoPath, remoteName, branch string, remoteHasBranch bool, org *config.Organization) error {
	cmd := gitCommand(repoPath, pushBranchArgs(remoteName, branch, false, org)...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		outputStr := string(output)
		// Check if it's because the repository doesn't exist
		if isRepositoryMissing(outputStr) {
			// If it's an SSH backup location, try to create the repository
			if org.BackupLocation && org.IsSSH() && !org.IsForgejo() {
				return createAndPushSSHBackupRepo(repoPath, remoteName, branch, org)
			}

			fmt.Printf("    Note: Remote repository %s does not exist - must be created manually\n", remoteName)
			fmt.Printf("    Skipping push to %s\n", remoteName)
			return nil // Not an error, just skip
		}

		// Check if it's because the branch doesn't exist on the remote
		if isBranchMissing(outputStr) {
			fmt.Printf("    Creating new branch on %s\n", remoteName)
			return pushWithUpstream(repoPath, remoteName, branch, org)
		}

		return fmt.Errorf("failed to push to %s: %w\n%s", remoteName, err, outputStr)
	}

	if !remoteHasBranch {
		fmt.Printf("    Successfully created branch %s on %s\n", branch, remoteName)
	}

	return nil
}

// createAndPushSSHBackupRepo creates a missing bare repository on an SSH
// backup location, then pushes branch into it.
//
// The push is retried a few times with a short backoff because repository
// creation (createSSHBareRepository) runs over a direct root SSH session
// against the backup host's filesystem, while the push that follows goes
// through the git-server's own SSH endpoint (a different process, possibly
// behind its own NFS mount of the same storage). That second path can take
// a moment to observe the directory root just created, so the very next
// push can fail transiently even though the repository now exists and is
// writable. Without a retry, one such transient failure disables backup
// syncing for the remainder of the run (see handlePushError /
// disableBackupForSession), silently skipping every other repository still
// to be processed. Retrying a few times keeps a single slow-to-propagate
// creation from taking out the rest of the backup run.
func createAndPushSSHBackupRepo(repoPath, remoteName, branch string, org *config.Organization) error {
	remoteURL, err := getRemoteURL(repoPath, remoteName)
	if err != nil {
		return fmt.Errorf("failed to get remote URL: %w", err)
	}

	repoName := extractRepoName(remoteURL)
	if repoName == "" {
		return fmt.Errorf("failed to extract repository name from URL: %s", remoteURL)
	}

	if err := createSSHBareRepository(org, repoName); err != nil {
		return fmt.Errorf("failed to create SSH repository: %w", err)
	}

	var lastErr error
	for attempt := 1; attempt <= sshBackupRepoPushRetries; attempt++ {
		cmd := gitCommand(repoPath, pushBranchArgs(remoteName, branch, false, org)...)
		output, err := cmd.CombinedOutput()
		if err == nil {
			fmt.Printf("    Successfully pushed to newly created backup repository\n")
			return nil
		}

		lastErr = fmt.Errorf("failed to push after creating repository (attempt %d/%d): %w\n%s",
			attempt, sshBackupRepoPushRetries, err, strings.TrimSpace(string(output)))
		if attempt < sshBackupRepoPushRetries {
			fmt.Printf("    %v\n    Retrying in %s (newly created backup repository may not be visible yet)...\n", lastErr, sshBackupRepoPushBackoff)
			time.Sleep(sshBackupRepoPushBackoff)
		}
	}

	return lastErr
}

// pushWithUpstream pushes a branch to a remote for the first time, setting
// the local branch to track it (-u).
func pushWithUpstream(repoPath, remoteName, branch string, org *config.Organization) error {
	cmd := gitCommand(repoPath, pushBranchArgs(remoteName, branch, true, org)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to push to %s: %w\n%s", remoteName, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func pushBranchArgs(remoteName, branch string, setUpstream bool, org *config.Organization) []string {
	args := []string{"push"}
	if setUpstream {
		args = append(args, "-u")
	}
	args = append(args, remoteName, branch, "--tags")
	if org != nil && org.BackupLocation && org.ForcePush {
		args = append(args, "--force")
	}
	return args
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
