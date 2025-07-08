package sync

import (
	"fmt"
	"os/exec"
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
}

// AbandonedBranchReport holds the analysis results
type AbandonedBranchReport struct {
	MainBranchUpdated   bool
	MainBranchLastCommit time.Time
	AbandonedBranches   []BranchInfo
	TotalBranches       int
}

// analyzeAbandonedBranches analyzes branches to find abandoned ones
func (s *Syncer) analyzeAbandonedBranches() (*AbandonedBranchReport, error) {
	report := &AbandonedBranchReport{
		AbandonedBranches: []BranchInfo{},
	}

	// Get all branches
	allBranches, err := s.getAllBranches()
	if err != nil {
		return nil, fmt.Errorf("failed to get branches: %w", err)
	}
	
	// Filter branches based on exclusion patterns
	branches := s.branchFilter.FilterBranches(allBranches)
	report.TotalBranches = len(branches)

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
	}

	// Find which remote has this branch and get the latest commit
	var latestCommit time.Time
	var latestRemote string

	for i := range s.config.Organizations {
		org := &s.config.Organizations[i]
		remoteName := s.getRemoteName(org)

		if s.remoteBranchExists(remoteName, branch) {
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

	if len(report.AbandonedBranches) == 0 {
		return "" // No abandoned branches
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n🔍 Abandoned branches in %s:\n", repoName))
	sb.WriteString(fmt.Sprintf("   Main branch last updated: %s\n", report.MainBranchLastCommit.Format("2006-01-02")))
	sb.WriteString(fmt.Sprintf("   Found %d abandoned branches (no commits for 6+ months):\n\n", len(report.AbandonedBranches)))

	for _, branch := range report.AbandonedBranches {
		sb.WriteString(fmt.Sprintf("   - %s (last commit: %s, %s)\n", 
			branch.Name, 
			branch.LastCommit.Format("2006-01-02"),
			branch.AbandonReason))
	}

	return sb.String()
}

// GenerateAbandonedBranchSummary generates a summary of all abandoned branches across repos
func (s *Syncer) GenerateAbandonedBranchSummary() string {
	if len(s.abandonedReports) == 0 {
		return ""
	}

	totalAbandoned := 0
	reposWithAbandoned := 0
	
	for _, report := range s.abandonedReports {
		if len(report.AbandonedBranches) > 0 {
			totalAbandoned += len(report.AbandonedBranches)
			reposWithAbandoned++
		}
	}

	if totalAbandoned == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("=", 70))
	sb.WriteString("\n📊 ABANDONED BRANCHES SUMMARY\n")
	sb.WriteString(strings.Repeat("=", 70))
	sb.WriteString("\n\n")
	sb.WriteString(fmt.Sprintf("Found %d abandoned branches across %d repositories\n\n", totalAbandoned, reposWithAbandoned))

	// Group by repository
	for repoName, report := range s.abandonedReports {
		if len(report.AbandonedBranches) == 0 {
			continue
		}
		
		sb.WriteString(fmt.Sprintf("📁 %s (%d branches):\n", repoName, len(report.AbandonedBranches)))
		for _, branch := range report.AbandonedBranches {
			sb.WriteString(fmt.Sprintf("   - %s (last commit: %s)\n", 
				branch.Name, 
				branch.LastCommit.Format("2006-01-02")))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("💡 Tip: Consider deleting these branches if they're no longer needed:\n")
	sb.WriteString("   git push origin --delete <branch-name>\n")
	sb.WriteString(strings.Repeat("=", 70))
	sb.WriteString("\n")

	return sb.String()
}