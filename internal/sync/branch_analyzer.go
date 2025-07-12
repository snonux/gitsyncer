package sync

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// BranchInfo holds information about a branch
type BranchInfo struct {
	Name         string
	LastCommit   time.Time
	Remote       string
	IsAbandoned  bool
	AbandonReason string
	RemotesWithBranch []string // List of remotes that have this branch
}

// AbandonedBranchReport holds the analysis results
type AbandonedBranchReport struct {
	MainBranchUpdated   bool
	MainBranchLastCommit time.Time
	AbandonedBranches   []BranchInfo
	AbandonedIgnoredBranches []BranchInfo // Abandoned branches that match exclusion patterns
	TotalBranches       int
	TotalIgnoredBranches int
}

// analyzeAbandonedBranches analyzes branches to find abandoned ones
func (s *Syncer) analyzeAbandonedBranches() (*AbandonedBranchReport, error) {
	report := &AbandonedBranchReport{
		AbandonedBranches: []BranchInfo{},
		AbandonedIgnoredBranches: []BranchInfo{},
	}

	// Get all branches
	allBranches, err := s.getAllBranches()
	if err != nil {
		return nil, fmt.Errorf("failed to get branches: %w", err)
	}
	
	// Filter branches based on exclusion patterns
	branches := s.branchFilter.FilterBranches(allBranches)
	report.TotalBranches = len(branches)
	
	// Get excluded branches for separate analysis
	excludedBranches := s.branchFilter.GetExcludedBranches(allBranches)
	report.TotalIgnoredBranches = len(excludedBranches)

	// Check main/master branch status
	mainBranch := s.findMainBranch(branches)
	if mainBranch != "" {
		mainInfo, err := s.getBranchInfo(mainBranch)
		if err == nil {
			// Consider project active if main branch has commits within last 3 years
			report.MainBranchUpdated = mainInfo.LastCommit.After(time.Now().AddDate(-3, 0, 0))
			report.MainBranchLastCommit = mainInfo.LastCommit
		}
	}

	// Only analyze if main branch is active (has commits within last 3 years)
	if !report.MainBranchUpdated {
		return report, nil
	}

	// Analyze each branch
	sixMonthsAgo := time.Now().AddDate(0, -6, 0)
	
	for _, branch := range branches {
		// Skip main/master branches
		if branch == "main" || branch == "master" {
			continue
		}

		branchInfo, err := s.getBranchInfo(branch)
		if err != nil {
			continue
		}

		// Check if branch is abandoned (no commits for 6+ months)
		if branchInfo.LastCommit.Before(sixMonthsAgo) {
			branchInfo.IsAbandoned = true
			daysSinceCommit := int(time.Since(branchInfo.LastCommit).Hours() / 24)
			branchInfo.AbandonReason = fmt.Sprintf("No commits for %d days", daysSinceCommit)
			report.AbandonedBranches = append(report.AbandonedBranches, *branchInfo)
		}
	}
	
	// Also analyze ignored branches for abandonment
	for _, branch := range excludedBranches {
		// Skip main/master branches even if they match exclusion patterns
		if branch == "main" || branch == "master" {
			continue
		}

		branchInfo, err := s.getBranchInfo(branch)
		if err != nil {
			continue
		}

		// Check if branch is abandoned (no commits for 6+ months)
		if branchInfo.LastCommit.Before(sixMonthsAgo) {
			branchInfo.IsAbandoned = true
			daysSinceCommit := int(time.Since(branchInfo.LastCommit).Hours() / 24)
			branchInfo.AbandonReason = fmt.Sprintf("No commits for %d days (ignored branch)", daysSinceCommit)
			report.AbandonedIgnoredBranches = append(report.AbandonedIgnoredBranches, *branchInfo)
		}
	}

	return report, nil
}

// findMainBranch finds the main or master branch
func (s *Syncer) findMainBranch(branches []string) string {
	for _, branch := range branches {
		if branch == "main" || branch == "master" {
			return branch
		}
	}
	return ""
}

// getBranchInfo gets information about a specific branch
func (s *Syncer) getBranchInfo(branch string) (*BranchInfo, error) {
	info := &BranchInfo{
		Name: branch,
		RemotesWithBranch: []string{},
	}

	// Find which remote has this branch and get the latest commit
	var latestCommit time.Time
	var latestRemote string

	for i := range s.config.Organizations {
		org := &s.config.Organizations[i]
		
		// Skip backup locations if backup is not enabled
		if org.BackupLocation && !s.backupEnabled {
			continue
		}
		
		remoteName := s.getRemoteName(org)

		if s.remoteBranchExists(remoteName, branch) {
			// Add this remote to the list
			info.RemotesWithBranch = append(info.RemotesWithBranch, remoteName)
			
			// Get last commit date for this branch on this remote
			commitTime, err := s.getLastCommitTime(remoteName, branch)
			if err == nil && (latestCommit.IsZero() || commitTime.After(latestCommit)) {
				latestCommit = commitTime
				latestRemote = remoteName
			}
		}
	}

	if latestCommit.IsZero() {
		// If no remote has the branch, check local
		commitTime, err := s.getLastCommitTime("", branch)
		if err != nil {
			return nil, fmt.Errorf("failed to get commit time for branch %s: %w", branch, err)
		}
		latestCommit = commitTime
		latestRemote = "local"
	}

	info.LastCommit = latestCommit
	info.Remote = latestRemote
	return info, nil
}

// getLastCommitTime gets the last commit time for a branch
func (s *Syncer) getLastCommitTime(remoteName, branch string) (time.Time, error) {
	var ref string
	if remoteName != "" {
		ref = fmt.Sprintf("%s/%s", remoteName, branch)
	} else {
		ref = branch
	}

	// Get Unix timestamp of last commit
	cmd := exec.Command("git", "log", "-1", "--format=%ct", ref)
	output, err := cmd.Output()
	if err != nil {
		return time.Time{}, err
	}

	timestampStr := strings.TrimSpace(string(output))
	timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse timestamp: %w", err)
	}

	return time.Unix(timestamp, 0), nil
}

// formatAbandonedBranchReport formats the report for display
func formatAbandonedBranchReport(report *AbandonedBranchReport, repoName string) string {
	if !report.MainBranchUpdated {
		return "" // Don't report on inactive repos
	}

	if len(report.AbandonedBranches) == 0 && len(report.AbandonedIgnoredBranches) == 0 {
		return "" // No abandoned branches
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n🔍 Abandoned branches in %s:\n", repoName))
	sb.WriteString(fmt.Sprintf("   Main branch last updated: %s\n", report.MainBranchLastCommit.Format("2006-01-02")))
	
	if len(report.AbandonedBranches) > 0 {
		sb.WriteString(fmt.Sprintf("   Found %d abandoned branches (no commits for 6+ months):\n\n", len(report.AbandonedBranches)))
		for _, branch := range report.AbandonedBranches {
			sb.WriteString(fmt.Sprintf("   - %s (last commit: %s, %s)\n", 
				branch.Name, 
				branch.LastCommit.Format("2006-01-02"),
				branch.AbandonReason))
		}
	}
	
	if len(report.AbandonedIgnoredBranches) > 0 {
		sb.WriteString(fmt.Sprintf("\n   Found %d abandoned IGNORED branches (no commits for 6+ months):\n\n", len(report.AbandonedIgnoredBranches)))
		for _, branch := range report.AbandonedIgnoredBranches {
			sb.WriteString(fmt.Sprintf("   - %s (last commit: %s, %s)\n", 
				branch.Name, 
				branch.LastCommit.Format("2006-01-02"),
				branch.AbandonReason))
		}
	}

	return sb.String()
}

// GenerateAbandonedBranchSummary generates a summary of all abandoned branches across repos
func (s *Syncer) GenerateAbandonedBranchSummary() string {
	if len(s.abandonedReports) == 0 {
		return ""
	}

	totalAbandoned := 0
	totalAbandonedIgnored := 0
	reposWithAbandoned := 0
	
	for _, report := range s.abandonedReports {
		if len(report.AbandonedBranches) > 0 || len(report.AbandonedIgnoredBranches) > 0 {
			totalAbandoned += len(report.AbandonedBranches)
			totalAbandonedIgnored += len(report.AbandonedIgnoredBranches)
			reposWithAbandoned++
		}
	}

	if totalAbandoned == 0 && totalAbandonedIgnored == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("=", 70))
	sb.WriteString("\n📊 ABANDONED BRANCHES SUMMARY\n")
	sb.WriteString(strings.Repeat("=", 70))
	sb.WriteString("\n\n")
	sb.WriteString(fmt.Sprintf("Found %d abandoned branches", totalAbandoned))
	if totalAbandonedIgnored > 0 {
		sb.WriteString(fmt.Sprintf(" + %d ignored branches", totalAbandonedIgnored))
	}
	sb.WriteString(fmt.Sprintf(" across %d repositories\n\n", reposWithAbandoned))

	// Group by repository
	for repoName, report := range s.abandonedReports {
		if len(report.AbandonedBranches) == 0 && len(report.AbandonedIgnoredBranches) == 0 {
			continue
		}
		
		totalBranches := len(report.AbandonedBranches) + len(report.AbandonedIgnoredBranches)
		sb.WriteString(fmt.Sprintf("📁 %s (%d branches):\n", repoName, totalBranches))
		
		// Regular abandoned branches
		if len(report.AbandonedBranches) > 0 {
			sb.WriteString("   Regular branches:\n")
			for _, branch := range report.AbandonedBranches {
				sb.WriteString(fmt.Sprintf("   - %s (last commit: %s)\n", 
					branch.Name, 
					branch.LastCommit.Format("2006-01-02")))
			}
		}
		
		// Ignored abandoned branches
		if len(report.AbandonedIgnoredBranches) > 0 {
			sb.WriteString("   Ignored branches:\n")
			for _, branch := range report.AbandonedIgnoredBranches {
				sb.WriteString(fmt.Sprintf("   - %s (last commit: %s)\n", 
					branch.Name, 
					branch.LastCommit.Format("2006-01-02")))
			}
		}
		
		sb.WriteString("\n")
	}

	sb.WriteString("💡 Tip: Consider deleting these branches if they're no longer needed:\n")
	sb.WriteString("   git push origin --delete <branch-name>\n")
	sb.WriteString(strings.Repeat("=", 70))
	sb.WriteString("\n")

	return sb.String()
}

// GenerateDeleteCommands generates shell commands to delete abandoned branches
func (s *Syncer) GenerateDeleteCommands(report *AbandonedBranchReport, repoName string) string {
	if len(report.AbandonedBranches) == 0 && len(report.AbandonedIgnoredBranches) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n# Delete commands for abandoned branches in %s\n", repoName))
	sb.WriteString("# Review these commands carefully before running them!\n\n")

	// Process regular abandoned branches
	if len(report.AbandonedBranches) > 0 {
		sb.WriteString("# === REGULAR BRANCHES ===\n")
		for _, branch := range report.AbandonedBranches {
			sb.WriteString(fmt.Sprintf("# Branch: %s (last commit: %s)\n", branch.Name, branch.LastCommit.Format("2006-01-02")))
			
			// Delete from all remotes that have this branch
			if len(branch.RemotesWithBranch) > 0 {
				sb.WriteString("# Delete from remotes:\n")
				for _, remote := range branch.RemotesWithBranch {
					sb.WriteString(fmt.Sprintf("git push %s --delete %s\n", remote, branch.Name))
				}
			}
			
			// Delete local branch
			sb.WriteString("# Delete local branch:\n")
			sb.WriteString(fmt.Sprintf("git branch -D %s\n\n", branch.Name))
		}
	}
	
	// Process ignored abandoned branches
	if len(report.AbandonedIgnoredBranches) > 0 {
		sb.WriteString("# === IGNORED BRANCHES ===\n")
		for _, branch := range report.AbandonedIgnoredBranches {
			sb.WriteString(fmt.Sprintf("# Branch: %s (last commit: %s) [IGNORED]\n", branch.Name, branch.LastCommit.Format("2006-01-02")))
			
			// Delete from all remotes that have this branch
			if len(branch.RemotesWithBranch) > 0 {
				sb.WriteString("# Delete from remotes:\n")
				for _, remote := range branch.RemotesWithBranch {
					sb.WriteString(fmt.Sprintf("git push %s --delete %s\n", remote, branch.Name))
				}
			}
			
			// Delete local branch
			sb.WriteString("# Delete local branch:\n")
			sb.WriteString(fmt.Sprintf("git branch -D %s\n\n", branch.Name))
		}
	}

	return sb.String()
}

// GenerateDeleteScript generates a shell script file to delete all abandoned branches
func (s *Syncer) GenerateDeleteScript() (string, error) {
	if len(s.abandonedReports) == 0 {
		return "", nil
	}

	// Count total abandoned branches
	totalAbandoned := 0
	totalIgnored := 0
	for _, report := range s.abandonedReports {
		totalAbandoned += len(report.AbandonedBranches)
		totalIgnored += len(report.AbandonedIgnoredBranches)
	}
	
	if totalAbandoned == 0 && totalIgnored == 0 {
		return "", nil
	}

	// Generate script filename with timestamp
	timestamp := time.Now().Format("20060102_150405")
	scriptPath := filepath.Join(s.workDir, fmt.Sprintf("delete_abandoned_branches_%s.sh", timestamp))

	// Create the script file
	file, err := os.Create(scriptPath)
	if err != nil {
		return "", fmt.Errorf("failed to create script file: %w", err)
	}
	defer file.Close()

	// Write script header
	fmt.Fprintf(file, "#!/bin/bash\n")
	fmt.Fprintf(file, "# Gitsyncer - Delete Abandoned Branches Script\n")
	fmt.Fprintf(file, "# Generated on: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(file, "# Total branches to delete: %d regular + %d ignored = %d total\n", totalAbandoned, totalIgnored, totalAbandoned+totalIgnored)
	fmt.Fprintf(file, "#\n")
	fmt.Fprintf(file, "# ⚠️  WARNING: This script will permanently delete branches!\n")
	fmt.Fprintf(file, "# Review carefully before executing.\n")
	fmt.Fprintf(file, "#\n")
	fmt.Fprintf(file, "# Usage:\n")
	fmt.Fprintf(file, "#   bash %s                # Delete branches (with confirmation)\n", filepath.Base(scriptPath))
	fmt.Fprintf(file, "#   bash %s --dry-run      # Preview what will be deleted\n", filepath.Base(scriptPath))
	fmt.Fprintf(file, "#   bash %s --review       # Review diffs before deletion\n", filepath.Base(scriptPath))
	fmt.Fprintf(file, "#   bash %s --review-full  # Review full diffs\n", filepath.Base(scriptPath))
	fmt.Fprintf(file, "\n")

	// Add mode detection
	fmt.Fprintf(file, "# Parse command line arguments\n")
	fmt.Fprintf(file, "MODE=\"delete\"\n")
	fmt.Fprintf(file, "if [[ \"$1\" == \"--dry-run\" ]]; then\n")
	fmt.Fprintf(file, "    MODE=\"dry-run\"\n")
	fmt.Fprintf(file, "elif [[ \"$1\" == \"--review\" ]]; then\n")
	fmt.Fprintf(file, "    MODE=\"review\"\n")
	fmt.Fprintf(file, "elif [[ \"$1\" == \"--review-full\" ]]; then\n")
	fmt.Fprintf(file, "    MODE=\"review-full\"\n")
	fmt.Fprintf(file, "fi\n\n")

	// Add color support for review mode
	fmt.Fprintf(file, "# Color codes for better readability\n")
	fmt.Fprintf(file, "RED='\\033[0;31m'\n")
	fmt.Fprintf(file, "GREEN='\\033[0;32m'\n")
	fmt.Fprintf(file, "YELLOW='\\033[0;33m'\n")
	fmt.Fprintf(file, "BLUE='\\033[0;34m'\n")
	fmt.Fprintf(file, "PURPLE='\\033[0;35m'\n")
	fmt.Fprintf(file, "CYAN='\\033[0;36m'\n")
	fmt.Fprintf(file, "NC='\\033[0m' # No Color\n\n")

	// Add helper functions
	fmt.Fprintf(file, "# Helper function to execute or print commands\n")
	fmt.Fprintf(file, "execute_cmd() {\n")
	fmt.Fprintf(file, "    if [[ \"$MODE\" == \"dry-run\" ]]; then\n")
	fmt.Fprintf(file, "        echo \"  [DRY RUN] $@\"\n")
	fmt.Fprintf(file, "    else\n")
	fmt.Fprintf(file, "        echo \"  Executing: $@\"\n")
	fmt.Fprintf(file, "        \"$@\"\n")
	fmt.Fprintf(file, "    fi\n")
	fmt.Fprintf(file, "}\n\n")

	// Add function to find main branch
	fmt.Fprintf(file, "# Function to find main/master branch\n")
	fmt.Fprintf(file, "find_main_branch() {\n")
	fmt.Fprintf(file, "    if git rev-parse --verify main >/dev/null 2>&1; then\n")
	fmt.Fprintf(file, "        echo \"main\"\n")
	fmt.Fprintf(file, "    elif git rev-parse --verify master >/dev/null 2>&1; then\n")
	fmt.Fprintf(file, "        echo \"master\"\n")
	fmt.Fprintf(file, "    else\n")
	fmt.Fprintf(file, "        echo \"\"\n")
	fmt.Fprintf(file, "    fi\n")
	fmt.Fprintf(file, "}\n\n")

	// Add review function
	fmt.Fprintf(file, "# Function to review branch diff\n")
	fmt.Fprintf(file, "review_branch() {\n")
	fmt.Fprintf(file, "    local branch=\"$1\"\n")
	fmt.Fprintf(file, "    local main_branch=\"$2\"\n")
	fmt.Fprintf(file, "    local last_commit=\"$3\"\n")
	fmt.Fprintf(file, "    local branch_type=\"$4\"\n")
	fmt.Fprintf(file, "    \n")
	fmt.Fprintf(file, "    echo -e \"${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}\"\n")
	fmt.Fprintf(file, "    echo -e \"${YELLOW}Branch:${NC} $branch ${PURPLE}[$branch_type]${NC}\"\n")
	fmt.Fprintf(file, "    echo -e \"${YELLOW}Last commit:${NC} $last_commit\"\n")
	fmt.Fprintf(file, "    echo -e \"${YELLOW}Comparing against:${NC} $main_branch\"\n")
	fmt.Fprintf(file, "    echo -e \"${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}\"\n")
	fmt.Fprintf(file, "    \n")
	fmt.Fprintf(file, "    # Check if branch exists locally\n")
	fmt.Fprintf(file, "    if ! git rev-parse --verify \"$branch\" >/dev/null 2>&1; then\n")
	fmt.Fprintf(file, "        echo -e \"${RED}⚠️  Branch '$branch' not found locally${NC}\"\n")
	fmt.Fprintf(file, "        return\n")
	fmt.Fprintf(file, "    fi\n")
	fmt.Fprintf(file, "    \n")
	fmt.Fprintf(file, "    echo -e \"${GREEN}📊 Diff statistics:${NC}\"\n")
	fmt.Fprintf(file, "    git diff --stat \"$main_branch\"...\"$branch\"\n")
	fmt.Fprintf(file, "    echo\n")
	fmt.Fprintf(file, "    echo -e \"${GREEN}📝 Commits in this branch:${NC}\"\n")
	fmt.Fprintf(file, "    git log --oneline --graph \"$main_branch\"..\"$branch\" | head -20\n")
	fmt.Fprintf(file, "    \n")
	fmt.Fprintf(file, "    if [[ \"$MODE\" == \"review-full\" ]]; then\n")
	fmt.Fprintf(file, "        echo\n")
	fmt.Fprintf(file, "        echo -e \"${GREEN}🔍 Full diff:${NC}\"\n")
	fmt.Fprintf(file, "        git diff \"$main_branch\"...\"$branch\"\n")
	fmt.Fprintf(file, "    fi\n")
	fmt.Fprintf(file, "    echo\n")
	fmt.Fprintf(file, "}\n\n")

	// Start main logic
	fmt.Fprintf(file, "# Main script logic\n")
	fmt.Fprintf(file, "case \"$MODE\" in\n")
	fmt.Fprintf(file, "    \"dry-run\")\n")
	fmt.Fprintf(file, "        echo \"🔍 DRY RUN MODE - No branches will be deleted\"\n")
	fmt.Fprintf(file, "        echo\n")
	fmt.Fprintf(file, "        ;;\n")
	fmt.Fprintf(file, "    \"review\"|\"review-full\")\n")
	fmt.Fprintf(file, "        echo -e \"${CYAN}🔍 Gitsyncer - Abandoned Branch Review${NC}\"\n")
	fmt.Fprintf(file, "        echo -e \"${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}\"\n")
	fmt.Fprintf(file, "        echo -e \"Found ${YELLOW}%d${NC} abandoned branches to review\"\n", totalAbandoned+totalIgnored)
	fmt.Fprintf(file, "        echo\n")
	fmt.Fprintf(file, "        ;;\n")
	fmt.Fprintf(file, "    \"delete\")\n")
	fmt.Fprintf(file, "        echo \"⚠️  This script will delete %d abandoned branches across %d repositories.\"\n", totalAbandoned+totalIgnored, len(s.abandonedReports))
	fmt.Fprintf(file, "        read -p \"Are you sure you want to continue? (yes/no): \" confirm\n")
	fmt.Fprintf(file, "        if [[ \"$confirm\" != \"yes\" ]]; then\n")
	fmt.Fprintf(file, "            echo \"Aborted.\"\n")
	fmt.Fprintf(file, "            exit 0\n")
	fmt.Fprintf(file, "        fi\n")
	fmt.Fprintf(file, "        echo\n")
	fmt.Fprintf(file, "        ;;\n")
	fmt.Fprintf(file, "esac\n\n")

	// Process each repository
	for repoName, report := range s.abandonedReports {
		if len(report.AbandonedBranches) == 0 && len(report.AbandonedIgnoredBranches) == 0 {
			continue
		}

		fmt.Fprintf(file, "# ======================================\n")
		fmt.Fprintf(file, "# Repository: %s\n", repoName)
		fmt.Fprintf(file, "# ======================================\n")
		fmt.Fprintf(file, "echo\n")
		fmt.Fprintf(file, "echo \"📁 Processing repository: %s\"\n", repoName)
		fmt.Fprintf(file, "cd \"%s/%s\" || { echo \"Failed to change to repository directory\"; exit 1; }\n\n", s.workDir, repoName)

		// Find main branch for review mode
		fmt.Fprintf(file, "if [[ \"$MODE\" == \"review\" || \"$MODE\" == \"review-full\" ]]; then\n")
		fmt.Fprintf(file, "    main_branch=$(find_main_branch)\n")
		fmt.Fprintf(file, "    if [[ -z \"$main_branch\" ]]; then\n")
		fmt.Fprintf(file, "        echo -e \"${RED}⚠️  No main/master branch found in %s${NC}\"\n", repoName)
		fmt.Fprintf(file, "    fi\n")
		fmt.Fprintf(file, "fi\n\n")

		// Process regular abandoned branches
		if len(report.AbandonedBranches) > 0 {
			fmt.Fprintf(file, "# Regular abandoned branches\n")
			for _, branch := range report.AbandonedBranches {
				fmt.Fprintf(file, "if [[ \"$MODE\" == \"review\" || \"$MODE\" == \"review-full\" ]]; then\n")
				fmt.Fprintf(file, "    if [[ -n \"$main_branch\" ]]; then\n")
				fmt.Fprintf(file, "        review_branch \"%s\" \"$main_branch\" \"%s\" \"regular\"\n", branch.Name, branch.LastCommit.Format("2006-01-02"))
				fmt.Fprintf(file, "    fi\n")
				fmt.Fprintf(file, "else\n")
				fmt.Fprintf(file, "    echo \"  🔸 Deleting branch: %s (last commit: %s)\"\n", branch.Name, branch.LastCommit.Format("2006-01-02"))
				
				// Delete from remotes
				for _, remote := range branch.RemotesWithBranch {
					fmt.Fprintf(file, "    execute_cmd git push %s --delete \"%s\"\n", remote, branch.Name)
				}
				
				// Delete local branch
				fmt.Fprintf(file, "    execute_cmd git branch -D \"%s\"\n", branch.Name)
				fmt.Fprintf(file, "fi\n\n")
			}
		}

		// Process ignored abandoned branches
		if len(report.AbandonedIgnoredBranches) > 0 {
			fmt.Fprintf(file, "# Ignored abandoned branches\n")
			for _, branch := range report.AbandonedIgnoredBranches {
				fmt.Fprintf(file, "if [[ \"$MODE\" == \"review\" || \"$MODE\" == \"review-full\" ]]; then\n")
				fmt.Fprintf(file, "    if [[ -n \"$main_branch\" ]]; then\n")
				fmt.Fprintf(file, "        review_branch \"%s\" \"$main_branch\" \"%s\" \"ignored\"\n", branch.Name, branch.LastCommit.Format("2006-01-02"))
				fmt.Fprintf(file, "    fi\n")
				fmt.Fprintf(file, "else\n")
				fmt.Fprintf(file, "    echo \"  🔹 Deleting ignored branch: %s (last commit: %s)\"\n", branch.Name, branch.LastCommit.Format("2006-01-02"))
				
				// Delete from remotes
				for _, remote := range branch.RemotesWithBranch {
					fmt.Fprintf(file, "    execute_cmd git push %s --delete \"%s\"\n", remote, branch.Name)
				}
				
				// Delete local branch
				fmt.Fprintf(file, "    execute_cmd git branch -D \"%s\"\n", branch.Name)
				fmt.Fprintf(file, "fi\n\n")
			}
		}
	}

	// Add completion message
	fmt.Fprintf(file, "echo\n")
	fmt.Fprintf(file, "echo \"✅ Script completed!\"\n")
	fmt.Fprintf(file, "case \"$MODE\" in\n")
	fmt.Fprintf(file, "    \"dry-run\")\n")
	fmt.Fprintf(file, "        echo \"This was a dry run. No branches were deleted.\"\n")
	fmt.Fprintf(file, "        echo \"To actually delete branches, run: bash %s\"\n", filepath.Base(scriptPath))
	fmt.Fprintf(file, "        ;;\n")
	fmt.Fprintf(file, "    \"review\"|\"review-full\")\n")
	fmt.Fprintf(file, "        echo \"Review completed. No branches were deleted.\"\n")
	fmt.Fprintf(file, "        echo \"To delete branches, run: bash %s\"\n", filepath.Base(scriptPath))
	fmt.Fprintf(file, "        ;;\n")
	fmt.Fprintf(file, "    \"delete\")\n")
	fmt.Fprintf(file, "        echo \"All abandoned branches have been deleted.\"\n")
	fmt.Fprintf(file, "        ;;\n")
	fmt.Fprintf(file, "esac\n")

	// Make the script executable
	if err := os.Chmod(scriptPath, 0755); err != nil {
		return scriptPath, fmt.Errorf("failed to make script executable: %w", err)
	}

	return scriptPath, nil
}