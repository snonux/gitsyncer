package showcase

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

type showcaseOverviewStats struct {
	totalProjects int
	totalCommits  int
	totalLOC      int
	totalDocs     int
	releasedCount int
	languageStats []LanguageStats
	docStats      []LanguageStats
}

// formatGemtext formats the summaries as Gemini Gemtext.
func (g *Generator) formatGemtext(summaries []ProjectSummary) string {
	var builder strings.Builder

	writeGemtextHeader(&builder)
	stats := collectShowcaseOverviewStats(summaries)
	writeShowcaseOverviewStats(&builder, stats)
	g.writeProjectsGemtext(&builder, summaries)

	return builder.String()
}

func writeGemtextHeader(builder *strings.Builder) {
	// Header
	builder.WriteString("# Project Showcase\n\n")

	// Generated date at the top
	builder.WriteString(fmt.Sprintf("Generated on: %s\n\n", time.Now().Format("2006-01-02")))

	// Link to the interactive SVG rank history graph — placed at the very top
	// so it is the first thing a reader sees after the date line.
	builder.WriteString("=> showcase-rank-history.svg Interactive Project Rank History Graph (SVG)\n\n")

	// Introduction paragraph
	builder.WriteString("This page showcases my side projects, providing an overview of what each project does, its technical implementation, and key metrics. Each project summary includes information about the programming languages used, development activity, releases, and licensing. The projects are ranked by score, which combines recent activity, project size, tag history, and whether the project has shipped a release.\n\n")

	// Template inline TOC
	builder.WriteString("<< template::inline::toc\n\n")
}

func collectShowcaseOverviewStats(summaries []ProjectSummary) showcaseOverviewStats {
	stats := showcaseOverviewStats{
		totalProjects: len(summaries),
	}
	languageTotals := make(map[string]int)
	docTotals := make(map[string]int)

	for _, summary := range summaries {
		if summary.Metadata == nil {
			continue
		}

		stats.totalCommits += summary.Metadata.CommitCount
		stats.totalLOC += summary.Metadata.LinesOfCode
		stats.totalDocs += summary.Metadata.LinesOfDocs

		if summary.Metadata.HasReleases {
			stats.releasedCount++
		}

		for _, lang := range summary.Metadata.Languages {
			languageTotals[lang.Name] += lang.Lines
		}

		for _, doc := range summary.Metadata.Documentation {
			docTotals[doc.Name] += doc.Lines
		}
	}

	stats.languageStats = buildSortedPercentageStats(languageTotals, stats.totalLOC)
	stats.docStats = buildSortedPercentageStats(docTotals, stats.totalDocs)
	return stats
}

func buildSortedPercentageStats(lineTotalsByName map[string]int, totalLines int) []LanguageStats {
	stats := make([]LanguageStats, 0, len(lineTotalsByName))
	for name, lines := range lineTotalsByName {
		percentage := 0.0
		if totalLines > 0 {
			percentage = float64(lines) * 100.0 / float64(totalLines)
		}
		stats = append(stats, LanguageStats{
			Name:       name,
			Lines:      lines,
			Percentage: percentage,
		})
	}

	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Percentage > stats[j].Percentage
	})
	return stats
}

func writeShowcaseOverviewStats(builder *strings.Builder, stats showcaseOverviewStats) {
	builder.WriteString("## Overall Statistics\n\n")
	builder.WriteString(fmt.Sprintf("* 📦 Total Projects: %d\n", stats.totalProjects))
	builder.WriteString(fmt.Sprintf("* 📊 Total Commits: %s\n", formatNumber(stats.totalCommits)))
	builder.WriteString(fmt.Sprintf("* 📈 Total Lines of Code: %s\n", formatNumber(stats.totalLOC)))
	if stats.totalDocs > 0 {
		builder.WriteString(fmt.Sprintf("* 📄 Total Lines of Documentation: %s\n", formatNumber(stats.totalDocs)))
	}
	if len(stats.languageStats) > 0 {
		builder.WriteString(fmt.Sprintf("* 💻 Languages: %s\n", FormatLanguagesWithPercentages(stats.languageStats)))
	}
	if len(stats.docStats) > 0 {
		builder.WriteString(fmt.Sprintf("* 📚 Documentation: %s\n", FormatLanguagesWithPercentages(stats.docStats)))
	}

	experimentalCount, releasedPercentage, experimentalPercentage := releaseStatusBreakdown(stats.totalProjects, stats.releasedCount)
	builder.WriteString(fmt.Sprintf("* 🚀 Release Status: %d released, %d experimental (%.1f%% with releases, %.1f%% experimental)\n",
		stats.releasedCount, experimentalCount, releasedPercentage, experimentalPercentage))
	builder.WriteString("\n")
}

func releaseStatusBreakdown(totalProjects, releasedCount int) (experimentalCount int, releasedPercentage, experimentalPercentage float64) {
	experimentalCount = totalProjects - releasedCount
	if totalProjects == 0 {
		return experimentalCount, 0, 0
	}
	releasedPercentage = float64(releasedCount) * 100 / float64(totalProjects)
	experimentalPercentage = float64(experimentalCount) * 100 / float64(totalProjects)
	return experimentalCount, releasedPercentage, experimentalPercentage
}

func (g *Generator) writeProjectsGemtext(builder *strings.Builder, summaries []ProjectSummary) {
	builder.WriteString("## Projects\n\n")

	for i, summary := range summaries {
		if i > 0 {
			builder.WriteString("\n---\n\n")
		}

		g.writeProjectGemtext(builder, i, summary)
	}
}

func (g *Generator) writeProjectGemtext(builder *strings.Builder, index int, summary ProjectSummary) {
	builder.WriteString(fmt.Sprintf("### %d. %s\n\n", index+1, summary.Name))
	writeProjectMetadata(builder, summary.Metadata)
	writeProjectSummaryContent(builder, summary)
	writeProjectLinks(builder, summary)
}

func writeProjectMetadata(builder *strings.Builder, metadata *RepoMetadata) {
	if metadata == nil {
		return
	}

	if len(metadata.Languages) > 0 {
		builder.WriteString(fmt.Sprintf("* 💻 Languages: %s\n", FormatLanguagesWithPercentages(metadata.Languages)))
	}
	if len(metadata.Documentation) > 0 {
		builder.WriteString(fmt.Sprintf("* 📚 Documentation: %s\n", FormatLanguagesWithPercentages(metadata.Documentation)))
	}
	builder.WriteString(fmt.Sprintf("* 📊 Commits: %d\n", metadata.CommitCount))
	builder.WriteString(fmt.Sprintf("* 📈 Lines of Code: %d\n", metadata.LinesOfCode))
	if metadata.LinesOfDocs > 0 {
		builder.WriteString(fmt.Sprintf("* 📄 Lines of Documentation: %d\n", metadata.LinesOfDocs))
	}
	builder.WriteString(fmt.Sprintf("* 🏷️ Tags: %d\n", metadata.TagCount))
	builder.WriteString(fmt.Sprintf("* 📅 Development Period: %s to %s\n", metadata.FirstCommitDate, metadata.LastCommitDate))
	builder.WriteString(fmt.Sprintf("* 🏆 Score: %.1f (combines recent activity, code size, tags, and release status)\n", metadata.Score))
	builder.WriteString(fmt.Sprintf("* ⚖️ License: %s\n", metadata.License))

	// Add release information or experimental status.
	if metadata.HasReleases && metadata.LatestTag != "" {
		if metadata.LatestTagDate != "" {
			builder.WriteString(fmt.Sprintf("* 🏷️ Latest Release: %s (%s)\n", metadata.LatestTag, metadata.LatestTagDate))
		} else {
			builder.WriteString(fmt.Sprintf("* 🏷️ Latest Release: %s\n", metadata.LatestTag))
		}
	} else {
		builder.WriteString("* 🧪 Status: Experimental (no releases yet)\n")
	}

	// Mark as inactive when the average age of the last 42 commits exceeds
	// 730 days (~2 years).  A single recent commit (e.g. a deprecation
	// notice) does not rescue a dormant project — ~42 recent commits are
	// required to move the average below the threshold.  This matches the
	// grey-line rule used in the interactive rank-history SVG.
	if metadata.AvgCommitAge > 730 {
		builder.WriteString("\n⚠️  **Notice**: This project appears to be inactive or no longer maintained. The average age of its last 42 commits exceeds 2 years. Use at your own risk.")
	}
	builder.WriteString("\n\n")
}

func writeProjectSummaryContent(builder *strings.Builder, summary ProjectSummary) {
	paragraphs := splitSummaryParagraphs(sanitizeSummaryForGemtext(summary.Summary))

	if len(summary.Images) > 0 {
		// First image after metadata, before text.
		builder.WriteString(fmt.Sprintf("=> %s %s screenshot\n\n", summary.Images[0], summary.Name))

		// First paragraph.
		if len(paragraphs) > 0 {
			builder.WriteString(fmt.Sprintf("%s\n\n", strings.TrimSpace(paragraphs[0])))
		}

		// Second image after first paragraph (if we have 2 images and multiple paragraphs).
		if len(summary.Images) > 1 && len(paragraphs) > 1 {
			builder.WriteString(fmt.Sprintf("=> %s %s screenshot\n\n", summary.Images[1], summary.Name))
		}

		// Remaining paragraphs.
		for i := 1; i < len(paragraphs); i++ {
			builder.WriteString(fmt.Sprintf("%s\n\n", strings.TrimSpace(paragraphs[i])))
		}
		return
	}

	// No images - just add all paragraphs.
	for _, para := range paragraphs {
		builder.WriteString(fmt.Sprintf("%s\n\n", strings.TrimSpace(para)))
	}
}

func writeProjectLinks(builder *strings.Builder, summary ProjectSummary) {
	if summary.CodebergURL != "" {
		builder.WriteString(fmt.Sprintf("=> %s View on Codeberg\n", summary.CodebergURL))
	}
	if summary.GitHubURL != "" {
		builder.WriteString(fmt.Sprintf("=> %s View on GitHub\n", summary.GitHubURL))
	}
	if summary.ForgejoURL != "" {
		builder.WriteString(fmt.Sprintf("=> %s View on Forgejo\n", summary.ForgejoURL))
		if forgejoURL, err := url.Parse(summary.ForgejoURL); err == nil {
			host := strings.ReplaceAll(forgejoURL.Hostname(), ".", " dot ")
			path := strings.ReplaceAll(strings.Trim(forgejoURL.Path, "/"), "/", " slash ")
			builder.WriteString(fmt.Sprintf("For Forgejo access go to %s slash %s\n", host, path))
		}
	}
}
