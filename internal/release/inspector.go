package release

import (
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"codeberg.org/snonux/gitsyncer/internal/version"
)

// GitInspector reads git state from a local working clone: version tags,
// commit ranges between tags, and diffs. It owns no network access and no
// release CRUD, which now live in the per-forge clients.
type GitInspector struct{}

// NewGitInspector creates a stateless git inspector. The methods all take an
// explicit repoPath, so the inspector carries no per-repository state.
func NewGitInspector() *GitInspector {
	return &GitInspector{}
}

// GetLocalTags returns all version tags from the local git repository, sorted
// in ascending version order.
func (g *GitInspector) GetLocalTags(repoPath string) ([]string, error) {
	cmd := exec.Command("git", "-C", repoPath, "tag", "--list")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get git tags: %w", err)
	}

	var versionTags []string
	tags := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag != "" && version.IsVersionTag(tag) {
			versionTags = append(versionTags, tag)
		}
	}

	// Sort tags by version
	sort.Slice(versionTags, func(i, j int) bool {
		return compareVersions(versionTags[i], versionTags[j]) < 0
	})

	return versionTags, nil
}

// GetCommitsSinceTag gets all commit subjects between fromTag (exclusive) and
// toTag (inclusive), returned oldest first. If fromTag is empty, all commits
// up to toTag are returned.
func (g *GitInspector) GetCommitsSinceTag(repoPath, fromTag, toTag string) ([]string, error) {
	var cmd *exec.Cmd
	if fromTag == "" {
		cmd = exec.Command("git", "-C", repoPath, "log", "--pretty=format:%s", toTag)
	} else {
		cmd = exec.Command("git", "-C", repoPath, "log", "--pretty=format:%s", fmt.Sprintf("%s..%s", fromTag, toTag))
	}

	output, err := cmd.Output()
	if err != nil {
		// If error, it might be because fromTag doesn't exist, try without it
		cmd = exec.Command("git", "-C", repoPath, "log", "--pretty=format:%s", toTag)
		output, err = cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("failed to get commits: %w", err)
		}
	}

	if len(output) == 0 {
		return []string{}, nil
	}

	commits := strings.Split(strings.TrimSpace(string(output)), "\n")
	// Reverse to show oldest first
	for i, j := 0, len(commits)-1; i < j; i, j = i+1, j-1 {
		commits[i], commits[j] = commits[j], commits[i]
	}

	return commits, nil
}

// GetDiffBetweenTags gets the diff (stat plus a truncated code diff for key
// source/readme files) between fromTag (exclusive) and toTag (inclusive). If
// fromTag is empty, the diff runs from the root commit to toTag.
func (g *GitInspector) GetDiffBetweenTags(repoPath, fromTag, toTag string) (string, error) {
	var cmd *exec.Cmd
	if fromTag == "" {
		// Diff from the root commit up to toTag. Resolve the first commit so the
		// range is explicit; fall back to a single-tag stat if that lookup fails.
		firstCommitCmd := exec.Command("git", "-C", repoPath, "rev-list", "--max-parents=0", "HEAD")
		firstCommitOutput, err := firstCommitCmd.Output()
		if err != nil {
			cmd = exec.Command("git", "-C", repoPath, "diff", "--stat", toTag)
		} else {
			firstCommit := strings.TrimSpace(string(firstCommitOutput))
			cmd = exec.Command("git", "-C", repoPath, "diff", "--stat", fmt.Sprintf("%s..%s", firstCommit, toTag))
		}
	} else {
		cmd = exec.Command("git", "-C", repoPath, "diff", "--stat", fmt.Sprintf("%s..%s", fromTag, toTag))
	}

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get diff: %w", err)
	}

	// Also get the actual diff for key files (limit to prevent huge outputs)
	var diffCmd *exec.Cmd
	if fromTag == "" {
		diffCmd = exec.Command("git", "-C", repoPath, "show", toTag, "--", "*.go", "*.js", "*.py", "*.rs", "*.c", "*.cpp", "*.java", "*.ts", "*.jsx", "*.tsx", "README*", "*.md")
	} else {
		diffCmd = exec.Command("git", "-C", repoPath, "diff", fmt.Sprintf("%s..%s", fromTag, toTag), "--", "*.go", "*.js", "*.py", "*.rs", "*.c", "*.cpp", "*.java", "*.ts", "*.jsx", "*.tsx", "README*", "*.md")
	}

	diffOutput, err := diffCmd.Output()
	if err != nil {
		// If error, just use the stat output
		return string(output), nil
	}

	// Combine stat and limited diff (truncate if too long)
	fullOutput := string(output) + "\n\n" + string(diffOutput)
	maxLength := 50000 // Limit to 50KB to avoid overwhelming Claude
	if len(fullOutput) > maxLength {
		fullOutput = fullOutput[:maxLength] + "\n\n... (diff truncated)"
	}

	return fullOutput, nil
}

// FindMissingReleases returns the local tags that have no corresponding
// release, preserving the local tag order.
func (g *GitInspector) FindMissingReleases(localTags, releaseTags []string) []string {
	releaseMap := make(map[string]bool)
	for _, tag := range releaseTags {
		releaseMap[tag] = true
	}

	var missing []string
	for _, tag := range localTags {
		if !releaseMap[tag] {
			missing = append(missing, tag)
		}
	}

	return missing
}

// compareVersions compares two version strings.
// Returns -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2.
func compareVersions(v1, v2 string) int {
	// Remove 'v' prefix if present
	v1 = strings.TrimPrefix(v1, "v")
	v2 = strings.TrimPrefix(v2, "v")

	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	// Pad with zeros to make equal length
	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var n1, n2 int

		if i < len(parts1) {
			n1 = parseVersionPart(parts1[i])
		}
		if i < len(parts2) {
			n2 = parseVersionPart(parts2[i])
		}

		if n1 < n2 {
			return -1
		} else if n1 > n2 {
			return 1
		}
	}

	return 0
}

func parseVersionPart(part string) int {
	start := 0
	if part != "" && (part[0] == '+' || part[0] == '-') {
		start = 1
	}

	end := start
	for end < len(part) && unicode.IsDigit(rune(part[end])) {
		end++
	}
	if end == start {
		return 0
	}

	n, err := strconv.Atoi(part[:end])
	if err != nil {
		return 0
	}

	return n
}
