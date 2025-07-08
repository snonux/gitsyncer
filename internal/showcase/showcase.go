package showcase

import (
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
}

// ProjectSummary holds the summary information for a project
type ProjectSummary struct {
	Name        string
	Summary     string
	CodebergURL string
	GitHubURL   string
	Metadata    *RepoMetadata
	Images      []string // Relative paths to images in showcase directory
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
	}
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
			fmt.Printf("Avg. age of last 42 commits: %.1f days\n", summary.Metadata.AvgCommitAge)
		}
		fmt.Println("--- End of summary ---")
		
		summaries = append(summaries, *summary)
		successCount++
	}

	if successCount == 0 {
		return fmt.Errorf("failed to generate any summaries")
	}

	fmt.Printf("\nSuccessfully generated %d/%d summaries\n", successCount, len(repos))

	// Sort summaries by average commit age (newest first)
	sort.Slice(summaries, func(i, j int) bool {
		// If metadata is missing, put at the end
		if summaries[i].Metadata == nil {
			return false
		}
		if summaries[j].Metadata == nil {
			return true
		}
		// Lower average age means more recent activity
		return summaries[i].Metadata.AvgCommitAge < summaries[j].Metadata.AvgCommitAge
	})

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
			fmt.Printf("Using cached Claude summary (cache file: %s)\n", cacheFile)
			cachedSummary = cached.Summary
			haveCachedSummary = true
		}
	}

	// Check if claude command exists (only if we need to run it)
	if !haveCachedSummary {
		if _, err := exec.LookPath("claude"); err != nil {
			return nil, fmt.Errorf("claude command not found. Please install Claude CLI")
		}
	}

	// Change to repository directory
	originalDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get current directory: %w", err)
	}
	defer os.Chdir(originalDir)

	if err := os.Chdir(repoPath); err != nil {
		return nil, fmt.Errorf("failed to change to repository directory: %w", err)
	}

	// Always extract metadata (not cached)
	fmt.Printf("Extracting repository metadata...\n")
	metadata, err := extractRepoMetadata(repoPath)
	if err != nil {
		fmt.Printf("Warning: Failed to extract some metadata: %v\n", err)
		// Continue anyway with partial metadata
	}

	// Get the summary - either from cache or by running Claude
	var summary string
	if haveCachedSummary {
		summary = cachedSummary
		fmt.Printf("Using cached Claude summary\n")
	} else {
		// Run claude command
		prompt := "Please provide a 1-2 paragraph summary of this project, explaining what it does, why it's useful, and how it's implemented. Focus on the key features and architecture. Be concise but informative."
		
		fmt.Printf("Running Claude command:\n")
		fmt.Printf("  claude --model sonnet \"%s\"\n", prompt)
		
		cmd := exec.Command("claude", "--model", "sonnet", prompt)
		output, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("failed to run claude: %w", err)
		}

		summary = strings.TrimSpace(string(output))
		if summary == "" {
			return nil, fmt.Errorf("received empty summary from claude")
		}
	}

	// Build URLs
	codebergURL := ""
	githubURL := ""
	
	if codebergOrg := g.config.FindCodebergOrg(); codebergOrg != nil {
		codebergURL = fmt.Sprintf("https://codeberg.org/%s/%s", codebergOrg.Name, repoName)
	}
	
	if githubOrg := g.config.FindGitHubOrg(); githubOrg != nil {
		githubURL = fmt.Sprintf("https://github.com/%s/%s", githubOrg.Name, repoName)
	}

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

	projectSummary := &ProjectSummary{
		Name:        repoName,
		Summary:     summary,
		CodebergURL: codebergURL,
		GitHubURL:   githubURL,
		Metadata:    metadata,
		Images:      images,
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
	
	// Introduction paragraph
	builder.WriteString("This page showcases my open source projects, providing an overview of what each project does, its technical implementation, and key metrics. Each project summary includes information about the programming languages used, development activity, and licensing.\n\n")
	
	// Template inline TOC
	builder.WriteString("<< template::inline::toc\n\n")
	
	// Calculate total stats
	totalProjects := len(summaries)
	totalCommits := 0
	totalLOC := 0
	totalDocs := 0
	languageTotals := make(map[string]int)
	docTotals := make(map[string]int)
	
	for _, summary := range summaries {
		if summary.Metadata != nil {
			totalCommits += summary.Metadata.CommitCount
			totalLOC += summary.Metadata.LinesOfCode
			totalDocs += summary.Metadata.LinesOfDocs
			
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
	builder.WriteString(fmt.Sprintf("* Total Projects: %d\n", totalProjects))
	builder.WriteString(fmt.Sprintf("* Total Commits: %s\n", formatNumber(totalCommits)))
	builder.WriteString(fmt.Sprintf("* Total Lines of Code: %s\n", formatNumber(totalLOC)))
	if totalDocs > 0 {
		builder.WriteString(fmt.Sprintf("* Total Lines of Documentation: %s\n", formatNumber(totalDocs)))
	}
	if len(languageStats) > 0 {
		builder.WriteString(fmt.Sprintf("* Languages: %s\n", FormatLanguagesWithPercentages(languageStats)))
	}
	if len(docStats) > 0 {
		builder.WriteString(fmt.Sprintf("* Documentation: %s\n", FormatLanguagesWithPercentages(docStats)))
	}
	builder.WriteString("\n")
	
	builder.WriteString(fmt.Sprintf("Generated on: %s\n\n", time.Now().Format("2006-01-02")))

	// Add Projects section
	builder.WriteString("## Projects\n\n")

	// Add each project
	for i, summary := range summaries {
		if i > 0 {
			builder.WriteString("\n---\n\n")
		}

		builder.WriteString(fmt.Sprintf("### %s\n\n", summary.Name))
		
		// Add metadata if available
		if summary.Metadata != nil {
			if len(summary.Metadata.Languages) > 0 {
				builder.WriteString(fmt.Sprintf("* Languages: %s\n", FormatLanguagesWithPercentages(summary.Metadata.Languages)))
			}
			if len(summary.Metadata.Documentation) > 0 {
				builder.WriteString(fmt.Sprintf("* Documentation: %s\n", FormatLanguagesWithPercentages(summary.Metadata.Documentation)))
			}
			builder.WriteString(fmt.Sprintf("* Commits: %d\n", summary.Metadata.CommitCount))
			builder.WriteString(fmt.Sprintf("* Lines of Code: %d\n", summary.Metadata.LinesOfCode))
			if summary.Metadata.LinesOfDocs > 0 {
				builder.WriteString(fmt.Sprintf("* Lines of Documentation: %d\n", summary.Metadata.LinesOfDocs))
			}
			builder.WriteString(fmt.Sprintf("* Development Period: %s to %s\n", summary.Metadata.FirstCommitDate, summary.Metadata.LastCommitDate))
			builder.WriteString(fmt.Sprintf("* Recent Activity: %.1f days (avg. age of last 42 commits)\n", summary.Metadata.AvgCommitAge))
			builder.WriteString(fmt.Sprintf("* License: %s\n\n", summary.Metadata.License))
		}
		
		// Handle images and paragraphs
		paragraphs := strings.Split(summary.Summary, "\n\n")
		
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
			// No images, just add paragraphs normally
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
	
	// Sort by average commit age (newest first)
	sort.Slice(allSummaries, func(i, j int) bool {
		// If metadata is missing, put at the end
		if allSummaries[i].Metadata == nil {
			return false
		}
		if allSummaries[j].Metadata == nil {
			return true
		}
		// Lower average age means more recent activity
		return allSummaries[i].Metadata.AvgCommitAge < allSummaries[j].Metadata.AvgCommitAge
	})

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
	if len(g.config.ExcludeFromShowcase) == 0 {
		return repos
	}
	
	// Create a map for quick lookup
	excludeMap := make(map[string]bool)
	for _, excluded := range g.config.ExcludeFromShowcase {
		excludeMap[excluded] = true
	}
	
	// Filter repositories
	var filtered []string
	for _, repo := range repos {
		if !excludeMap[repo] {
			filtered = append(filtered, repo)
		} else {
			fmt.Printf("Excluding repository from showcase: %s\n", repo)
		}
	}
	
	return filtered
}

// isExcluded checks if a repository is in the exclusion list
func (g *Generator) isExcluded(repo string) bool {
	for _, excluded := range g.config.ExcludeFromShowcase {
		if excluded == repo {
			return true
		}
	}
	return false
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