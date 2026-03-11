package showcase

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"codeberg.org/snonux/gitsyncer/internal/config"
)

// Generator handles showcase generation for repositories
type Generator struct {
	config  *config.Config
	workDir string
	aiTool  string
}

// ProjectSummary holds the summary information for a project
type ProjectSummary struct {
	Name         string
	Summary      string
	CodebergURL  string
	GitHubURL    string
	Metadata     *RepoMetadata
	RankHistory  []RepoRankHistory // Latest 5 weekly rank points, newest first
	Images       []string          // Relative paths to images in showcase directory
	CodeSnippet  string            // Code snippet to show when no images
	CodeLanguage string            // Language and file info for the snippet
}

// LegacyRepoMetadata for backwards compatibility with old cache files
type LegacyRepoMetadata struct {
	Languages       []string
	CommitCount     int
	LinesOfCode     int
	FirstCommitDate string
	LastCommitDate  string
	License         string
	AvgCommitAge    float64
}

// New creates a new showcase generator
func New(cfg *config.Config, workDir string) *Generator {
	return &Generator{
		config:  cfg,
		workDir: workDir,
		aiTool:  "amp", // default to amp
	}
}

// SetAITool sets the AI tool to use for generating summaries
func (g *Generator) SetAITool(tool string) {
	g.aiTool = tool
}

// GenerateShowcase generates a showcase for repositories
// If repoFilter is provided, only those repositories are processed
// If repoFilter is empty/nil, all repositories in work directory are processed
func (g *Generator) GenerateShowcase(repoFilter []string, forceRegenerate bool) error {
	var repos []string
	var err error

	if len(repoFilter) > 0 {
		// Use the provided filter
		repos = repoFilter
	} else {
		// Get all repositories in work directory
		repos, err = g.getRepositories()
		if err != nil {
			return fmt.Errorf("failed to get repositories: %w", err)
		}
	}

	if len(repos) == 0 {
		return fmt.Errorf("no repositories found")
	}

	// Filter out excluded repositories
	filteredRepos := g.filterExcludedRepos(repos)

	fmt.Printf("Found %d repositories to process (after filtering %d excluded)\n",
		len(filteredRepos), len(repos)-len(filteredRepos))

	// Generate summaries for each repository
	summaries := make([]ProjectSummary, 0, len(filteredRepos))
	successCount := 0

	for i, repo := range filteredRepos {
		fmt.Printf("\n[%d/%d] Processing %s...\n", i+1, len(filteredRepos), repo)

		summary, err := g.generateProjectSummary(repo, forceRegenerate)
		if err != nil {
			fmt.Printf("WARNING: Failed to generate summary for %s: %v\n", repo, err)
			continue
		}

		// Print the generated summary to stdout
		fmt.Printf("\n--- Generated summary for %s ---\n", repo)
		fmt.Println(summary.Summary)
		if summary.Metadata != nil {
			fmt.Printf("Languages: %s\n", FormatLanguagesWithPercentages(summary.Metadata.Languages))
			fmt.Printf("Commits: %d\n", summary.Metadata.CommitCount)
			fmt.Printf("Lines of Code: %d\n", summary.Metadata.LinesOfCode)
			fmt.Printf("First Commit: %s\n", summary.Metadata.FirstCommitDate)
			fmt.Printf("Last Commit: %s\n", summary.Metadata.LastCommitDate)
			fmt.Printf("License: %s\n", summary.Metadata.License)
			fmt.Printf("Tags: %d\n", summary.Metadata.TagCount)
			fmt.Printf("Score: %.1f\n", summary.Metadata.Score)
		}
		fmt.Println("--- End of summary ---")

		summaries = append(summaries, *summary)
		successCount++
	}

	if successCount == 0 {
		return fmt.Errorf("failed to generate any summaries")
	}

	fmt.Printf("\nSuccessfully generated %d/%d summaries\n", successCount, len(repos))

	// Sort summaries by score (highest first)
	sort.Slice(summaries, func(i, j int) bool {
		// If metadata is missing, put at the end
		if summaries[i].Metadata == nil {
			return false
		}
		if summaries[j].Metadata == nil {
			return true
		}
		// Higher score is better (combines LOC and recent activity)
		return summaries[i].Metadata.Score > summaries[j].Metadata.Score
	})

	anchorDate := time.Now()
	rankHistoryFile := filepath.Join(g.workDir, rankHistoryFilename)
	rankHistoryStore, err := loadRankHistory(rankHistoryFile)
	if err != nil {
		return fmt.Errorf("failed to load rank history: %w", err)
	}

	// Only full showcase runs should update ranking snapshots.
	if len(repoFilter) == 0 {
		upsertSnapshotForDate(rankHistoryStore, anchorDate, buildCurrentRanks(summaries))
		if err := saveRankHistory(rankHistoryFile, rankHistoryStore); err != nil {
			return fmt.Errorf("failed to save rank history: %w", err)
		}
	}

	applyRankHistoryToSummaries(summaries, rankHistoryStore, anchorDate, rankHistoryPoints)

	// When filtering (single repo), we need to update existing showcase
	if len(repoFilter) > 0 {
		if err := g.updateShowcaseFile(summaries); err != nil {
			return fmt.Errorf("failed to update showcase file: %w", err)
		}
	} else {
		// Full regeneration - format as Gemtext and write
		content := g.formatGemtext(summaries)
		if err := g.writeShowcaseFile(content); err != nil {
			return fmt.Errorf("failed to write showcase file: %w", err)
		}
	}

	return nil
}

// runCommandWithTimeout runs a command with a short timeout and returns trimmed stdout.
// Stderr is included in the error message for easier debugging when GITSYNCER_DEBUG=1.
func runCommandWithTimeout(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("command timed out")
	}
	if err != nil {
		// include a snippet of output for debugging
		msg := strings.TrimSpace(string(out))
		if len(msg) > 300 {
			msg = msg[:300] + "..."
		}
		if msg != "" {
			return "", fmt.Errorf("%v: %s", err, msg)
		}
		return "", err
	}
	return string(out), nil
}

func findReadmeContent(repoPath string) ([]byte, string, bool) {
	readmeFiles := []string{
		"README.md", "readme.md", "Readme.md",
		"README.MD", "README.txt", "readme.txt",
		"README", "readme",
	}

	for _, readmeFile := range readmeFiles {
		path := filepath.Join(repoPath, readmeFile)
		content, err := os.ReadFile(path)
		if err == nil {
			return content, readmeFile, true
		}
	}

	return nil, "", false
}

func selectSummaryTool(aiTool string) string {
	switch aiTool {
	case "amp", "":
		if _, err := exec.LookPath("amp"); err == nil {
			return "amp"
		}
		if _, err := exec.LookPath("hexai"); err == nil {
			return "hexai"
		}
		if _, err := exec.LookPath("claude"); err == nil {
			return "claude"
		}
		if _, err := exec.LookPath("aichat"); err == nil {
			return "aichat"
		}
	case "claude", "claude-code":
		if _, err := exec.LookPath("claude"); err == nil {
			return "claude"
		}
		if _, err := exec.LookPath("hexai"); err == nil {
			return "hexai"
		}
		if _, err := exec.LookPath("aichat"); err == nil {
			return "aichat"
		}
	case "hexai", "aichat":
		if _, err := exec.LookPath(aiTool); err == nil {
			return aiTool
		}
	}

	return ""
}

func runSummaryTool(selectedTool, prompt, repoPath, readmeFile string, readmeContent []byte, readmeFound bool) string {
	var cmd *exec.Cmd

	switch selectedTool {
	case "amp":
		fmt.Printf("Running amp command (stdin payload)\n")
		if readmeFound {
			fmt.Printf("  echo <README content> | amp --execute \"%s\"\n", prompt)
			fmt.Printf("  Using %s as input\n", readmeFile)
			cmd = exec.Command("amp", "--execute", prompt)
			cmd.Stdin = strings.NewReader(string(readmeContent))
		}
	case "claude":
		fmt.Printf("Running Claude command:\n")
		fmt.Printf("  claude --model sonnet \"%s\"\n", prompt)
		cmd = exec.Command("claude", "--model", "sonnet", prompt)
	case "hexai":
		fmt.Printf("Running hexai command (stdin payload)\n")
		if readmeFound {
			fmt.Printf("  echo <README content> | hexai \"%s\"\n", prompt)
			fmt.Printf("  Using %s as input\n", readmeFile)
			cmd = exec.Command("hexai", prompt)
			cmd.Stdin = strings.NewReader(string(readmeContent))
		}
	case "aichat":
		fmt.Printf("Running aichat command:\n")
		if readmeFound {
			fmt.Printf("  echo <README content> | aichat \"%s\"\n", prompt)
			fmt.Printf("  Using %s as input\n", readmeFile)
			cmd = exec.Command("aichat", prompt)
			cmd.Stdin = strings.NewReader(string(readmeContent))
		}
	}

	if cmd == nil {
		return ""
	}

	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(output))
}

func fallbackSummary(repoName string, readmeContent []byte, readmeFound bool) string {
	if readmeFound {
		if summary := extractUsefulSummary(string(readmeContent), 1); summary != "" {
			return summary
		}
	}

	return fmt.Sprintf("%s: source code repository.", repoName)
}

func extractUsefulSummary(text string, maxParagraphs int) string {
	if maxParagraphs <= 0 {
		maxParagraphs = 1
	}

	parts := splitSummaryParagraphs(text)
	useful := make([]string, 0, maxParagraphs)

	for _, part := range parts {
		part = normalizeSummaryParagraph(part)
		if part == "" {
			continue
		}

		useful = append(useful, part)
		if len(useful) >= maxParagraphs {
			break
		}
	}

	return strings.Join(useful, "\n\n")
}

func normalizeSummaryParagraph(paragraph string) string {
	rawParagraph := strings.TrimSpace(paragraph)
	switch {
	case isHeadingOnlyParagraph(rawParagraph):
		return ""
	case isImageOnlyParagraph(rawParagraph):
		return ""
	case isHTMLOnlyParagraph(rawParagraph):
		return ""
	case isTOCParagraph(rawParagraph):
		return ""
	case isListOnlyParagraph(rawParagraph):
		return ""
	case isBadgeParagraph(rawParagraph):
		return ""
	}

	paragraph = sanitizeSummaryForGemtext(paragraph)
	if paragraph == "" {
		return ""
	}

	if normalized, ok := normalizeManpageParagraph(paragraph); ok {
		paragraph = normalized
	}

	if isLabelOnlyParagraph(paragraph) {
		return ""
	}

	return paragraph
}

func splitSummaryParagraphs(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	rawParts := strings.Split(text, "\n\n")
	parts := make([]string, 0, len(rawParts))
	for _, part := range rawParts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		parts = append(parts, part)
	}

	return parts
}

func sanitizeSummaryForGemtext(summary string) string {
	summary = strings.ReplaceAll(summary, "\r\n", "\n")
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return ""
	}

	lines := strings.Split(summary, "\n")
	cleaned := make([]string, 0, len(lines))
	inCodeFence := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if inCodeFence {
				continue
			}
			cleaned = append(cleaned, "")
			continue
		}

		if strings.HasPrefix(trimmed, "```") {
			inCodeFence = !inCodeFence
			continue
		}
		if inCodeFence {
			continue
		}

		if isHTMLOnlyLine(trimmed) || isMarkdownImageLine(trimmed) {
			continue
		}

		if isSetextUnderline(trimmed) && len(cleaned) > 0 && strings.TrimSpace(cleaned[len(cleaned)-1]) != "" {
			continue
		}

		if heading, ok := trimMarkdownHeading(trimmed); ok {
			if heading != "" {
				cleaned = append(cleaned, heading)
			}
			continue
		}

		cleaned = append(cleaned, strings.TrimRight(line, " \t"))
	}

	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

func isHeadingOnlyParagraph(paragraph string) bool {
	lines := strings.Split(strings.TrimSpace(strings.ReplaceAll(paragraph, "\r\n", "\n")), "\n")
	if len(lines) == 1 {
		_, ok := trimMarkdownHeading(strings.TrimSpace(lines[0]))
		return ok
	}
	if len(lines) == 2 {
		return strings.TrimSpace(lines[0]) != "" && isSetextUnderline(strings.TrimSpace(lines[1]))
	}
	return false
}

func isImageOnlyParagraph(paragraph string) bool {
	trimmed := strings.TrimSpace(paragraph)
	if trimmed == "" || strings.Contains(trimmed, "\n") {
		return false
	}
	return strings.HasPrefix(trimmed, "<img") || strings.HasPrefix(trimmed, "![")
}

func isHTMLOnlyParagraph(paragraph string) bool {
	lines := strings.Split(strings.TrimSpace(paragraph), "\n")
	if len(lines) == 0 {
		return false
	}

	seen := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !isHTMLOnlyLine(trimmed) {
			return false
		}
		seen = true
	}

	return seen
}

func isHTMLOnlyLine(line string) bool {
	return strings.HasPrefix(line, "<") && strings.HasSuffix(line, ">")
}

func isMarkdownImageLine(line string) bool {
	return strings.HasPrefix(line, "![") && strings.Contains(line, "](")
}

func isTOCParagraph(paragraph string) bool {
	lines := strings.Split(strings.TrimSpace(paragraph), "\n")
	if len(lines) == 0 {
		return false
	}

	first := strings.TrimSpace(lines[0])
	if !strings.EqualFold(first, "toc:") && !strings.EqualFold(first, "table of contents:") {
		return false
	}

	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !isOrderedListLine(trimmed) {
			return false
		}
	}

	return true
}

func isListOnlyParagraph(paragraph string) bool {
	lines := strings.Split(strings.TrimSpace(paragraph), "\n")
	if len(lines) == 0 {
		return false
	}

	seen := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !isListLine(trimmed) {
			return false
		}
		seen = true
	}

	return seen
}

func isListLine(line string) bool {
	return strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "- ") || isOrderedListLine(line)
}

func isOrderedListLine(line string) bool {
	if line == "" || line[0] < '0' || line[0] > '9' {
		return false
	}

	i := 0
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	if i == 0 || i >= len(line) {
		return false
	}
	if (line[i] != '.' && line[i] != ')') || i+1 >= len(line) || line[i+1] != ' ' {
		return false
	}

	return true
}

func isLabelOnlyParagraph(paragraph string) bool {
	lines := strings.Split(strings.TrimSpace(paragraph), "\n")
	if len(lines) != 1 {
		return false
	}

	line := strings.TrimSpace(lines[0])
	if line == "" {
		return false
	}
	if strings.HasSuffix(line, ":") && len(strings.Fields(line)) <= 5 {
		return true
	}

	return line == strings.ToUpper(line) && len(strings.Fields(line)) <= 4
}

func isBadgeParagraph(paragraph string) bool {
	lines := strings.Split(strings.TrimSpace(paragraph), "\n")
	if len(lines) != 1 {
		return false
	}

	line := strings.TrimSpace(lines[0])
	if line == "" {
		return false
	}

	markerCount := strings.Count(line, "](") + strings.Count(line, "![")
	return markerCount >= 2
}

func normalizeManpageParagraph(paragraph string) (string, bool) {
	lines := strings.Split(strings.TrimSpace(paragraph), "\n")
	if len(lines) < 2 {
		return "", false
	}
	if strings.TrimSpace(lines[0]) != "NAME" {
		return "", false
	}

	body := make([]string, 0, len(lines)-1)
	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		body = append(body, trimmed)
	}
	if len(body) == 0 {
		return "", false
	}

	return strings.Join(body, " "), true
}

func trimMarkdownHeading(line string) (string, bool) {
	if line == "" || !strings.HasPrefix(line, "#") {
		return "", false
	}

	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level == 0 || level > 6 {
		return "", false
	}
	if level < len(line) && line[level] != ' ' && line[level] != '\t' {
		return "", false
	}

	heading := strings.TrimSpace(line[level:])
	heading = strings.TrimSpace(strings.TrimRight(heading, "#"))
	return heading, true
}

func isSetextUnderline(line string) bool {
	if len(line) < 3 {
		return false
	}
	return strings.Trim(line, "=") == "" || strings.Trim(line, "-") == ""
}

// getRepositories returns a list of repository directories in the work directory
func (g *Generator) getRepositories() ([]string, error) {
	entries, err := os.ReadDir(g.workDir)
	if err != nil {
		return nil, err
	}

	var repos []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Check if it's a git repository
		gitDir := filepath.Join(g.workDir, entry.Name(), ".git")
		if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
			repos = append(repos, entry.Name())
		}
	}

	// Sort repositories alphabetically
	sort.Strings(repos)
	return repos, nil
}

func (g *Generator) buildProjectLinks(repoName string) (string, string) {
	codebergURL := ""
	githubURL := ""

	if codebergOrg := g.config.FindCodebergOrg(); codebergOrg != nil {
		codebergURL = fmt.Sprintf("https://codeberg.org/%s/%s", codebergOrg.Name, repoName)
	}

	if githubOrg := g.config.FindGitHubOrg(); githubOrg != nil {
		githubURL = fmt.Sprintf("https://github.com/%s/%s", githubOrg.Name, repoName)
	}

	return codebergURL, githubURL
}

// generateProjectSummary generates a summary for a single project
func (g *Generator) generateProjectSummary(repoName string, forceRegenerate bool) (*ProjectSummary, error) {
	repoPath := filepath.Join(g.workDir, repoName)

	// Check cache first
	cacheDir := filepath.Join(g.workDir, ".gitsyncer-showcase-cache")
	cacheFile := filepath.Join(cacheDir, repoName+".json")

	// Try to load cached summary (but we'll still update metadata and images)
	var cachedSummary string
	var haveCachedSummary bool
	if !forceRegenerate {
		if cached, err := g.loadFromCache(cacheFile); err == nil {
			fmt.Printf("Using cached AI summary (cache file: %s)\n", cacheFile)
			cachedSummary = cached.Summary
			haveCachedSummary = true
		}
	}

	// Determine which AI tool to use (only if we need to run it)
	// Prefer amp if available when default tool is "" (aligns with release flow)
	selectedTool := g.aiTool
	if !haveCachedSummary {
		selectedTool = selectSummaryTool(g.aiTool)
	}

	readmeContent, readmeFile, readmeFound := findReadmeContent(repoPath)

	// Always extract metadata (not cached)
	fmt.Printf("Extracting repository metadata...\n")
	metadata, err := extractRepoMetadata(repoPath)
	if err != nil {
		fmt.Printf("Warning: Failed to extract some metadata: %v\n", err)
		// Continue anyway with partial metadata
	}

	// Get the summary - either from cache or by running AI tool
	var summary string
	if haveCachedSummary {
		summary = cachedSummary
		fmt.Printf("Using cached AI summary\n")
	} else {
		prompt := "Please provide a 1-2 paragraph summary of this project, explaining what it does, why it's useful, and how it's implemented. Focus on the key features and architecture. Be concise but informative."
		summary = runSummaryTool(selectedTool, prompt, repoPath, readmeFile, readmeContent, readmeFound)

		// Fallback: create a minimal summary from README if AI unavailable/failed
		if summary == "" {
			summary = fallbackSummary(repoName, readmeContent, readmeFound)
		}
	}
	summary = extractUsefulSummary(summary, 2)
	if summary == "" {
		summary = fallbackSummary(repoName, readmeContent, readmeFound)
	}
	summary = sanitizeSummaryForGemtext(summary)

	// Build URLs
	codebergURL, githubURL := g.buildProjectLinks(repoName)

	// Always extract images from README (not cached)
	fmt.Printf("Extracting images from README...\n")
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	showcaseDir := filepath.Join(home, "git", "foo.zone-content", "gemtext", "about")
	images, err := extractImagesFromRepo(repoPath, repoName, showcaseDir)
	if err != nil {
		fmt.Printf("Warning: Failed to extract images: %v\n", err)
		// Continue without images
	}

	// Extract code snippet for all projects
	var codeSnippet, codeLanguage string
	if metadata != nil && len(metadata.Languages) > 0 {
		snippet, lang, err := extractCodeSnippet(repoPath, metadata.Languages)
		if err != nil {
			fmt.Printf("Warning: Failed to extract code snippet: %v\n", err)
		} else {
			codeSnippet = snippet
			codeLanguage = lang
		}
	}

	projectSummary := &ProjectSummary{
		Name:         repoName,
		Summary:      summary,
		CodebergURL:  codebergURL,
		GitHubURL:    githubURL,
		Metadata:     metadata,
		Images:       images,
		CodeSnippet:  codeSnippet,
		CodeLanguage: codeLanguage,
	}

	// Save to cache
	if err := g.saveToCache(cacheFile, projectSummary); err != nil {
		fmt.Printf("Warning: Failed to save to cache: %v\n", err)
	} else {
		fmt.Printf("Summary cached at: %s\n", cacheFile)
	}

	return projectSummary, nil
}

// formatGemtext formats the summaries as Gemini Gemtext
func (g *Generator) formatGemtext(summaries []ProjectSummary) string {
	var builder strings.Builder

	// Header
	builder.WriteString("# Project Showcase\n\n")

	// Generated date at the top
	builder.WriteString(fmt.Sprintf("Generated on: %s\n\n", time.Now().Format("2006-01-02")))

	// Introduction paragraph
	builder.WriteString("This page showcases my side projects, providing an overview of what each project does, its technical implementation, and key metrics. Each project summary includes information about the programming languages used, development activity, releases, and licensing. The projects are ranked by score, which combines recent activity, project size, and tag history.\n\n")

	// Template inline TOC
	builder.WriteString("<< template::inline::toc\n\n")

	// Calculate total stats
	totalProjects := len(summaries)
	totalCommits := 0
	totalLOC := 0
	totalDocs := 0
	releasedCount := 0
	languageTotals := make(map[string]int)
	docTotals := make(map[string]int)

	for _, summary := range summaries {
		if summary.Metadata != nil {
			totalCommits += summary.Metadata.CommitCount
			totalLOC += summary.Metadata.LinesOfCode
			totalDocs += summary.Metadata.LinesOfDocs

			// Count projects with releases
			if summary.Metadata.HasReleases {
				releasedCount++
			}

			// Aggregate language statistics
			for _, lang := range summary.Metadata.Languages {
				languageTotals[lang.Name] += lang.Lines
			}

			// Aggregate documentation statistics
			for _, doc := range summary.Metadata.Documentation {
				docTotals[doc.Name] += doc.Lines
			}
		}
	}

	// Calculate language percentages
	var languageStats []LanguageStats
	for name, lines := range languageTotals {
		percentage := 0.0
		if totalLOC > 0 {
			percentage = float64(lines) * 100.0 / float64(totalLOC)
		}
		languageStats = append(languageStats, LanguageStats{
			Name:       name,
			Lines:      lines,
			Percentage: percentage,
		})
	}

	// Sort languages by percentage
	sort.Slice(languageStats, func(i, j int) bool {
		return languageStats[i].Percentage > languageStats[j].Percentage
	})

	// Calculate documentation percentages
	var docStats []LanguageStats
	for name, lines := range docTotals {
		percentage := 0.0
		if totalDocs > 0 {
			percentage = float64(lines) * 100.0 / float64(totalDocs)
		}
		docStats = append(docStats, LanguageStats{
			Name:       name,
			Lines:      lines,
			Percentage: percentage,
		})
	}

	// Sort documentation by percentage
	sort.Slice(docStats, func(i, j int) bool {
		return docStats[i].Percentage > docStats[j].Percentage
	})

	// Write total stats section
	builder.WriteString("## Overall Statistics\n\n")
	builder.WriteString(fmt.Sprintf("* 📦 Total Projects: %d\n", totalProjects))
	builder.WriteString(fmt.Sprintf("* 📊 Total Commits: %s\n", formatNumber(totalCommits)))
	builder.WriteString(fmt.Sprintf("* 📈 Total Lines of Code: %s\n", formatNumber(totalLOC)))
	if totalDocs > 0 {
		builder.WriteString(fmt.Sprintf("* 📄 Total Lines of Documentation: %s\n", formatNumber(totalDocs)))
	}
	if len(languageStats) > 0 {
		builder.WriteString(fmt.Sprintf("* 💻 Languages: %s\n", FormatLanguagesWithPercentages(languageStats)))
	}
	if len(docStats) > 0 {
		builder.WriteString(fmt.Sprintf("* 📚 Documentation: %s\n", FormatLanguagesWithPercentages(docStats)))
	}
	experimentalCount := totalProjects - releasedCount
	builder.WriteString(fmt.Sprintf("* 🚀 Release Status: %d released, %d experimental (%.1f%% with releases, %.1f%% experimental)\n",
		releasedCount, experimentalCount,
		float64(releasedCount)*100/float64(totalProjects),
		float64(experimentalCount)*100/float64(totalProjects)))
	builder.WriteString("\n")

	// Add Projects section
	builder.WriteString("## Projects\n\n")

	// Add each project
	for i, summary := range summaries {
		if i > 0 {
			builder.WriteString("\n---\n\n")
		}

		builder.WriteString(fmt.Sprintf("### %d. %s%s\n\n", i+1, summary.Name, formatRankHistoryForHeader(summary.RankHistory)))

		// Add metadata if available
		if summary.Metadata != nil {
			if len(summary.Metadata.Languages) > 0 {
				builder.WriteString(fmt.Sprintf("* 💻 Languages: %s\n", FormatLanguagesWithPercentages(summary.Metadata.Languages)))
			}
			if len(summary.Metadata.Documentation) > 0 {
				builder.WriteString(fmt.Sprintf("* 📚 Documentation: %s\n", FormatLanguagesWithPercentages(summary.Metadata.Documentation)))
			}
			builder.WriteString(fmt.Sprintf("* 📊 Commits: %d\n", summary.Metadata.CommitCount))
			builder.WriteString(fmt.Sprintf("* 📈 Lines of Code: %d\n", summary.Metadata.LinesOfCode))
			if summary.Metadata.LinesOfDocs > 0 {
				builder.WriteString(fmt.Sprintf("* 📄 Lines of Documentation: %d\n", summary.Metadata.LinesOfDocs))
			}
			builder.WriteString(fmt.Sprintf("* 🏷️ Tags: %d\n", summary.Metadata.TagCount))
			builder.WriteString(fmt.Sprintf("* 📅 Development Period: %s to %s\n", summary.Metadata.FirstCommitDate, summary.Metadata.LastCommitDate))
			builder.WriteString(fmt.Sprintf("* 🏆 Score: %.1f (combines recent activity, code size, and tags)\n", summary.Metadata.Score))
			builder.WriteString(fmt.Sprintf("* ⚖️ License: %s\n", summary.Metadata.License))

			// Add release information or experimental status
			if summary.Metadata.HasReleases && summary.Metadata.LatestTag != "" {
				if summary.Metadata.LatestTagDate != "" {
					builder.WriteString(fmt.Sprintf("* 🏷️ Latest Release: %s (%s)\n", summary.Metadata.LatestTag, summary.Metadata.LatestTagDate))
				} else {
					builder.WriteString(fmt.Sprintf("* 🏷️ Latest Release: %s\n", summary.Metadata.LatestTag))
				}
			} else {
				builder.WriteString("* 🧪 Status: Experimental (no releases yet)\n")
			}

			// Check if project might be obsolete (avg age > 2 years AND last commit > 1 year)
			if summary.Metadata.AvgCommitAge > 730 && summary.Metadata.LastCommitDate != "" {
				// Parse the last commit date
				lastCommit, err := time.Parse("2006-01-02", summary.Metadata.LastCommitDate)
				if err == nil {
					daysSinceLastCommit := time.Since(lastCommit).Hours() / 24
					if daysSinceLastCommit > 365 {
						builder.WriteString("\n⚠️  **Notice**: This project appears to be finished, obsolete, or no longer maintained. Last meaningful activity was over 2 years ago. Use at your own risk.")
					}
				}
			}
			builder.WriteString("\n\n")
		}

		// Handle images and paragraphs
		paragraphs := splitSummaryParagraphs(sanitizeSummaryForGemtext(summary.Summary))

		// If we have images, distribute them nicely
		if len(summary.Images) > 0 {
			// First image after metadata, before text
			builder.WriteString(fmt.Sprintf("=> %s %s screenshot\n\n", summary.Images[0], summary.Name))

			// First paragraph
			if len(paragraphs) > 0 {
				builder.WriteString(fmt.Sprintf("%s\n\n", strings.TrimSpace(paragraphs[0])))
			}

			// Second image after first paragraph (if we have 2 images and multiple paragraphs)
			if len(summary.Images) > 1 && len(paragraphs) > 1 {
				builder.WriteString(fmt.Sprintf("=> %s %s screenshot\n\n", summary.Images[1], summary.Name))
			}

			// Remaining paragraphs
			for i := 1; i < len(paragraphs); i++ {
				builder.WriteString(fmt.Sprintf("%s\n\n", strings.TrimSpace(paragraphs[i])))
			}
		} else {
			// No images - just add all paragraphs
			for _, para := range paragraphs {
				builder.WriteString(fmt.Sprintf("%s\n\n", strings.TrimSpace(para)))
			}
		}

		// Add links
		if summary.CodebergURL != "" {
			builder.WriteString(fmt.Sprintf("=> %s View on Codeberg\n", summary.CodebergURL))
		}
		if summary.GitHubURL != "" {
			builder.WriteString(fmt.Sprintf("=> %s View on GitHub\n", summary.GitHubURL))
		}

	}

	return builder.String()
}

// writeShowcaseFile writes the showcase content to the target file
func (g *Generator) writeShowcaseFile(content string) error {
	// Build target path
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	targetDir := filepath.Join(home, "git", "foo.zone-content", "gemtext", "about")
	targetFile := filepath.Join(targetDir, "showcase.gmi.tpl")

	// Create directory if it doesn't exist
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", targetDir, err)
	}

	// Write file
	if err := os.WriteFile(targetFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	fmt.Printf("\nShowcase written to: %s\n", targetFile)
	return nil
}

// updateShowcaseFile updates specific entries in an existing showcase file
func (g *Generator) updateShowcaseFile(newSummaries []ProjectSummary) error {
	// Load existing summaries from cache files instead of parsing Gemtext
	existingSummaries := make(map[string]ProjectSummary)

	// Get all repositories in work directory to load their cached summaries
	repos, err := g.getRepositories()
	if err == nil {
		cacheDir := filepath.Join(g.workDir, ".gitsyncer-showcase-cache")
		for _, repo := range repos {
			// Skip excluded repos
			if g.isExcluded(repo) {
				continue
			}

			cacheFile := filepath.Join(cacheDir, repo+".json")
			if cached, err := g.loadFromCache(cacheFile); err == nil {
				existingSummaries[repo] = *cached
			}
		}
	}

	// Update with new summaries
	for _, summary := range newSummaries {
		existingSummaries[summary.Name] = summary
	}

	// Convert map to slice
	var allSummaries []ProjectSummary
	for _, summary := range existingSummaries {
		allSummaries = append(allSummaries, summary)
	}

	// Sort by score (highest first)
	sort.Slice(allSummaries, func(i, j int) bool {
		// If metadata is missing, put at the end
		if allSummaries[i].Metadata == nil {
			return false
		}
		if allSummaries[j].Metadata == nil {
			return true
		}
		// Higher score is better (combines LOC and recent activity)
		return allSummaries[i].Metadata.Score > allSummaries[j].Metadata.Score
	})

	rankHistoryFile := filepath.Join(g.workDir, rankHistoryFilename)
	rankHistoryStore, err := loadRankHistory(rankHistoryFile)
	if err != nil {
		return fmt.Errorf("failed to load rank history: %w", err)
	}
	applyRankHistoryToSummaries(allSummaries, rankHistoryStore, time.Now(), rankHistoryPoints)

	// Format and write
	content := g.formatGemtext(allSummaries)
	if err := g.writeShowcaseFile(content); err != nil {
		return err
	}

	return nil
}

// loadFromCache loads a project summary from cache
func (g *Generator) loadFromCache(cacheFile string) (*ProjectSummary, error) {
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return nil, err
	}

	var summary ProjectSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return nil, err
	}

	return &summary, nil
}

// saveToCache saves a project summary to cache
func (g *Generator) saveToCache(cacheFile string, summary *ProjectSummary) error {
	// Create cache directory if it doesn't exist
	cacheDir := filepath.Dir(cacheFile)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return err
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}

	// Write to file
	return os.WriteFile(cacheFile, data, 0644)
}

// verifyImages checks if cached images still exist
func (g *Generator) verifyImages(summary *ProjectSummary) error {
	if len(summary.Images) == 0 {
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	showcaseDir := filepath.Join(home, "git", "foo.zone-content", "gemtext", "about")

	for _, imgPath := range summary.Images {
		fullPath := filepath.Join(showcaseDir, imgPath)
		if _, err := os.Stat(fullPath); err != nil {
			return fmt.Errorf("image not found: %s", imgPath)
		}
	}

	return nil
}

// filterExcludedRepos filters out repositories that are in the exclusion list
func (g *Generator) filterExcludedRepos(repos []string) []string {
	// Filter repositories
	var filtered []string
	for _, repo := range repos {
		if !g.isExcluded(repo) {
			filtered = append(filtered, repo)
		} else {
			fmt.Printf("Excluding repository from showcase (%s): %s\n", g.exclusionReason(repo), repo)
		}
	}

	return filtered
}

// isExcluded checks if a repository is in the exclusion list
func (g *Generator) isExcluded(repo string) bool {
	if isBackupRepo(repo) {
		return true
	}

	for _, excluded := range g.config.ExcludeFromShowcase {
		if excluded == repo {
			return true
		}
	}
	return false
}

// exclusionReason returns why a repository is excluded from showcase generation.
func (g *Generator) exclusionReason(repo string) string {
	var reasons []string

	if isBackupRepo(repo) {
		reasons = append(reasons, "backup suffix")
	}

	for _, excluded := range g.config.ExcludeFromShowcase {
		if excluded == repo {
			reasons = append(reasons, "config")
			break
		}
	}

	if len(reasons) == 0 {
		return "unknown reason"
	}

	return strings.Join(reasons, ", ")
}

// isBackupRepo checks whether a repository name has a backup suffix.
// Excluded patterns: *.bak and *.bak.*
func isBackupRepo(repo string) bool {
	return strings.HasSuffix(repo, ".bak") || strings.Contains(repo, ".bak.")
}

// formatNumber formats a number with thousands separators
func formatNumber(n int) string {
	str := fmt.Sprintf("%d", n)
	if len(str) <= 3 {
		return str
	}

	// Insert commas from right to left
	var result []byte
	for i := len(str) - 1; i >= 0; i-- {
		if (len(str)-i-1) > 0 && (len(str)-i-1)%3 == 0 {
			result = append([]byte{','}, result...)
		}
		result = append([]byte{str[i]}, result...)
	}

	return string(result)
}
