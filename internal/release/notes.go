package release

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"codeberg.org/snonux/gitsyncer/internal/aitool"
)

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

	// Build a full prompt string for tools that read a single argument
	fullPrompt := instr.String() + "\n\n" + input.String()

	var releaseNotes string

	for _, tool := range n.availableReleaseNotesTools(nil) {
		switch tool {
		case aitool.ToolOpencode:
			fmt.Println("  Running ollama launch opencode ...")
			cmd := exec.Command("ollama", "launch", "opencode", "--model", "glm-5.2:cloud", "-y", "--", "run", fullPrompt)
			cmd.Stderr = os.Stderr
			out, err := cmd.Output()
			if err != nil {
				fmt.Printf("opencode ollama failed: %v\n", err)
				continue
			}
			notes := strings.TrimSpace(string(out))
			if notes == "" {
				fmt.Println("  ollama opencode returned empty output; will try fallbacks...")
				continue
			}
			releaseNotes = notes
		case aitool.ToolHexAI:
			fmt.Println("  Running hexai CLI command (stdin payload)...")
			cmd := exec.Command("hexai", instr.String())
			cmd.Stdin = strings.NewReader(input.String())
			cmd.Stderr = os.Stderr
			out, err := cmd.Output()
			if err != nil {
				fmt.Printf("  hexai CLI failed: %v\n", err)
				continue
			}
			notes := strings.TrimSpace(string(out))
			if notes == "" {
				fmt.Println("  hexai returned empty output; will try fallbacks...")
				continue
			}
			releaseNotes = notes
		case aitool.ToolClaude:
			fmt.Println("  Running claude CLI command...")
			cmd := exec.Command("claude", "--model", "sonnet", fullPrompt)
			cmd.Env = append(os.Environ(), "CLAUDE_DEBUG=1")
			notes, err := n.executeAICommand(cmd, string(tool))
			if err != nil {
				fmt.Printf("  Claude CLI failed: %v\n", err)
				continue
			}
			releaseNotes = notes
		case aitool.ToolAmp:
			// Note: print stderr to console, but only use stdout for notes
			fmt.Println("  Running amp CLI command (stdin payload)...")
			cmd := exec.Command("amp", "--execute", instr.String())
			cmd.Stdin = strings.NewReader(input.String())
			cmd.Stderr = os.Stderr
			out, err := cmd.Output()
			if err != nil {
				fmt.Printf("  amp CLI failed: %v\n", err)
				continue
			}
			notes := strings.TrimSpace(string(out))
			if notes == "" {
				fmt.Println("  amp returned empty output; will try fallbacks...")
				continue
			}
			releaseNotes = notes
		}

		if releaseNotes != "" {
			break
		}
	}

	if releaseNotes == "" {
		return "", fmt.Errorf("all AI tools failed to generate release notes")
	}

	// Add header
	var finalNotes strings.Builder
	finalNotes.WriteString(fmt.Sprintf("# Release %s\n\n", tag))
	finalNotes.WriteString(releaseNotes)

	return finalNotes.String(), nil
}

// executeAICommand executes an AI command and returns the trimmed output or an
// error. It also screens the output for common error indicators so a tool that
// exits 0 while printing an error is still treated as a failure.
func (n *NotesGenerator) executeAICommand(cmd *exec.Cmd, toolName string) (string, error) {
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s command failed: %w. Output: %s", toolName, err, string(output))
	}

	content := strings.TrimSpace(string(output))
	if content == "" {
		return "", fmt.Errorf("received empty output from %s", toolName)
	}

	// Check for common error indicators in the output
	if strings.HasPrefix(content, "Error:") ||
		(toolName == "claude" && strings.Contains(content, "API Error")) ||
		(toolName == "claude" && strings.Contains(content, "authentication_error")) {
		return "", fmt.Errorf("%s returned an error: %s", toolName, content)
	}

	return content, nil
}

// availableReleaseNotesTools returns the AI tools from the configured chain
// that are actually installed on this host. Exported via a method so tests can
// drive it with a fake LookPath.
func (n *NotesGenerator) availableReleaseNotesTools(lookPath aitool.LookPathFunc) []aitool.Tool {
	return aitool.AvailableChain(n.aiTool, lookPath)
}
