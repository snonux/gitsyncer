package release

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

// Tag represents a git tag
type Tag struct {
	Name string
}

// Release represents a release on a platform
type Release struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Body    string `json:"body"`
}

// Manager handles release operations
type Manager struct {
	workDir       string
	githubToken   string
	codebergToken string
}

// NewManager creates a new release manager
func NewManager(workDir string) *Manager {
	return &Manager{
		workDir: workDir,
	}
}

// SetGitHubToken sets the GitHub token for API authentication
func (m *Manager) SetGitHubToken(token string) {
	m.githubToken = token
}

// SetCodebergToken sets the Codeberg token for API authentication
func (m *Manager) SetCodebergToken(token string) {
	m.codebergToken = token
}

// isVersionTag checks if a tag name is a version tag
// Supports formats: vX.Y.Z, vX.Y, vX, X.Y.Z, X.Y, X
func isVersionTag(tag string) bool {
	// Pattern matches version tags with optional 'v' prefix
	pattern := `^v?\d+(\.\d+)?(\.\d+)?$`
	matched, _ := regexp.MatchString(pattern, tag)
	return matched
}

// GetLocalTags returns all version tags from the local git repository
func (m *Manager) GetLocalTags(repoPath string) ([]string, error) {
	cmd := exec.Command("git", "-C", repoPath, "tag", "--list")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get git tags: %w", err)
	}

	var versionTags []string
	tags := strings.Split(strings.TrimSpace(string(output)), "\n")
	
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag != "" && isVersionTag(tag) {
			versionTags = append(versionTags, tag)
		}
	}

	// Sort tags by version
	sort.Slice(versionTags, func(i, j int) bool {
		return compareVersions(versionTags[i], versionTags[j]) < 0
	})

	return versionTags, nil
}

// compareVersions compares two version strings
// Returns -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2
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
			fmt.Sscanf(parts1[i], "%d", &n1)
		}
		if i < len(parts2) {
			fmt.Sscanf(parts2[i], "%d", &n2)
		}

		if n1 < n2 {
			return -1
		} else if n1 > n2 {
			return 1
		}
	}

	return 0
}

// GetCommitsSinceTag gets all commits since a specific tag
func (m *Manager) GetCommitsSinceTag(repoPath, fromTag, toTag string) ([]string, error) {
	// Use git log to get commits between tags
	// If fromTag is empty, get all commits up to toTag
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

// GenerateReleaseNotes generates release notes from commits
func (m *Manager) GenerateReleaseNotes(repoPath, tag string, allTags []string) string {
	// Find the previous tag
	var prevTag string
	tagIndex := -1
	for i, t := range allTags {
		if t == tag {
			tagIndex = i
			break
		}
	}
	
	if tagIndex > 0 {
		prevTag = allTags[tagIndex-1]
	}
	
	// Get commits since previous tag
	commits, err := m.GetCommitsSinceTag(repoPath, prevTag, tag)
	if err != nil {
		return fmt.Sprintf("Release %s", tag)
	}
	
	if len(commits) == 0 {
		return fmt.Sprintf("Release %s", tag)
	}
	
	// Group commits by type
	var features, fixes, other []string
	
	for _, commit := range commits {
		lower := strings.ToLower(commit)
		if strings.HasPrefix(lower, "feat:") || strings.HasPrefix(lower, "feature:") {
			features = append(features, commit)
		} else if strings.HasPrefix(lower, "fix:") || strings.HasPrefix(lower, "bugfix:") {
			fixes = append(fixes, commit)
		} else {
			other = append(other, commit)
		}
	}
	
	// Build release notes
	var notes strings.Builder
	notes.WriteString(fmt.Sprintf("Release %s\n\n", tag))
	
	if prevTag != "" {
		notes.WriteString(fmt.Sprintf("Changes since %s:\n\n", prevTag))
	}
	
	if len(features) > 0 {
		notes.WriteString("## New Features\n\n")
		for _, feat := range features {
			notes.WriteString(fmt.Sprintf("- %s\n", feat))
		}
		notes.WriteString("\n")
	}
	
	if len(fixes) > 0 {
		notes.WriteString("## Bug Fixes\n\n")
		for _, fix := range fixes {
			notes.WriteString(fmt.Sprintf("- %s\n", fix))
		}
		notes.WriteString("\n")
	}
	
	if len(other) > 0 {
		notes.WriteString("## Other Changes\n\n")
		for _, commit := range other {
			notes.WriteString(fmt.Sprintf("- %s\n", commit))
		}
		notes.WriteString("\n")
	}
	
	notes.WriteString(fmt.Sprintf("\n**Full Changelog**: %s...%s", prevTag, tag))
	
	return notes.String()
}

// GetDiffBetweenTags gets the diff between two tags
func (m *Manager) GetDiffBetweenTags(repoPath, fromTag, toTag string) (string, error) {
	// Use git diff to get changes between tags
	// If fromTag is empty, get all changes up to toTag
	var cmd *exec.Cmd
	if fromTag == "" {
		// Get diff from the beginning to toTag
		cmd = exec.Command("git", "-C", repoPath, "show", "--format=", "--no-patch", toTag)
		// This won't work well, so let's get the first commit
		firstCommitCmd := exec.Command("git", "-C", repoPath, "rev-list", "--max-parents=0", "HEAD")
		firstCommitOutput, err := firstCommitCmd.Output()
		if err != nil {
			// Fallback to just showing the tag
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

// GenerateAIReleaseNotes generates prose release notes using Claude CLI
func (m *Manager) GenerateAIReleaseNotes(repoPath, repoName, tag string, allTags []string, commits []string) (string, error) {
	// Find the previous tag
	var prevTag string
	tagIndex := -1
	for i, t := range allTags {
		if t == tag {
			tagIndex = i
			break
		}
	}
	
	if tagIndex > 0 {
		prevTag = allTags[tagIndex-1]
	}
	
	// Get the diff between tags
	diff, err := m.GetDiffBetweenTags(repoPath, prevTag, tag)
	if err != nil {
		return "", fmt.Errorf("failed to get diff: %w", err)
	}
	
	// Prepare the prompt for Claude
	var prompt strings.Builder
	prompt.WriteString(fmt.Sprintf("Generate professional release notes for %s version %s.\n\n", repoName, tag))
	
	if prevTag != "" {
		prompt.WriteString(fmt.Sprintf("Previous version: %s\n", prevTag))
	}
	
	prompt.WriteString("\nCommit messages:\n")
	for _, commit := range commits {
		prompt.WriteString(fmt.Sprintf("- %s\n", commit))
	}
	
	prompt.WriteString("\nCode changes:\n")
	prompt.WriteString(diff)
	prompt.WriteString("\n\nBased on the commits and code changes above, write professional release notes that:\n")
	prompt.WriteString("1. Start with a brief overview of what this release accomplishes\n")
	prompt.WriteString("2. Group changes into logical sections (Features, Improvements, Bug Fixes, etc.)\n")
	prompt.WriteString("3. Explain WHY each change is useful to users, not just what changed\n")
	prompt.WriteString("4. Use clear, non-technical language where possible\n")
	prompt.WriteString("5. Highlight any breaking changes or migration steps\n")
	prompt.WriteString("6. Keep it concise but informative\n")
	prompt.WriteString("7. Format using Markdown\n")
	prompt.WriteString("\nDo not include the version number in the title as it will be added automatically.")
	
	// Run Claude CLI
	fmt.Println("  Running Claude CLI command:")
	fmt.Println("  claude --model sonnet \"...\"")
	fmt.Printf("  Prompt length: %d characters\n", len(prompt.String()))
	fmt.Println("  Prompt preview (first 500 chars):")
	promptStr := prompt.String()
	if len(promptStr) > 500 {
		fmt.Printf("  %s...\n", promptStr[:500])
	} else {
		fmt.Printf("  %s\n", promptStr)
	}
	
	cmd := exec.Command("claude", "--model", "sonnet", prompt.String())
	output, err := cmd.Output()
	if err != nil {
		// Try with opus model
		fmt.Println("  Trying with opus model...")
		cmd = exec.Command("claude", "--model", "opus", prompt.String())
		output, err = cmd.Output()
		if err != nil {
			// Try with default model
			fmt.Println("  Trying with default model...")
			cmd = exec.Command("claude", prompt.String())
			output, err = cmd.Output()
			if err != nil {
				return "", fmt.Errorf("failed to run claude: %w", err)
			}
		}
	}
	
	releaseNotes := strings.TrimSpace(string(output))
	if releaseNotes == "" {
		return "", fmt.Errorf("received empty release notes from claude")
	}
	
	// Add header and footer
	var finalNotes strings.Builder
	finalNotes.WriteString(fmt.Sprintf("# Release %s\n\n", tag))
	finalNotes.WriteString(releaseNotes)
	finalNotes.WriteString("\n\n---\n\n")
	if prevTag != "" {
		finalNotes.WriteString(fmt.Sprintf("**Full Changelog**: %s...%s", prevTag, tag))
	}
	
	return finalNotes.String(), nil
}

// GetGitHubReleases fetches releases from GitHub
func (m *Manager) GetGitHubReleases(owner, repo string) ([]string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases", owner, repo)
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Add GitHub token if available
	if m.githubToken != "" {
		req.Header.Set("Authorization", "Bearer "+m.githubToken)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		// Repository might not exist on GitHub
		return []string{}, nil
	}

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error: %s - %s", resp.Status, string(body))
	}

	var releases []Release
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}

	var tags []string
	for _, release := range releases {
		tags = append(tags, release.TagName)
	}

	return tags, nil
}

// GetCodebergReleases fetches releases from Codeberg
func (m *Manager) GetCodebergReleases(owner, repo string) ([]string, error) {
	url := fmt.Sprintf("https://codeberg.org/api/v1/repos/%s/%s/releases", owner, repo)
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Add Codeberg token if available
	if m.codebergToken != "" {
		req.Header.Set("Authorization", "token "+m.codebergToken)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		// Repository might not exist on Codeberg
		return []string{}, nil
	}

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Codeberg API error: %s - %s", resp.Status, string(body))
	}

	var releases []Release
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}

	var tags []string
	for _, release := range releases {
		tags = append(tags, release.TagName)
	}

	return tags, nil
}

// FindMissingReleases finds tags that don't have releases
func (m *Manager) FindMissingReleases(localTags, releaseTags []string) []string {
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

// CreateGitHubRelease creates a release on GitHub
func (m *Manager) CreateGitHubRelease(owner, repo, tag, releaseNotes string) error {
	if m.githubToken == "" {
		return fmt.Errorf("GitHub token is required for creating releases")
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases", owner, repo)
	
	// Use provided release notes or default
	body := releaseNotes
	if body == "" {
		body = fmt.Sprintf("Release %s", tag)
	}
	
	release := Release{
		TagName: tag,
		Name:    tag,
		Body:    body,
	}

	jsonData, err := json.Marshal(release)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+m.githubToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create GitHub release: %s - %s", resp.Status, string(body))
	}

	return nil
}

// CreateCodebergRelease creates a release on Codeberg
func (m *Manager) CreateCodebergRelease(owner, repo, tag, releaseNotes string) error {
	if m.codebergToken == "" {
		return fmt.Errorf("Codeberg token is required for creating releases")
	}

	url := fmt.Sprintf("https://codeberg.org/api/v1/repos/%s/%s/releases", owner, repo)
	
	// Use provided release notes or default
	body := releaseNotes
	if body == "" {
		body = fmt.Sprintf("Release %s", tag)
	}
	
	release := Release{
		TagName: tag,
		Name:    tag,
		Body:    body,
	}

	jsonData, err := json.Marshal(release)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "token "+m.codebergToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create Codeberg release: %s - %s", resp.Status, string(body))
	}

	return nil
}

// PromptConfirmation asks for user confirmation
func PromptConfirmation(message string) bool {
	fmt.Printf("%s [y/N]: ", message)
	
	var response string
	fmt.Scanln(&response)
	
	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}

// PromptConfirmationWithNotes asks for user confirmation and shows release notes
func PromptConfirmationWithNotes(message, releaseNotes string) bool {
	fmt.Printf("\n%s\n", strings.Repeat("-", 70))
	fmt.Printf("Release Notes:\n%s\n", strings.Repeat("-", 70))
	fmt.Println(releaseNotes)
	fmt.Printf("%s\n\n", strings.Repeat("-", 70))
	
	fmt.Printf("%s [y/N]: ", message)
	
	var response string
	fmt.Scanln(&response)
	
	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}

// UpdateGitHubRelease updates an existing release on GitHub
func (m *Manager) UpdateGitHubRelease(owner, repo, tag, releaseNotes string) error {
	if m.githubToken == "" {
		return fmt.Errorf("GitHub token is required for updating releases")
	}

	// First, get the release ID
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/tags/%s", owner, repo, tag)
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+m.githubToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to get release: %s - %s", resp.Status, string(body))
	}

	var releaseInfo struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&releaseInfo); err != nil {
		return err
	}

	// Now update the release
	updateURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/%d", owner, repo, releaseInfo.ID)
	
	release := Release{
		TagName: tag,
		Name:    tag,
		Body:    releaseNotes,
	}

	jsonData, err := json.Marshal(release)
	if err != nil {
		return err
	}

	updateReq, err := http.NewRequest("PATCH", updateURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	updateReq.Header.Set("Authorization", "Bearer "+m.githubToken)
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.Header.Set("Accept", "application/vnd.github.v3+json")

	updateResp, err := client.Do(updateReq)
	if err != nil {
		return err
	}
	defer updateResp.Body.Close()

	if updateResp.StatusCode != 200 {
		body, _ := io.ReadAll(updateResp.Body)
		return fmt.Errorf("failed to update GitHub release: %s - %s", updateResp.Status, string(body))
	}

	return nil
}

// UpdateCodebergRelease updates an existing release on Codeberg
func (m *Manager) UpdateCodebergRelease(owner, repo, tag, releaseNotes string) error {
	if m.codebergToken == "" {
		return fmt.Errorf("Codeberg token is required for updating releases")
	}

	// First, get the release ID
	url := fmt.Sprintf("https://codeberg.org/api/v1/repos/%s/%s/releases/tags/%s", owner, repo, tag)
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "token "+m.codebergToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to get release: %s - %s", resp.Status, string(body))
	}

	var releaseInfo struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&releaseInfo); err != nil {
		return err
	}

	// Now update the release
	updateURL := fmt.Sprintf("https://codeberg.org/api/v1/repos/%s/%s/releases/%d", owner, repo, releaseInfo.ID)
	
	release := Release{
		TagName: tag,
		Name:    tag,
		Body:    releaseNotes,
	}

	jsonData, err := json.Marshal(release)
	if err != nil {
		return err
	}

	updateReq, err := http.NewRequest("PATCH", updateURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	updateReq.Header.Set("Authorization", "token "+m.codebergToken)
	updateReq.Header.Set("Content-Type", "application/json")

	updateResp, err := client.Do(updateReq)
	if err != nil {
		return err
	}
	defer updateResp.Body.Close()

	if updateResp.StatusCode != 200 {
		body, _ := io.ReadAll(updateResp.Body)
		return fmt.Errorf("failed to update Codeberg release: %s - %s", updateResp.Status, string(body))
	}

	return nil
}