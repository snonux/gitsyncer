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

	"codeberg.org/snonux/gitsyncer/internal/aitool"
	"codeberg.org/snonux/gitsyncer/internal/config"
	"codeberg.org/snonux/gitsyncer/internal/localrepos"
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
	CgitURL      string
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
		aiTool:  "opencode", // default to opencode (via ollama launch with glm-5.2:cloud)
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

	// When filtering (single repo), we need to update existing showcase.
	// updateShowcaseFile merges the new summary with all cached ones and returns
	// the complete, sorted list so we can regenerate the SVG consistently.
	var allSummaries []ProjectSummary
	if len(repoFilter) > 0 {
		allSummaries, err = g.updateShowcaseFile(summaries)
		if err != nil {
			return fmt.Errorf("failed to update showcase file: %w", err)
		}
	} else {
		// Full regeneration - format as Gemtext and write.
		content := g.formatGemtext(summaries)
		if err := g.writeShowcaseFile(content); err != nil {
			return fmt.Errorf("failed to write showcase file: %w", err)
		}
		allSummaries = summaries
	}

	// Always regenerate the interactive SVG rank history alongside the text showcase.
	if err := g.writeRankHistorySVGFile(allSummaries); err != nil {
		// Non-fatal: log the warning but don't abort the showcase run.
		fmt.Printf("Warning: failed to write rank history SVG: %v\n", err)
	}

	return nil
}

// runCommandWithTimeout runs a command with a short timeout and returns trimmed stdout.
// Stderr is included in the error message for easier debugging when GITSYNCER_DEBUG=1.
func runCommandWithTimeout(name string, args ...string) (string, error) {
	return runCommandWithCustomTimeout(8*time.Second, name, args...)
}

func runCommandWithCustomTimeout(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
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
	return selectSummaryToolWithLookPath(aiTool, nil)
}

func selectSummaryToolWithLookPath(aiTool string, lookPath aitool.LookPathFunc) string {
	return string(aitool.FirstAvailable(aiTool, lookPath))
}

func runSummaryTool(selectedTool, prompt, repoPath, readmeFile string, readmeContent []byte, readmeFound bool) string {
	var cmd *exec.Cmd

	switch selectedTool {
	case "opencode":
		fmt.Printf("Running ollama launch opencode command\n")
		if readmeFound {
			fullPrompt := prompt + "\n\nREADME content:\n" + string(readmeContent)
			fmt.Printf("  ollama launch opencode --model glm-5.2:cloud -y -- run \"...\"\n")
			fmt.Printf("  Using %s as input\n", readmeFile)
			cmd = exec.Command("ollama", "launch", "opencode", "--model", "glm-5.2:cloud", "-y", "--", "run", fullPrompt)
		}
	case "hexai":
		fmt.Printf("Running hexai command (stdin payload)\n")
		if readmeFound {
			fmt.Printf("  echo <README content> | hexai \"%s\"\n", prompt)
			fmt.Printf("  Using %s as input\n", readmeFile)
			cmd = exec.Command("hexai", prompt)
			cmd.Stdin = strings.NewReader(string(readmeContent))
		}
	case "claude":
		fmt.Printf("Running Claude command:\n")
		fmt.Printf("  claude --model sonnet \"%s\"\n", prompt)
		cmd = exec.Command("claude", "--model", "sonnet", prompt)
	case "amp":
		fmt.Printf("Running amp command (stdin payload)\n")
		if readmeFound {
			fmt.Printf("  echo <README content> | amp --execute \"%s\"\n", prompt)
			fmt.Printf("  Using %s as input\n", readmeFile)
			cmd = exec.Command("amp", "--execute", prompt)
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

// getRepositories returns a list of repository directories in the work directory
func (g *Generator) getRepositories() ([]string, error) {
	return localrepos.ListLocalReposWithGitDir(g.workDir)
}

func (g *Generator) buildProjectLinks(repoName, repoPath string) (string, string, string) {
	cfg := g.config
	if cfg == nil {
		cfg = &config.Config{}
	}

	codebergURL := ""
	githubURL := ""
	cgitURL := fmt.Sprintf("%s/%s/", cfg.GetShowcaseCgitHost(), repoName)

	// Only link to Codeberg when Codeberg syncing is enabled in the config and
	// this repository is actually wired up to sync with Codeberg (i.e. a
	// Codeberg remote is configured in its local working copy). A Codeberg org
	// may be present in the config for tokens/showcase without every project
	// being synced there.
	if cfg.CodebergSyncEnabled() && repoHasCodebergRemote(repoPath) {
		if codebergOrg := cfg.FindCodebergOrg(); codebergOrg != nil {
			codebergURL = fmt.Sprintf("https://codeberg.org/%s/%s", codebergOrg.Name, repoName)
		}
	}

	if githubOrg := cfg.FindGitHubOrg(); githubOrg != nil {
		githubURL = fmt.Sprintf("https://github.com/%s/%s", githubOrg.Name, repoName)
	}

	return codebergURL, githubURL, cgitURL
}

// repoHasCodebergRemote reports whether the working copy at repoPath has a git
// remote pointing at codeberg.org, i.e. the repository is actually set up to
// sync with Codeberg.
func repoHasCodebergRemote(repoPath string) bool {
	if repoPath == "" {
		return false
	}
	cmd := exec.Command("git", "-C", repoPath, "remote", "-v")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), "codeberg.org")
}

func (g *Generator) prepareStatsRepoPath(repoName, repoPath string) (string, func() error, error) {
	if g.config == nil {
		return repoPath, func() error { return nil }, nil
	}

	branch := strings.TrimSpace(g.config.ShowcaseStatsBranches[repoName])
	if branch == "" {
		return repoPath, func() error { return nil }, nil
	}

	resolvedRef, err := resolveShowcaseStatsRef(repoPath, branch)
	if err != nil {
		return "", nil, fmt.Errorf("failed to resolve showcase stats branch for %s: %w", repoName, err)
	}

	tempPrefix := strings.ReplaceAll(repoName, string(os.PathSeparator), "-")
	tempRoot, err := os.MkdirTemp("", "gitsyncer-showcase-"+tempPrefix+"-")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temporary worktree root for %s: %w", repoName, err)
	}

	worktreePath := filepath.Join(tempRoot, "repo")
	if _, err := runCommandWithCustomTimeout(45*time.Second, "git", "-C", repoPath, "worktree", "add", "--detach", worktreePath, resolvedRef); err != nil {
		_ = os.RemoveAll(tempRoot)
		return "", nil, fmt.Errorf("failed to create showcase stats worktree for %s on branch %q: %w", repoName, branch, err)
	}

	cleanup := func() error {
		// Best-effort cleanup of the temporary worktree root, matching the
		// discard above for the same call on the worktree-add failure path:
		// this is scratch space, and by the time we get here the actual
		// worktree removal below is the operation whose error matters.
		defer func() { _ = os.RemoveAll(tempRoot) }()

		if _, err := runCommandWithCustomTimeout(45*time.Second, "git", "-C", repoPath, "worktree", "remove", "--force", worktreePath); err != nil {
			return fmt.Errorf("failed to remove temporary worktree for %s: %w", repoName, err)
		}

		return nil
	}

	if resolvedRef == branch {
		fmt.Printf("Using showcase stats branch %q for %s\n", branch, repoName)
	} else {
		fmt.Printf("Using showcase stats branch %q for %s (resolved to %s)\n", branch, repoName, resolvedRef)
	}

	return worktreePath, cleanup, nil
}

func resolveShowcaseStatsRef(repoPath, branch string) (string, error) {
	localRef := "refs/heads/" + branch
	if _, err := runCommandWithTimeout("git", "-C", repoPath, "show-ref", "--verify", "--quiet", localRef); err == nil {
		return branch, nil
	}

	output, err := runCommandWithTimeout("git", "-C", repoPath, "for-each-ref", "--format=%(refname)", "refs/remotes")
	if err != nil {
		return "", fmt.Errorf("failed to inspect remote refs for branch %q: %w", branch, err)
	}

	var candidates []string
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		ref := strings.TrimSpace(line)
		if ref == "" || strings.HasSuffix(ref, "/HEAD") {
			continue
		}
		if strings.HasSuffix(ref, "/"+branch) {
			candidates = append(candidates, ref)
		}
	}

	if len(candidates) == 0 {
		return "", fmt.Errorf("branch %q not found locally or on any remote", branch)
	}

	sort.Strings(candidates)
	for _, ref := range candidates {
		if strings.HasPrefix(ref, "refs/remotes/origin/") {
			return ref, nil
		}
	}

	return candidates[0], nil
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
	// Prefer opencode if available when default tool is "" (aligns with release flow)
	selectedTool := g.aiTool
	if !haveCachedSummary {
		selectedTool = selectSummaryTool(g.aiTool)
	}

	readmeContent, readmeFile, readmeFound := findReadmeContent(repoPath)

	statsRepoPath, cleanupStatsRepoPath, err := g.prepareStatsRepoPath(repoName, repoPath)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := cleanupStatsRepoPath(); err != nil {
			fmt.Printf("Warning: %v\n", err)
		}
	}()

	// Always extract metadata (not cached)
	fmt.Printf("Extracting repository metadata...\n")
	metadata, err := extractRepoMetadata(statsRepoPath)
	if err != nil {
		fmt.Printf("Warning: Failed to extract some metadata: %v\n", err)
		// Continue anyway with partial metadata
	}

	summary := g.resolveSummary(
		repoName,
		repoPath,
		selectedTool,
		readmeFile,
		readmeContent,
		readmeFound,
		cachedSummary,
		haveCachedSummary,
	)

	// Build URLs
	codebergURL, githubURL, cgitURL := g.buildProjectLinks(repoName, repoPath)

	images, codeSnippet, codeLanguage, err := g.collectAssets(repoName, repoPath, statsRepoPath, metadata)
	if err != nil {
		return nil, err
	}

	projectSummary := &ProjectSummary{
		Name:         repoName,
		Summary:      summary,
		CodebergURL:  codebergURL,
		GitHubURL:    githubURL,
		CgitURL:      cgitURL,
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

func (g *Generator) resolveSummary(
	repoName, repoPath, selectedTool, readmeFile string,
	readmeContent []byte,
	readmeFound bool,
	cachedSummary string,
	haveCachedSummary bool,
) string {
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

	return sanitizeSummaryForGemtext(summary)
}

func (g *Generator) collectAssets(repoName, repoPath, statsRepoPath string, metadata *RepoMetadata) ([]string, string, string, error) {
	// Always extract images from README (not cached)
	fmt.Printf("Extracting images from README...\n")
	showcaseDir, err := g.showcaseOutputDir()
	if err != nil {
		return nil, "", "", err
	}

	images, err := extractImagesFromRepo(repoPath, repoName, showcaseDir)
	if err != nil {
		fmt.Printf("Warning: Failed to extract images: %v\n", err)
		// Continue without images
	}

	// Extract code snippet for all projects
	var codeSnippet, codeLanguage string
	if metadata != nil && len(metadata.Languages) > 0 {
		snippet, lang, err := extractCodeSnippet(statsRepoPath, metadata.Languages)
		if err != nil {
			fmt.Printf("Warning: Failed to extract code snippet: %v\n", err)
		} else {
			codeSnippet = snippet
			codeLanguage = lang
		}
	}

	return images, codeSnippet, codeLanguage, nil
}

// showcaseOutputDir returns the configured showcase output directory,
// falling back to defaults when not configured.
func (g *Generator) showcaseOutputDir() (string, error) {
	if g.config == nil {
		return (&config.Config{}).GetShowcaseOutputDir()
	}
	return g.config.GetShowcaseOutputDir()
}

// writeShowcaseFile writes the showcase content to the target file
func (g *Generator) writeShowcaseFile(content string) error {
	targetDir, err := g.showcaseOutputDir()
	if err != nil {
		return err
	}

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

// writeRankHistorySVGFile generates an interactive SVG rank history graph and
// writes it to the same directory as the showcase Gemtext file.
func (g *Generator) writeRankHistorySVGFile(summaries []ProjectSummary) error {
	targetDir, err := g.showcaseOutputDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", targetDir, err)
	}

	svgContent := GenerateRankHistorySVG(summaries)

	targetFile := filepath.Join(targetDir, "showcase-rank-history.svg")
	if err := os.WriteFile(targetFile, []byte(svgContent), 0644); err != nil {
		return fmt.Errorf("failed to write rank history SVG: %w", err)
	}

	fmt.Printf("Rank history SVG written to: %s\n", targetFile)
	return nil
}

// updateShowcaseFile merges newSummaries with all existing cached project
// summaries, re-sorts by score, applies rank history, regenerates the Gemtext
// showcase file, and returns the complete sorted summary list so the caller
// can also regenerate the SVG graph consistently.
func (g *Generator) updateShowcaseFile(newSummaries []ProjectSummary) ([]ProjectSummary, error) {
	// Load all cached summaries from disk so the full project list is available.
	existingSummaries := make(map[string]ProjectSummary)

	// Load all existing cached summaries so the single-repo update does not
	// accidentally truncate the full showcase to just the one regenerated project.
	// If getRepositories fails (e.g. work directory temporarily unavailable),
	// log a warning and continue — the overlay below will still write newSummaries,
	// which is better than silently producing an empty/truncated output file.
	repos, err := g.getRepositories()
	if err != nil {
		fmt.Printf("Warning: failed to list repositories for showcase update: %v (cached repos may be missing from output)\n", err)
	} else {
		cacheDir := filepath.Join(g.workDir, ".gitsyncer-showcase-cache")
		for _, repo := range repos {
			if g.isExcluded(repo) {
				continue
			}
			cacheFile := filepath.Join(cacheDir, repo+".json")
			if cached, err := g.loadFromCache(cacheFile); err == nil {
				existingSummaries[repo] = *cached
			}
		}
	}

	// Overlay the freshly generated summaries onto the cached set.
	for _, summary := range newSummaries {
		existingSummaries[summary.Name] = summary
	}

	// Flatten map to slice and sort by score (highest first).
	allSummaries := make([]ProjectSummary, 0, len(existingSummaries))
	for _, summary := range existingSummaries {
		allSummaries = append(allSummaries, summary)
	}
	sort.Slice(allSummaries, func(i, j int) bool {
		if allSummaries[i].Metadata == nil {
			return false
		}
		if allSummaries[j].Metadata == nil {
			return true
		}
		return allSummaries[i].Metadata.Score > allSummaries[j].Metadata.Score
	})

	rankHistoryFile := filepath.Join(g.workDir, rankHistoryFilename)
	rankHistoryStore, err := loadRankHistory(rankHistoryFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load rank history: %w", err)
	}
	applyRankHistoryToSummaries(allSummaries, rankHistoryStore, time.Now(), rankHistoryPoints)

	// Format and write the Gemtext showcase.
	content := g.formatGemtext(allSummaries)
	if err := g.writeShowcaseFile(content); err != nil {
		return nil, err
	}

	return allSummaries, nil
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

	showcaseDir, err := g.showcaseOutputDir()
	if err != nil {
		return err
	}

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
