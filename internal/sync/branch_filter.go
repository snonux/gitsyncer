package sync

import (
	"fmt"
	"regexp"
	"strings"
)

// BranchFilter handles branch filtering based on exclusion patterns
type BranchFilter struct {
	excludePatterns []*regexp.Regexp
}

// NewBranchFilter creates a new branch filter from exclusion patterns
func NewBranchFilter(excludePatterns []string) (*BranchFilter, error) {
	filter := &BranchFilter{
		excludePatterns: make([]*regexp.Regexp, 0, len(excludePatterns)),
	}

	// Compile regex patterns
	for _, pattern := range excludePatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid regex pattern '%s': %w", pattern, err)
		}
		filter.excludePatterns = append(filter.excludePatterns, re)
	}

	return filter, nil
}

// ShouldExclude checks if a branch should be excluded based on the patterns
func (f *BranchFilter) ShouldExclude(branchName string) bool {
	for _, pattern := range f.excludePatterns {
		if pattern.MatchString(branchName) {
			return true
		}
	}
	return false
}

// FilterBranches filters a list of branches, removing excluded ones
func (f *BranchFilter) FilterBranches(branches []string) []string {
	if len(f.excludePatterns) == 0 {
		return branches
	}

	filtered := make([]string, 0, len(branches))
	for _, branch := range branches {
		if !f.ShouldExclude(branch) {
			filtered = append(filtered, branch)
		}
	}
	return filtered
}

// GetExcludedBranches returns a list of branches that were excluded
func (f *BranchFilter) GetExcludedBranches(branches []string) []string {
	if len(f.excludePatterns) == 0 {
		return nil
	}

	excluded := make([]string, 0)
	for _, branch := range branches {
		if f.ShouldExclude(branch) {
			excluded = append(excluded, branch)
		}
	}
	return excluded
}

// FormatExclusionReport formats a report of excluded branches
func FormatExclusionReport(excludedBranches []string, patterns []string) string {
	if len(excludedBranches) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n🚫 Excluded %d branches based on patterns:\n", len(excludedBranches)))
	
	// Show patterns
	sb.WriteString("   Patterns: ")
	for i, pattern := range patterns {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(fmt.Sprintf("'%s'", pattern))
	}
	sb.WriteString("\n")
	
	// Show excluded branches
	sb.WriteString("   Excluded branches:\n")
	for _, branch := range excludedBranches {
		sb.WriteString(fmt.Sprintf("   - %s\n", branch))
	}
	
	return sb.String()
}