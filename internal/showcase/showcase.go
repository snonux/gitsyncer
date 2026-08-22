package showcase

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/snonux/gitsyncer/internal/aitool"
	"github.com/snonux/gitsyncer/internal/config"
	"github.com/snonux/gitsyncer/internal/localrepos"
)

// Generator handles showcase generation for repositories. It orchestrates
// the per-repo pipeline (filter -> summarize -> link -> write) and delegates
// the actual work to focused collaborators: SummaryCache owns cache/rank-
// history persistence, ProjectLinker owns outbound URL construction, and
// aitool.Runner (via resolveSummary) owns AI-tool selection and invocation.
type Generator struct {
	config  *config.Config
	workDir string
	aiTool  string
	cache   *SummaryCache
	linker  *ProjectLinker
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
		aiTool:  "opencode", // default to opencode (via ollama launch with glm-5.2:cloud)
		cache:   NewSummaryCache(workDir),
		linker:  NewProjectLinker(cfg),
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
	repos, err := g.resolveRepoList(repoFilter)
	if err != nil {
		return err
	}

	// Filter out excluded repositories
	filteredRepos := g.filterExcludedRepos(repos)
	fmt.Printf("Found %d repositories to process (after filtering %d excluded)\n",
		len(filteredRepos), len(repos)-len(filteredRepos))

	summaries, successCount := g.generateSummaries(filteredRepos, forceRegenerate)
	if successCount == 0 {
		return fmt.Errorf("failed to generate any summaries")
	}
	fmt.Printf("\nSuccessfully generated %d/%d summaries\n", successCount, len(repos))

	sortSummariesByScore(summaries)

	anchorDate := time.Now()
	if err := g.updateRankHistory(repoFilter, summaries, anchorDate); err != nil {
		return err
	}

	// When filtering (single repo), we need to update existing showcase.
	// writeShowcaseOutput merges the new summary with all cached ones (or, for
	// a full run, writes summaries directly) and returns the complete, sorted
	// list so we can regenerate the SVG consistently.
	allSummaries, err := g.writeShowcaseOutput(repoFilter, summaries)
	if err != nil {
		return err
	}

	// Always regenerate the interactive SVG rank history alongside the text showcase.
	if err := g.writeRankHistorySVGFile(allSummaries); err != nil {
		// Non-fatal: log the warning but don't abort the showcase run.
		fmt.Printf("Warning: failed to write rank history SVG: %v\n", err)
	}

	return nil
}

// resolveRepoList returns repoFilter verbatim when non-empty (a caller-
// requested subset), otherwise discovers all repositories in the work
// directory. Either way, an empty result is treated as an error since
// there'd be nothing to showcase.
func (g *Generator) resolveRepoList(repoFilter []string) ([]string, error) {
	repos := repoFilter
	if len(repos) == 0 {
		discovered, err := g.getRepositories()
		if err != nil {
			return nil, fmt.Errorf("failed to get repositories: %w", err)
		}
		repos = discovered
	}

	if len(repos) == 0 {
		return nil, fmt.Errorf("no repositories found")
	}
	return repos, nil
}

// generateSummaries runs generateProjectSummary for each repo in
// filteredRepos, printing a per-repo report and collecting the successful
// summaries. A repo whose summary generation fails is skipped with a
// warning rather than aborting the whole run.
func (g *Generator) generateSummaries(filteredRepos []string, forceRegenerate bool) ([]ProjectSummary, int) {
	summaries := make([]ProjectSummary, 0, len(filteredRepos))
	successCount := 0

	for i, repo := range filteredRepos {
		fmt.Printf("\n[%d/%d] Processing %s...\n", i+1, len(filteredRepos), repo)

		summary, err := g.generateProjectSummary(repo, forceRegenerate)
		if err != nil {
			fmt.Printf("WARNING: Failed to generate summary for %s: %v\n", repo, err)
			continue
		}

		printProjectSummary(repo, summary)
		summaries = append(summaries, *summary)
		successCount++
	}

	return summaries, successCount
}

// printProjectSummary prints a freshly generated summary and its metadata to
// stdout, for operator visibility while a showcase run is in progress.
func printProjectSummary(repo string, summary *ProjectSummary) {
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
}

// sortSummariesByScore sorts summaries by Metadata.Score descending (higher
// score is better, combining LOC and recent activity). Summaries with
// missing metadata sort last.
func sortSummariesByScore(summaries []ProjectSummary) {
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].Metadata == nil {
			return false
		}
		if summaries[j].Metadata == nil {
			return true
		}
		return summaries[i].Metadata.Score > summaries[j].Metadata.Score
	})
}

// updateRankHistory upserts today's ranking snapshot (only for full showcase
// runs — a single-repo update via repoFilter must not perturb the shared
// weekly snapshot) and then annotates summaries with their RankHistory field.
func (g *Generator) updateRankHistory(repoFilter []string, summaries []ProjectSummary, anchorDate time.Time) error {
	if len(repoFilter) == 0 {
		if err := g.cache.UpdateRankHistorySnapshot(anchorDate, summaries); err != nil {
			return err
		}
	}
	return g.cache.ApplyRankHistory(summaries, anchorDate)
}

// writeShowcaseOutput regenerates and writes the Gemtext showcase file: a
// single-repo update (repoFilter set) merges into the cached full project
// list via updateShowcaseFile, while a full run formats and writes summaries
// directly. It returns the complete summary list used for the write so the
// caller can also regenerate the SVG rank-history graph consistently.
func (g *Generator) writeShowcaseOutput(repoFilter []string, summaries []ProjectSummary) ([]ProjectSummary, error) {
	if len(repoFilter) > 0 {
		allSummaries, err := g.updateShowcaseFile(summaries)
		if err != nil {
			return nil, fmt.Errorf("failed to update showcase file: %w", err)
		}
		return allSummaries, nil
	}

	content := g.formatGemtext(summaries)
	if err := g.writeShowcaseFile(content); err != nil {
		return nil, fmt.Errorf("failed to write showcase file: %w", err)
	}
	return summaries, nil
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

	worktreePath, cleanup, err := createStatsWorktree(repoName, repoPath, branch, resolvedRef)
	if err != nil {
		return "", nil, err
	}

	if resolvedRef == branch {
		fmt.Printf("Using showcase stats branch %q for %s\n", branch, repoName)
	} else {
		fmt.Printf("Using showcase stats branch %q for %s (resolved to %s)\n", branch, repoName, resolvedRef)
	}

	return worktreePath, cleanup, nil
}

// createStatsWorktree checks out resolvedRef into a fresh temporary detached
// worktree under repoPath, so metadata/code-snippet extraction can read the
// showcase stats branch without disturbing the caller's main checkout. It
// returns the worktree path and a cleanup func that removes both the
// worktree and its temporary root; callers must invoke the cleanup func
// (typically via defer) once done with the worktree. branch is only used for
// the error message (it's the configured branch name, before resolution to
// resolvedRef) so failures are reported in terms the operator configured.
func createStatsWorktree(repoName, repoPath, branch, resolvedRef string) (string, func() error, error) {
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

	// Try to load a cached summary (but we'll still update metadata and
	// images below regardless of cache hit/miss). On a miss, this also gives
	// us the chain of AI tools to try.
	cachedSummary, haveCachedSummary, chain := g.loadCachedSummaryOrChain(repoName, forceRegenerate)

	readmeContent, _, readmeFound := findReadmeContent(repoPath)

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
		chain,
		readmeContent,
		readmeFound,
		cachedSummary,
		haveCachedSummary,
	)

	return g.assembleProjectSummary(repoName, repoPath, statsRepoPath, summary, metadata)
}

// assembleProjectSummary builds the final ProjectSummary from the pieces
// gathered by generateProjectSummary (project links, images, code snippet),
// persists it to the on-disk cache, and returns it.
func (g *Generator) assembleProjectSummary(repoName, repoPath, statsRepoPath, summary string, metadata *RepoMetadata) (*ProjectSummary, error) {
	codebergURL, githubURL := g.linker.BuildLinks(repoName, repoPath)

	images, codeSnippet, codeLanguage, err := g.collectAssets(repoName, repoPath, statsRepoPath, metadata)
	if err != nil {
		return nil, err
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

	g.saveSummaryToCache(repoName, projectSummary)

	return projectSummary, nil
}

// loadCachedSummaryOrChain tries to load a cached AI summary for repoName
// (skipped entirely when forceRegenerate is set). On a cache hit, it
// returns the cached summary text and a nil chain, since no AI tool needs to
// run. On a miss, it returns the chain of AI tools to try instead: this
// prefers opencode when the configured tool is "" (aligns with the
// release-notes flow), then falls back through the rest of the chain that's
// actually installed.
func (g *Generator) loadCachedSummaryOrChain(repoName string, forceRegenerate bool) (cachedSummary string, haveCachedSummary bool, chain []aitool.Tool) {
	if !forceRegenerate {
		if cached, err := g.cache.Load(repoName); err == nil {
			fmt.Printf("Using cached AI summary (cache file: %s)\n", g.cache.FilePath(repoName))
			return cached.Summary, true, nil
		}
	}

	return "", false, aitool.AvailableChain(g.aiTool, nil)
}

// saveSummaryToCache persists projectSummary to the on-disk cache. A write
// failure is logged as a warning rather than propagated: a showcase run
// should still complete (and print the summary) even if caching fails.
func (g *Generator) saveSummaryToCache(repoName string, projectSummary *ProjectSummary) {
	if err := g.cache.Save(repoName, projectSummary); err != nil {
		fmt.Printf("Warning: Failed to save to cache: %v\n", err)
		return
	}
	fmt.Printf("Summary cached at: %s\n", g.cache.FilePath(repoName))
}

func (g *Generator) resolveSummary(
	repoName, repoPath string,
	chain []aitool.Tool,
	readmeContent []byte,
	readmeFound bool,
	cachedSummary string,
	haveCachedSummary bool,
) string {
	// Get the summary - either from cache or by running the AI tool chain.
	var summary string
	if haveCachedSummary {
		summary = cachedSummary
		fmt.Printf("Using cached AI summary\n")
	} else {
		prompt := "Please provide a 1-2 paragraph summary of this project, explaining what it does, why it's useful, and how it's implemented. Focus on the key features and architecture. Be concise but informative."

		// README content (if any) rides along as the stdin payload; aitool
		// runs prompt+repoPath through each available tool in chain until
		// one succeeds, so this package doesn't need its own raw-string
		// switch over tool names.
		stdin := ""
		if readmeFound {
			stdin = string(readmeContent)
		}
		if aiSummary, _, err := aitool.RunChain(chain, repoPath, prompt, stdin); err == nil {
			summary = aiSummary
		}

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
	// Load all existing cached summaries so the single-repo update does not
	// accidentally truncate the full showcase to just the one regenerated project.
	// If getRepositories fails (e.g. work directory temporarily unavailable),
	// log a warning and continue — the overlay below will still write newSummaries,
	// which is better than silently producing an empty/truncated output file.
	existingSummaries := make(map[string]ProjectSummary)
	repos, err := g.getRepositories()
	if err != nil {
		fmt.Printf("Warning: failed to list repositories for showcase update: %v (cached repos may be missing from output)\n", err)
	} else {
		existingSummaries = g.cache.LoadAll(repos, g.isExcluded)
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
	sortSummariesByScore(allSummaries)

	if err := g.cache.ApplyRankHistory(allSummaries, time.Now()); err != nil {
		return nil, err
	}

	// Format and write the Gemtext showcase.
	content := g.formatGemtext(allSummaries)
	if err := g.writeShowcaseFile(content); err != nil {
		return nil, err
	}

	return allSummaries, nil
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
