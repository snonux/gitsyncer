package sync

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"
)

//go:embed delete_script.tmpl
var deleteScriptTemplateText string

var deleteScriptTemplate = template.Must(template.New("deleteScript").Parse(deleteScriptTemplateText))

type deleteScriptTemplateData struct {
	GeneratedAt     string
	TotalAbandoned  int
	TotalIgnored    int
	TotalBranches   int
	RepositoryCount int
	ScriptBaseName  string
}

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

var defaultAutoDeleteProtectedBranches = map[string]map[string]struct{}{
	"xerl": {
		"hosts": {},
	},
}

func isProtectedFromAutoDelete(repoName, branchName string) bool {
	branches, ok := defaultAutoDeleteProtectedBranches[repoName]
	if !ok {
		return false
	}

	_, ok = branches[branchName]
	return ok
}

func filterProtectedBranchInfos(repoName string, branches []BranchInfo) []BranchInfo {
	if len(branches) == 0 {
		return branches
	}

	filtered := make([]BranchInfo, 0, len(branches))
	for _, branch := range branches {
		if isProtectedFromAutoDelete(repoName, branch.Name) {
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

	return filterProtectedAbandonedBranchReport(s.repoName, report), nil
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

func writeDeleteScriptTemplate(writer io.Writer, templateName string, data deleteScriptTemplateData) error {
	if err := deleteScriptTemplate.ExecuteTemplate(writer, templateName, data); err != nil {
		return fmt.Errorf("failed to execute %s template: %w", templateName, err)
	}

	return nil
}

func writeBranchDeletionBlock(writer io.Writer, branches []BranchInfo, reviewBranchType, deleteMessagePrefix string) error {
	for _, branch := range branches {
		if _, err := fmt.Fprintf(writer, "if [[ \"$MODE\" == \"review\" || \"$MODE\" == \"review-full\" ]]; then\n"); err != nil {
			return fmt.Errorf("failed to write review mode condition for branch %s: %w", branch.Name, err)
		}
		if _, err := fmt.Fprintf(writer, "    if [[ -n \"$main_branch\" ]]; then\n"); err != nil {
			return fmt.Errorf("failed to write main branch check for branch %s: %w", branch.Name, err)
		}
		if _, err := fmt.Fprintf(writer, "        review_branch \"%s\" \"$main_branch\" \"%s\" \"%s\"\n", branch.Name, branch.LastCommit.Format("2006-01-02"), reviewBranchType); err != nil {
			return fmt.Errorf("failed to write review command for branch %s: %w", branch.Name, err)
		}
		if _, err := fmt.Fprintf(writer, "    fi\n"); err != nil {
			return fmt.Errorf("failed to write review branch end for branch %s: %w", branch.Name, err)
		}
		if _, err := fmt.Fprintf(writer, "else\n"); err != nil {
			return fmt.Errorf("failed to write delete branch condition for branch %s: %w", branch.Name, err)
		}
		if _, err := fmt.Fprintf(writer, "    echo \"  %s%s (last commit: %s)\"\n", deleteMessagePrefix, branch.Name, branch.LastCommit.Format("2006-01-02")); err != nil {
			return fmt.Errorf("failed to write delete message for branch %s: %w", branch.Name, err)
		}
		if _, err := fmt.Fprintf(writer, "    # Check if we're on the branch to be deleted\n"); err != nil {
			return fmt.Errorf("failed to write current branch comment for branch %s: %w", branch.Name, err)
		}
		if _, err := fmt.Fprintf(writer, "    current_branch=$(git branch --show-current)\n"); err != nil {
			return fmt.Errorf("failed to write current branch command for branch %s: %w", branch.Name, err)
		}
		if _, err := fmt.Fprintf(writer, "    if [[ \"$current_branch\" == \"%s\" ]]; then\n", branch.Name); err != nil {
			return fmt.Errorf("failed to write current branch condition for branch %s: %w", branch.Name, err)
		}
		if _, err := fmt.Fprintf(writer, "        echo \"    Switching from %s to main/master branch before deletion...\"\n", branch.Name); err != nil {
			return fmt.Errorf("failed to write branch switch message for branch %s: %w", branch.Name, err)
		}
		if _, err := fmt.Fprintf(writer, "        main_branch=$(find_main_branch)\n"); err != nil {
			return fmt.Errorf("failed to write find main branch command for branch %s: %w", branch.Name, err)
		}
		if _, err := fmt.Fprintf(writer, "        if [[ -n \"$main_branch\" ]]; then\n"); err != nil {
			return fmt.Errorf("failed to write branch switch check for branch %s: %w", branch.Name, err)
		}
		if _, err := fmt.Fprintf(writer, "            execute_cmd git checkout \"$main_branch\"\n"); err != nil {
			return fmt.Errorf("failed to write checkout command for branch %s: %w", branch.Name, err)
		}
		if _, err := fmt.Fprintf(writer, "        else\n"); err != nil {
			return fmt.Errorf("failed to write missing main branch else block for branch %s: %w", branch.Name, err)
		}
		if _, err := fmt.Fprintf(writer, "            echo \"    ⚠️  No main/master branch found to switch to!\"\n"); err != nil {
			return fmt.Errorf("failed to write missing main branch message for branch %s: %w", branch.Name, err)
		}
		if _, err := fmt.Fprintf(writer, "            echo \"    Skipping deletion of %s\"\n", branch.Name); err != nil {
			return fmt.Errorf("failed to write skip branch message for branch %s: %w", branch.Name, err)
		}
		if _, err := fmt.Fprintf(writer, "        fi\n"); err != nil {
			return fmt.Errorf("failed to write main branch switch end for branch %s: %w", branch.Name, err)
		}
		if _, err := fmt.Fprintf(writer, "    fi\n"); err != nil {
			return fmt.Errorf("failed to write current branch condition end for branch %s: %w", branch.Name, err)
		}
		if _, err := fmt.Fprintf(writer, "    # Skip to next branch if we couldn't switch\n"); err != nil {
			return fmt.Errorf("failed to write skip comment for branch %s: %w", branch.Name, err)
		}
		if _, err := fmt.Fprintf(writer, "    if [[ \"$current_branch\" == \"%s\" ]] && [[ -z \"$main_branch\" ]]; then\n", branch.Name); err != nil {
			return fmt.Errorf("failed to write skip condition for branch %s: %w", branch.Name, err)
		}
		if _, err := fmt.Fprintf(writer, "        continue\n"); err != nil {
			return fmt.Errorf("failed to write continue for branch %s: %w", branch.Name, err)
		}
		if _, err := fmt.Fprintf(writer, "    fi\n"); err != nil {
			return fmt.Errorf("failed to write skip condition end for branch %s: %w", branch.Name, err)
		}
		for _, remote := range branch.RemotesWithBranch {
			if _, err := fmt.Fprintf(writer, "    execute_cmd git push %s --delete \"%s\"\n", remote, branch.Name); err != nil {
				return fmt.Errorf("failed to write remote delete command for branch %s: %w", branch.Name, err)
			}
		}
		if _, err := fmt.Fprintf(writer, "    execute_cmd git branch -D \"%s\"\n", branch.Name); err != nil {
			return fmt.Errorf("failed to write local delete command for branch %s: %w", branch.Name, err)
		}
		if _, err := fmt.Fprintf(writer, "fi\n\n"); err != nil {
			return fmt.Errorf("failed to write branch block end for branch %s: %w", branch.Name, err)
		}
	}

	return nil
}

// GenerateDeleteScript generates a shell script file to delete all abandoned branches
func (s *Syncer) GenerateDeleteScript() (string, error) {
	if len(s.abandonedReports) == 0 {
		return "", nil
	}

	// Count total abandoned branches
	totalAbandoned := 0
	totalIgnored := 0
	for repoName, report := range s.abandonedReports {
		report = filterProtectedAbandonedBranchReport(repoName, report)
		totalAbandoned += len(report.AbandonedBranches)
		totalIgnored += len(report.AbandonedIgnoredBranches)
	}

	if totalAbandoned == 0 && totalIgnored == 0 {
		return "", nil
	}

	// Generate script filename with timestamp
	timestamp := time.Now().Format("20060102_150405")
	scriptPath := filepath.Join(s.workDir, fmt.Sprintf("delete_abandoned_branches_%s.sh", timestamp))
	scriptBaseName := filepath.Base(scriptPath)

	// Create the script file
	file, err := os.Create(scriptPath)
	if err != nil {
		return "", fmt.Errorf("failed to create script file: %w", err)
	}
	defer file.Close()

	if err := writeDeleteScriptTemplate(file, "deleteScriptPreamble", deleteScriptTemplateData{
		GeneratedAt:     time.Now().Format("2006-01-02 15:04:05"),
		TotalAbandoned:  totalAbandoned,
		TotalIgnored:    totalIgnored,
		TotalBranches:   totalAbandoned + totalIgnored,
		RepositoryCount: len(s.abandonedReports),
		ScriptBaseName:  scriptBaseName,
	}); err != nil {
		return scriptPath, err
	}

	// Process each repository
	for repoName, report := range s.abandonedReports {
		report = filterProtectedAbandonedBranchReport(repoName, report)
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
			if err := writeBranchDeletionBlock(file, report.AbandonedBranches, "regular", "🔸 Deleting branch: "); err != nil {
				return scriptPath, err
			}
		}

		// Process ignored abandoned branches
		if len(report.AbandonedIgnoredBranches) > 0 {
			fmt.Fprintf(file, "# Ignored abandoned branches\n")
			if err := writeBranchDeletionBlock(file, report.AbandonedIgnoredBranches, "ignored", "🔹 Deleting ignored branch: "); err != nil {
				return scriptPath, err
			}
		}
	}

	if err := writeDeleteScriptTemplate(file, "deleteScriptFooter", deleteScriptTemplateData{
		ScriptBaseName: scriptBaseName,
	}); err != nil {
		return scriptPath, err
	}

	// Make the script executable
	if err := os.Chmod(scriptPath, 0755); err != nil {
		return scriptPath, fmt.Errorf("failed to make script executable: %w", err)
	}

	return scriptPath, nil
}
