package sync

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"codeberg.org/snonux/gitsyncer/internal/config"
	"codeberg.org/snonux/gitsyncer/internal/sync/deletescript"
)

// BranchInfo holds information about a branch
type BranchInfo struct {
	Name              string
	LastCommit        time.Time
	Remote            string
	IsAbandoned       bool
	AbandonReason     string
	RemotesWithBranch []string // List of remotes that have this branch
}

// AbandonedBranchReport holds the analysis results
type AbandonedBranchReport struct {
	MainBranchUpdated        bool
	MainBranchLastCommit     time.Time
	AbandonedBranches        []BranchInfo
	AbandonedIgnoredBranches []BranchInfo // Abandoned branches that match exclusion patterns
	TotalBranches            int
	TotalIgnoredBranches     int
}

// filterProtectedBranchInfos drops branches that the config package's
// auto-delete protection policy exempts for repoName (e.g. a branch that
// carries meaning beyond its git history, such as "hosts" in "xerl"). The
// policy itself lives in internal/config since it is a repo-wide safety
// setting, not something specific to branch analysis.
func filterProtectedBranchInfos(repoName string, branches []BranchInfo) []BranchInfo {
	if len(branches) == 0 {
		return branches
	}

	filtered := make([]BranchInfo, 0, len(branches))
	for _, branch := range branches {
		if config.IsProtectedFromAutoDelete(repoName, branch.Name) {
			continue
		}
		filtered = append(filtered, branch)
	}

	return filtered
}

func filterProtectedAbandonedBranchReport(repoName string, report *AbandonedBranchReport) *AbandonedBranchReport {
	if report == nil {
		return nil
	}

	filtered := *report
	filtered.AbandonedBranches = filterProtectedBranchInfos(repoName, report.AbandonedBranches)
	filtered.AbandonedIgnoredBranches = filterProtectedBranchInfos(repoName, report.AbandonedIgnoredBranches)

	return &filtered
}

// analyzeAbandonedBranches analyzes branches to find abandoned ones
func (s *Syncer) analyzeAbandonedBranches() (*AbandonedBranchReport, error) {
	report := &AbandonedBranchReport{
		AbandonedBranches:        []BranchInfo{},
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

	// Analyze each branch. Regular and excluded/ignored branches go through
	// the same get-info/compare-against-sixMonthsAgo/append logic; only the
	// reason-string suffix differs, so both calls share collectAbandoned.
	sixMonthsAgo := time.Now().AddDate(0, -6, 0)

	report.AbandonedBranches = s.collectAbandoned(branches, sixMonthsAgo, "")
	report.AbandonedIgnoredBranches = s.collectAbandoned(excludedBranches, sixMonthsAgo, " (ignored branch)")

	return filterProtectedAbandonedBranchReport(s.repoName, report), nil
}

// collectAbandoned inspects each named branch and returns the BranchInfo
// entries whose last commit predates sixMonthsAgo (i.e. abandoned). It backs
// both the regular and excluded-branch passes of analyzeAbandonedBranches;
// reasonSuffix distinguishes the two in the resulting AbandonReason text
// (e.g. " (ignored branch)" for excluded branches, "" for regular ones).
// main/master branches are always skipped, even if present in names because
// they match an exclusion pattern.
func (s *Syncer) collectAbandoned(names []string, sixMonthsAgo time.Time, reasonSuffix string) []BranchInfo {
	var abandoned []BranchInfo

	for _, branch := range names {
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
			branchInfo.AbandonReason = fmt.Sprintf("No commits for %d days%s", daysSinceCommit, reasonSuffix)
			abandoned = append(abandoned, *branchInfo)
		}
	}

	return abandoned
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
		Name:              branch,
		RemotesWithBranch: []string{},
	}

	// Find which remote has this branch and get the latest commit
	var latestCommit time.Time
	var latestRemote string

	orgs := s.syncOrgs()
	for i := range orgs {
		org := &orgs[i]

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
	cmd := gitCommand(s.repoPath(), "log", "-1", "--format=%ct", ref)
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
	report = filterProtectedAbandonedBranchReport(repoName, report)
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

	for repoName, report := range s.abandonedReports {
		report = filterProtectedAbandonedBranchReport(repoName, report)
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
		report = filterProtectedAbandonedBranchReport(repoName, report)
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
	report = filterProtectedAbandonedBranchReport(repoName, report)
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

// GenerateDeleteScript generates a shell script file to delete all abandoned
// branches across every repository analyzed this session. Filtering
// protected branches and turning the collected reports into the values the
// deletescript package needs happens here; deletescript itself only knows
// how to render bash from those values (see deleteScriptReports below).
func (s *Syncer) GenerateDeleteScript() (string, error) {
	return deletescript.Generate(s.workDir, s.deleteScriptReports())
}

// deleteScriptReports converts the Syncer's collected abandoned-branch
// reports into deletescript.RepoReport values, filtering out branches
// protected from automatic deletion and skipping repositories left with
// nothing to delete once that filtering is applied.
func (s *Syncer) deleteScriptReports() []deletescript.RepoReport {
	reports := make([]deletescript.RepoReport, 0, len(s.abandonedReports))
	for repoName, report := range s.abandonedReports {
		report = filterProtectedAbandonedBranchReport(repoName, report)
		if len(report.AbandonedBranches) == 0 && len(report.AbandonedIgnoredBranches) == 0 {
			continue
		}
		reports = append(reports, deletescript.RepoReport{
			RepoName: repoName,
			Regular:  toDeleteScriptBranches(report.AbandonedBranches),
			Ignored:  toDeleteScriptBranches(report.AbandonedIgnoredBranches),
		})
	}
	return reports
}

// toDeleteScriptBranches narrows BranchInfo down to the fields the
// deletescript package needs (name, last commit, remotes), keeping that
// package decoupled from the sync package's richer branch-analysis type.
func toDeleteScriptBranches(branches []BranchInfo) []deletescript.BranchInfo {
	out := make([]deletescript.BranchInfo, 0, len(branches))
	for _, b := range branches {
		out = append(out, deletescript.BranchInfo{
			Name:              b.Name,
			LastCommit:        b.LastCommit,
			RemotesWithBranch: b.RemotesWithBranch,
		})
	}
	return out
}
