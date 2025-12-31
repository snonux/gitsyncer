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
)

// LanguageStats holds statistics for a programming language
type LanguageStats struct {
	Name       string
	Lines      int
	Percentage float64
}

// RepoMetadata holds metadata about a repository
type RepoMetadata struct {
	Languages       []LanguageStats // Programming languages with usage statistics
	Documentation   []LanguageStats // Documentation/text files with usage statistics
	CommitCount     int
	LinesOfCode     int // Lines of code (excluding documentation)
	LinesOfDocs     int // Lines of documentation
	FirstCommitDate string
	LastCommitDate  string
	License         string
	AvgCommitAge    float64 // Average age of last 42 commits in days
	Score           float64 // Project score combining LOC and recent activity: log10(LOC) * 1000 / (avgCommitAge + 1)
	LatestTag       string  // Latest version tag (empty if no tags)
	LatestTagDate   string  // Date of the latest tag (empty if no tags)
	HasReleases     bool    // Whether the project has any releases/tags
}

// extractRepoMetadata extracts metadata from a repository
func extractRepoMetadata(repoPath string) (*RepoMetadata, error) {
	metadata := &RepoMetadata{}

	// Get programming languages and documentation by analyzing file extensions
	languages, documentation, err := detectLanguages(repoPath)
	if err != nil {
		fmt.Printf("Warning: Failed to detect languages: %v\n", err)
	}
	metadata.Languages = languages
	metadata.Documentation = documentation

	// Get commit count
	commitCount, err := getCommitCount(repoPath)
	if err != nil {
		fmt.Printf("Warning: Failed to get commit count: %v\n", err)
	}
	metadata.CommitCount = commitCount

	// Calculate lines of code and documentation from language stats
	loc := 0
	for _, lang := range metadata.Languages {
		loc += lang.Lines
	}
	metadata.LinesOfCode = loc

	locDocs := 0
	for _, doc := range metadata.Documentation {
		locDocs += doc.Lines
	}
	metadata.LinesOfDocs = locDocs

	// Get first and last commit dates
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

	// Check for license file
	license := detectLicense(repoPath)
	metadata.License = license

	// Get average age of last 42 commits (42 is the answer!)
	avgAge, err := getAverageCommitAge(repoPath, 42)
	if err != nil {
		fmt.Printf("Warning: Failed to get average commit age: %v\n", err)
	}
	metadata.AvgCommitAge = avgAge

	// Calculate score: log10(LOC) * 1000 / (avgCommitAge + 1)
	// This balances project size with recent activity
	score := 0.0
	if metadata.LinesOfCode > 0 {
		score = math.Log10(float64(metadata.LinesOfCode)) * 1000.0 / (metadata.AvgCommitAge + 1.0)
	}
	metadata.Score = score

	// Get latest tag and check for releases
	latestTag, latestTagDate, hasReleases, err := getLatestTag(repoPath)
	if err != nil {
		fmt.Printf("Warning: Failed to get latest tag: %v\n", err)
	}
	metadata.LatestTag = latestTag
	metadata.LatestTagDate = latestTagDate
	metadata.HasReleases = hasReleases

	return metadata, nil
}

// getCommitCount returns the total number of commits
func getCommitCount(repoPath string) (int, error) {
	cmd := exec.Command("git", "-C", repoPath, "rev-list", "--all", "--count")
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

// countLinesOfCode counts lines of code (excluding binary files and common non-code files)
func countLinesOfCode(repoPath string) (int, error) {
	// Use git ls-files to get tracked files, then count lines
	// Exclude binary files and common non-code files
	cmd := exec.Command("bash", "-c", fmt.Sprintf(
		`cd "%s" && git ls-files | grep -E '\.(go|py|js|ts|java|c|cpp|h|hpp|cs|rb|php|swift|kt|rs|scala|r|sh|bash|zsh|pl|lua|vim|el|clj|hs|ml|ex|exs|dart|jl|nim|v|zig|html|css|scss|sass|json|xml|yaml|yml|toml|ini|conf|cfg)$' | xargs wc -l 2>/dev/null | tail -n 1 | awk '{print $1}'`,
		repoPath,
	))

	output, err := cmd.Output()
	if err != nil {
		// Fallback: try a simpler approach
		cmd = exec.Command("bash", "-c", fmt.Sprintf(
			`find "%s" -type f -name "*.go" -o -name "*.py" -o -name "*.js" -o -name "*.java" -o -name "*.c" -o -name "*.cpp" -o -name "*.rs" | xargs wc -l 2>/dev/null | tail -n 1 | awk '{print $1}'`,
			repoPath,
		))
		output, err = cmd.Output()
		if err != nil {
			return 0, err
		}
	}

	loc, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return 0, err
	}

	return loc, nil
}

// getFirstCommitDate returns the date of the first commit
func getFirstCommitDate(repoPath string) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "log", "--reverse", "--pretty=format:%ai", "--date=short")
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
	cmd := exec.Command("git", "-C", repoPath, "log", "-1", "--pretty=format:%ai", "--date=short")
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
	cmd := exec.Command("git", "-C", repoPath, "log", fmt.Sprintf("-%d", commitCount), "--pretty=format:%at")
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return 0, fmt.Errorf("no commits found")
	}

	// Calculate average age
	now := float64(time.Now().Unix())
	var totalAge float64
	validCommits := 0

	for _, line := range lines {
		if line == "" {
			continue
		}

		timestamp, err := strconv.ParseInt(line, 10, 64)
		if err != nil {
			continue
		}

		age := (now - float64(timestamp)) / 86400 // Convert to days
		totalAge += age
		validCommits++
	}

	if validCommits == 0 {
		return 0, fmt.Errorf("no valid commits found")
	}

	return totalAge / float64(validCommits), nil
}

// getLatestTag returns the latest git tag, its date, and whether the repo has any releases
func getLatestTag(repoPath string) (string, string, bool, error) {
	// First try to get tags sorted by version
	cmd := exec.Command("git", "-C", repoPath, "tag", "-l", "--sort=-version:refname")
	output, err := cmd.Output()
	if err != nil {
		// Fallback to describe
		cmd = exec.Command("git", "-C", repoPath, "describe", "--tags", "--abbrev=0")
		output, err = cmd.Output()
		if err != nil {
			// No tags at all
			return "", "", false, nil
		}
	}

	tags := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(tags) == 0 || tags[0] == "" {
		return "", "", false, nil
	}

	// Find the first tag that looks like a version number
	latestTag := ""
	for _, tag := range tags {
		if isVersionTag(tag) {
			latestTag = tag
			break
		}
	}

	if latestTag == "" {
		// No version-like tags found
		return "", "", false, nil
	}

	// Get the date of the latest tag
	cmd = exec.Command("git", "-C", repoPath, "log", "-1", "--format=%ai", latestTag)
	dateOutput, err := cmd.Output()
	if err != nil {
		// Tag exists but couldn't get date
		return latestTag, "", true, nil
	}

	// Extract just the date part (YYYY-MM-DD)
	parts := strings.Fields(string(dateOutput))
	tagDate := ""
	if len(parts) > 0 {
		tagDate = parts[0]
	}

	// Return the latest tag and its date
	return latestTag, tagDate, true, nil
}

// isVersionTag checks if a tag looks like a version number
func isVersionTag(tag string) bool {
	// Remove 'v' prefix if present
	versionStr := strings.TrimPrefix(tag, "v")

	// Check if the remaining string contains at least one digit and one dot
	hasDigit := false
	hasDot := false

	for _, ch := range versionStr {
		if ch >= '0' && ch <= '9' {
			hasDigit = true
		} else if ch == '.' {
			hasDot = true
		} else if ch != '-' && ch != '+' && ch != '_' &&
			(ch < 'a' || ch > 'z') && (ch < 'A' || ch > 'Z') {
			// Allow alphanumeric characters and common separators
			// but anything else makes it not a version
			return false
		}
	}

	// Must have at least one digit, and either:
	// - have a dot (e.g., 1.0, 0.1.2)
	// - be just digits (e.g., 2, 2024)
	// - start with a digit (e.g., 1-beta)
	if hasDigit && len(versionStr) > 0 {
		firstChar := versionStr[0]
		if firstChar >= '0' && firstChar <= '9' {
			return true
		}
	}

	return hasDigit && hasDot
}
