package release

import (
	"fmt"
	"regexp"
	"strings"

	"codeberg.org/snonux/gitsyncer/internal/aitool"
)

// featCommitRE and fixCommitRE recognize Conventional Commit subjects for
// features and fixes, with an optional (scope) between the type and the
// colon (e.g. "feat(sync): add X", "fix(config): correct Y"). The scope
// group is intentionally unconstrained ([^)]*) since callers may use any
// text there (package name, component, etc). Matching is case-insensitive
// so "Feat:" / "FIX:" style subjects are still recognized.
var (
	featCommitRE = regexp.MustCompile(`(?i)^(feat|feature)(\([^)]*\))?:`)
	fixCommitRE  = regexp.MustCompile(`(?i)^(fix|bugfix)(\([^)]*\))?:`)
)

// categorizeCommits buckets commit subjects into features, fixes, and
// everything else ("other") based on their Conventional Commit type prefix.
// Both unscoped ("feat: add X") and scoped ("feat(sync): add X") subjects
// are recognized; anything that doesn't match a known feat/fix prefix falls
// into "other". Split out from GenerateReleaseNotes so the categorization
// rules can be unit-tested independently of git/tag plumbing.
func categorizeCommits(commits []string) (features, fixes, other []string) {
	for _, commit := range commits {
		switch {
		case featCommitRE.MatchString(commit):
			features = append(features, commit)
		case fixCommitRE.MatchString(commit):
			fixes = append(fixes, commit)
		default:
			other = append(other, commit)
		}
	}
	return features, fixes, other
}

// NotesGenerator builds release notes from git history. It composes a
// GitInspector for the commit/diff input and an AI tool chain for the prose
// output, with a deterministic fallback when no AI tool is configured or all
// of them fail.
type NotesGenerator struct {
	aiTool string
	git    *GitInspector
}

// NewNotesGenerator creates a notes generator backed by the given inspector.
// The AI tool preference is optional; an empty value uses the default chain.
func NewNotesGenerator(aiTool string, git *GitInspector) *NotesGenerator {
	if git == nil {
		git = NewGitInspector()
	}
	return &NotesGenerator{aiTool: aiTool, git: git}
}

// SetAITool sets the preferred AI tool for release notes generation.
func (n *NotesGenerator) SetAITool(tool string) {
	n.aiTool = tool
}

// GenerateReleaseNotes generates structured (non-AI) release notes from the
// commits between the previous tag and the given tag.
func (n *NotesGenerator) GenerateReleaseNotes(repoPath, tag string, allTags []string) string {
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
	commits, err := n.git.GetCommitsSinceTag(repoPath, prevTag, tag)
	if err != nil {
		return fmt.Sprintf("Release %s", tag)
	}

	if len(commits) == 0 {
		return fmt.Sprintf("Release %s", tag)
	}

	// Group commits by Conventional Commit type.
	features, fixes, other := categorizeCommits(commits)

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

	return notes.String()
}

// GenerateAIReleaseNotes generates prose release notes using an AI tool, with
// fallback across the configured tool chain.
func (n *NotesGenerator) GenerateAIReleaseNotes(repoPath, repoName, tag string, allTags []string, commits []string) (string, error) {
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
	diff, err := n.git.GetDiffBetweenTags(repoPath, prevTag, tag)
	if err != nil {
		return "", fmt.Errorf("failed to get diff: %w", err)
	}

	// Prepare prompt/instructions and input payload
	var instr strings.Builder
	instr.WriteString(fmt.Sprintf("Generate professional release notes for %s version %s.\n", repoName, tag))
	if prevTag != "" {
		instr.WriteString(fmt.Sprintf("Previous version: %s\n", prevTag))
	}
	instr.WriteString("\nBased on the provided commits and code changes, write professional release notes that:\n")
	instr.WriteString("1. Start with a brief overview of what this release accomplishes\n")
	instr.WriteString("2. Group changes into logical sections (Features, Improvements, Bug Fixes, etc.)\n")
	instr.WriteString("3. Explain WHY each change is useful to users, not just what changed\n")
	instr.WriteString("4. Use clear, non-technical language where possible\n")
	instr.WriteString("5. Highlight any breaking changes or migration steps\n")
	instr.WriteString("6. Keep it concise but informative\n")
	instr.WriteString("7. Format using Markdown\n")
	instr.WriteString("\nDo not include the version number in the title as it will be added automatically.")

	var input strings.Builder
	input.WriteString("Commit messages:\n")
	for _, commit := range commits {
		input.WriteString(fmt.Sprintf("- %s\n", commit))
	}
	input.WriteString("\nCode changes:\n")
	input.WriteString(diff)

	fmt.Printf("  Prompt: Generate release notes for %s %s\n", repoName, tag)
	fmt.Printf("  Prompt includes: %d commits, %.1fKB of code changes\n", len(commits), float64(len(diff))/1024)
	fmt.Printf("  Total prompt length: %d characters\n", len(instr.String())+len(input.String()))

	// Run each available AI tool in turn until one produces output. Command
	// construction, stdin/argument conventions, and error screening for each
	// tool live in aitool.RunChain/Runner so this package doesn't need a
	// type switch over aitool.Tool: instr is the instructional prompt, input
	// (commits + diff) is the payload, and each Runner decides for itself
	// whether its underlying CLI wants that payload combined into a single
	// argument (opencode, claude) or piped via stdin (hexai, amp).
	releaseNotes, _, err := aitool.RunChain(n.availableReleaseNotesTools(nil), "", instr.String(), input.String())
	if err != nil {
		return "", fmt.Errorf("all AI tools failed to generate release notes")
	}

	// Add header
	var finalNotes strings.Builder
	finalNotes.WriteString(fmt.Sprintf("# Release %s\n\n", tag))
	finalNotes.WriteString(releaseNotes)

	return finalNotes.String(), nil
}

// availableReleaseNotesTools returns the AI tools from the configured chain
// that are actually installed on this host. Exported via a method so tests can
// drive it with a fake LookPath.
func (n *NotesGenerator) availableReleaseNotesTools(lookPath aitool.LookPathFunc) []aitool.Tool {
	return aitool.AvailableChain(n.aiTool, lookPath)
}
