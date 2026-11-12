package showcase

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/snonux/gitsyncer/internal/version"
)

const unreleasedScorePenalty = 0.75

// Recent-activity window used by effectiveActivityAge: a project with at
// least minRecentCommits commits on HEAD within the last
// recentActivityWindowDays days is judged by the age of those commits rather
// than by its last 42 commits, which keeps squashed histories from looking
// dormant. Five commits (not one) are required so a stray maintenance commit
// cannot revive an abandoned project.
const (
	recentActivityWindowDays = 90
	minRecentCommits         = 5
)

// inactiveAfterDays is the effective activity age (~2 years) beyond which a
// project is flagged as inactive in the showcase and drawn grey in the
// rank-history SVG.
const inactiveAfterDays = 730

// LanguageStats holds statistics for a programming language
type LanguageStats struct {
	Name       string
	Lines      int
	Percentage float64
}

// RepoMetadata holds metadata about a repository
type RepoMetadata struct {
	Languages         []LanguageStats // Programming languages with usage statistics
	Documentation     []LanguageStats // Documentation/text files with usage statistics
	CommitCount       int
	LinesOfCode       int // Lines of code (excluding documentation)
	LinesOfDocs       int // Lines of documentation
	FirstCommitDate   string
	LastCommitDate    string
	LastActivityDate  string // Most recent commit on any local branch (--all); used for activity checks
	License           string
	AvgCommitAge      float64 // Average age of last 42 commits in days (HEAD only)
	RecentCommitCount int     // Commits on HEAD within the last recentActivityWindowDays days
	ActivityAge       float64 // Effective commit age in days used for score and inactive notice; see effectiveActivityAge
	TagCount          int     // Total number of git tags in the repository
	Score             float64 // Project score combining recent activity, reduced LOC weight, tag count, and release status
	LatestTag         string  // Latest version tag (empty if no tags)
	LatestTagDate     string  // Date of the latest tag (empty if no tags)
	HasReleases       bool    // Whether the project has any releases/tags
}

// extractRepoMetadata extracts metadata from a repository. Each concern
// (languages, commit dates, activity, tags) is gathered by its own helper;
// failures are reported as warnings and leave the affected fields at their
// zero values so a partially broken repository still gets a showcase entry.
func extractRepoMetadata(repoPath string) (*RepoMetadata, error) {
	metadata := &RepoMetadata{}

	extractLanguageMetadata(repoPath, metadata)
	extractCommitDateMetadata(repoPath, metadata)
	metadata.License = detectLicense(repoPath)
	extractActivityMetadata(repoPath, metadata)
	// Tags are gathered before scoring so they can influence the ranking.
	extractTagMetadata(repoPath, metadata)

	// Calculate score with recent activity as the strongest signal,
	// a smaller LOC contribution than before, a modest tag bonus,
	// and a penalty for projects without a release yet.
	metadata.Score = calculateRepoScore(metadata.LinesOfCode, metadata.ActivityAge, metadata.TagCount, metadata.HasReleases)

	return metadata, nil
}

// extractLanguageMetadata fills the language/documentation breakdown and the
// derived code and documentation line totals.
func extractLanguageMetadata(repoPath string, metadata *RepoMetadata) {
	languages, documentation, err := detectLanguages(repoPath)
	if err != nil {
		fmt.Printf("Warning: Failed to detect languages: %v\n", err)
	}
	metadata.Languages = languages
	metadata.Documentation = documentation

	for _, lang := range metadata.Languages {
		metadata.LinesOfCode += lang.Lines
	}
	for _, doc := range metadata.Documentation {
		metadata.LinesOfDocs += doc.Lines
	}
}

// extractCommitDateMetadata fills the commit count and the first/last commit
// dates of HEAD, plus the last activity date across all local branches.
func extractCommitDateMetadata(repoPath string, metadata *RepoMetadata) {
	commitCount, err := getCommitCount(repoPath)
	if err != nil {
		fmt.Printf("Warning: Failed to get commit count: %v\n", err)
	}
	metadata.CommitCount = commitCount

	firstDate, err := getFirstCommitDate(repoPath)
	if err != nil {
		fmt.Printf("Warning: Failed to get first commit date: %v\n", err)
	}
	metadata.FirstCommitDate = firstDate

	lastDate, err := getLastCommitDate(repoPath)
	if err != nil {
		fmt.Printf("Warning: Failed to get last commit date: %v\n", err)
	}
	metadata.LastCommitDate = lastDate

	// LastActivityDate is the most recent commit across ALL local branches so
	// that projects with active development on non-default branches (e.g. a
	// "develop" branch while HEAD is an old "master") are not falsely marked
	// inactive.  Code stats (LOC, ActivityAge, score) remain HEAD-only per
	// the configured branch rules.
	activityDate, err := getLastActivityDate(repoPath)
	if err != nil {
		fmt.Printf("Warning: Failed to get last activity date: %v\n", err)
		activityDate = lastDate // fall back to HEAD-based date
	}
	metadata.LastActivityDate = activityDate
}

// extractActivityMetadata fills AvgCommitAge (last 42 commits on HEAD, 42 is
// the answer!), the recent-commit statistics and the effective ActivityAge
// used for scoring and the inactive notice. See effectiveActivityAge for why
// both measures are needed.
func extractActivityMetadata(repoPath string, metadata *RepoMetadata) {
	avgAge, err := getAverageCommitAge(repoPath, 42)
	if err != nil {
		fmt.Printf("Warning: Failed to get average commit age: %v\n", err)
	}
	metadata.AvgCommitAge = avgAge

	recentCount, recentAvgAge, err := getRecentCommitStats(repoPath, recentActivityWindowDays)
	if err != nil {
		fmt.Printf("Warning: Failed to get recent commit stats: %v\n", err)
	}
	metadata.RecentCommitCount = recentCount
	metadata.ActivityAge = effectiveActivityAge(avgAge, recentCount, recentAvgAge)
}

// extractTagMetadata fills the latest version tag, its date, the release
// flag and the total tag count.
func extractTagMetadata(repoPath string, metadata *RepoMetadata) {
	latestTag, latestTagDate, hasReleases, tagCount, err := getLatestTag(repoPath)
	if err != nil {
		fmt.Printf("Warning: Failed to get latest tag: %v\n", err)
	}
	metadata.LatestTag = latestTag
	metadata.LatestTagDate = latestTagDate
	metadata.HasReleases = hasReleases
	metadata.TagCount = tagCount
}

// effectiveActivityAge returns the commit age (in days) that drives the score
// and the inactive notice.
//
// The baseline is the average age of the last 42 commits, so one stray
// commit (e.g. a deprecation notice) cannot make a dormant project look
// active. That measure breaks for projects whose recent history was squashed:
// hundreds of recent commits collapse into a handful, and the remaining slots
// of the 42-commit window are filled with years-old commits (dtail: 323
// commits since July squashed into 9, averaging ~935 days). So when a project
// has at least minRecentCommits commits within the recent window, the average
// age of those recent commits is used instead, but only if it is lower:
// projects that are already active by the 42-commit measure never get worse.
func effectiveActivityAge(avgCommitAge float64, recentCount int, recentAvgAge float64) float64 {
	if recentCount >= minRecentCommits && recentAvgAge < avgCommitAge {
		return recentAvgAge
	}
	return avgCommitAge
}

// ActivityAgeDays returns the effective activity age, falling back to
// AvgCommitAge for summaries cached before ActivityAge existed (they decode
// with ActivityAge == 0, which would otherwise read as "brand new").
func (m *RepoMetadata) ActivityAgeDays() float64 {
	if m.ActivityAge > 0 {
		return m.ActivityAge
	}
	return m.AvgCommitAge
}

// IsInactive reports whether the project looks unmaintained: its effective
// activity age (see effectiveActivityAge) exceeds inactiveAfterDays. Both the
// gemtext notice and the rank-history SVG use this so they always agree.
func (m *RepoMetadata) IsInactive() bool {
	return m.ActivityAgeDays() > inactiveAfterDays
}

// calculateRepoScore combines code size and tag count, divided by the
// effective activity age (see effectiveActivityAge) so recently active
// projects rank highest.
func calculateRepoScore(linesOfCode int, activityAge float64, tagCount int, hasReleases bool) float64 {
	sizeComponent := 0.0
	if linesOfCode > 0 {
		sizeComponent = math.Sqrt(math.Log10(float64(linesOfCode)+1.0)) * 250.0
	}

	tagComponent := 0.0
	if tagCount > 0 {
		tagComponent = math.Log1p(float64(tagCount)) * 40.0
	}

	score := (sizeComponent + tagComponent) / (activityAge + 1.0)
	if !hasReleases {
		score *= unreleasedScorePenalty
	}

	return score
}

// getCommitCount returns the total number of commits reachable from the current HEAD.
func getCommitCount(repoPath string) (int, error) {
	cmd := exec.Command("git", "-C", repoPath, "rev-list", "--count", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	count, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return 0, err
	}

	return count, nil
}

// getFirstCommitDate returns the date of the first commit
func getFirstCommitDate(repoPath string) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "log", "--reverse", "--pretty=format:%ai", "--date=short", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) > 0 && lines[0] != "" {
		// Extract just the date part (YYYY-MM-DD)
		parts := strings.Fields(lines[0])
		if len(parts) > 0 {
			return parts[0], nil
		}
	}

	return "", fmt.Errorf("no commits found")
}

// getLastCommitDate returns the date of the last commit
func getLastCommitDate(repoPath string) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "log", "-1", "--pretty=format:%ai", "--date=short", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	// Extract just the date part (YYYY-MM-DD)
	parts := strings.Fields(string(output))
	if len(parts) > 0 {
		return parts[0], nil
	}

	return "", fmt.Errorf("no commits found")
}

// getLastActivityDate returns the date of the most recent commit across all
// local branches (--all).  This is intentionally local-only: no network
// fetch is performed, so it reflects the state of the last sync.  It is used
// solely for the inactivity check (greying in the rank-history SVG) so that
// projects with active work on non-default branches are not falsely flagged.
// Code stats (LOC, AvgCommitAge, score) remain HEAD-only as per config.
func getLastActivityDate(repoPath string) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "log", "--all", "-1", "--pretty=format:%ai")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	parts := strings.Fields(string(output))
	if len(parts) > 0 {
		return parts[0], nil
	}
	return "", fmt.Errorf("no commits found across all branches")
}

// detectLicense checks for common license files
func detectLicense(repoPath string) string {
	licenseFiles := []string{
		"LICENSE",
		"LICENSE.txt",
		"LICENSE.md",
		"license",
		"license.txt",
		"license.md",
		"COPYING",
		"COPYING.txt",
		"COPYRIGHT",
		"COPYRIGHT.txt",
	}

	for _, filename := range licenseFiles {
		path := filepath.Join(repoPath, filename)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			// Try to detect license type by reading the file
			content, err := os.ReadFile(path)
			if err == nil {
				contentStr := string(content)
				switch {
				case strings.Contains(contentStr, "MIT License"):
					return "MIT"
				case strings.Contains(contentStr, "Apache License") && strings.Contains(contentStr, "Version 2.0"):
					return "Apache-2.0"
				case strings.Contains(contentStr, "GNU GENERAL PUBLIC LICENSE") && strings.Contains(contentStr, "Version 3"):
					return "GPL-3.0"
				case strings.Contains(contentStr, "GNU GENERAL PUBLIC LICENSE") && strings.Contains(contentStr, "Version 2"):
					return "GPL-2.0"
				case strings.Contains(contentStr, "BSD 3-Clause License"):
					return "BSD-3-Clause"
				case strings.Contains(contentStr, "BSD 2-Clause License"):
					return "BSD-2-Clause"
				case strings.Contains(contentStr, "Mozilla Public License Version 2.0"):
					return "MPL-2.0"
				case strings.Contains(contentStr, "ISC License"):
					return "ISC"
				case strings.Contains(contentStr, "GNU LESSER GENERAL PUBLIC LICENSE"):
					return "LGPL"
				case strings.Contains(contentStr, "The Unlicense"):
					return "Unlicense"
				case strings.Contains(contentStr, "CC0"):
					return "CC0"
				default:
					return "Custom License"
				}
			}
			return "License file found"
		}
	}

	return "No license found"
}

// getAverageCommitAge calculates the average age of the last N commits in days
func getAverageCommitAge(repoPath string, commitCount int) (float64, error) {
	// Get the last N commit dates
	cmd := exec.Command("git", "-C", repoPath, "log", fmt.Sprintf("-%d", commitCount), "--pretty=format:%at", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	count, avgAge := averageAgeDays(string(output), time.Now())
	if count == 0 {
		return 0, fmt.Errorf("no commits found")
	}
	return avgAge, nil
}

// getRecentCommitStats returns how many commits on HEAD are younger than
// windowDays and their average age in days. A repository without recent
// commits yields (0, 0, nil); that is a normal state, not an error.
func getRecentCommitStats(repoPath string, windowDays int) (int, float64, error) {
	cmd := exec.Command("git", "-C", repoPath, "log", fmt.Sprintf("--since=%d.days", windowDays), "--pretty=format:%at", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return 0, 0, err
	}

	count, avgAge := averageAgeDays(string(output), time.Now())
	return count, avgAge, nil
}

// averageAgeDays parses newline-separated Unix timestamps (git's %at) and
// returns how many were valid and their average age in days relative to
// now. Blank or malformed lines are skipped; zero valid lines yield (0, 0).
func averageAgeDays(output string, now time.Time) (int, float64) {
	nowUnix := float64(now.Unix())
	var totalAge float64
	validCommits := 0

	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		timestamp, err := strconv.ParseInt(strings.TrimSpace(line), 10, 64)
		if err != nil {
			continue
		}
		totalAge += (nowUnix - float64(timestamp)) / 86400 // seconds -> days
		validCommits++
	}

	if validCommits == 0 {
		return 0, 0
	}
	return validCommits, totalAge / float64(validCommits)
}

// getLatestTag returns the latest version-like tag merged into HEAD, its date,
// whether the repo has releases, and total merged tag count.
func getLatestTag(repoPath string) (string, string, bool, int, error) {
	// First try to get tags sorted by version
	cmd := exec.Command("git", "-C", repoPath, "tag", "-l", "--merged", "HEAD", "--sort=-version:refname")
	output, err := cmd.Output()
	if err != nil {
		// Fallback to describe
		cmd = exec.Command("git", "-C", repoPath, "describe", "--tags", "--abbrev=0", "HEAD")
		output, err = cmd.Output()
		if err != nil {
			// No tags at all
			return "", "", false, 0, nil
		}
	}

	tags := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(tags) == 0 || tags[0] == "" {
		return "", "", false, 0, nil
	}

	tagCount := 0
	for _, tag := range tags {
		if strings.TrimSpace(tag) != "" {
			tagCount++
		}
	}

	// Find the first tag that looks like a version number
	latestTag := ""
	for _, tag := range tags {
		if version.IsVersionTag(tag) {
			latestTag = tag
			break
		}
	}

	if latestTag == "" {
		// No version-like tags found
		return "", "", false, tagCount, nil
	}

	// Get the date of the latest tag
	cmd = exec.Command("git", "-C", repoPath, "log", "-1", "--format=%ai", latestTag)
	dateOutput, err := cmd.Output()
	if err != nil {
		// Tag exists but couldn't get date
		return latestTag, "", true, tagCount, nil
	}

	// Extract just the date part (YYYY-MM-DD)
	parts := strings.Fields(string(dateOutput))
	tagDate := ""
	if len(parts) > 0 {
		tagDate = parts[0]
	}

	// Return the latest tag and its date
	return latestTag, tagDate, true, tagCount, nil
}
