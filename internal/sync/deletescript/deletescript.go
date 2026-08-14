// Package deletescript renders the bash script that deletes abandoned
// branches. It only knows about branch names, last-commit dates and remotes
// — it has no notion of "abandoned" or "protected" branches. That analysis
// lives in the sync package, which decides which branches to report and
// hands this package the resulting values via RepoReport.
package deletescript

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/template"
	"time"
)

//go:embed delete_script.tmpl
var deleteScriptTemplateText string

var deleteScriptTemplate = template.Must(template.New("deleteScript").Parse(deleteScriptTemplateText))

type deleteScriptTemplateData struct {
	GeneratedAt     string
	TotalAbandoned  int
	TotalIgnored    int
	TotalBranches   int
	RepositoryCount int
	ScriptBaseName  string
}

// BranchInfo holds the subset of branch metadata needed to emit the delete
// commands for a single branch: its name, when it was last touched, and
// which remotes still carry it.
type BranchInfo struct {
	Name              string
	LastCommit        time.Time
	RemotesWithBranch []string
}

// RepoReport holds the branches to delete for a single repository, already
// split into the "regular" and "ignored" (exclusion-pattern-matched)
// buckets used by the generated script's headers and review labels.
type RepoReport struct {
	RepoName string
	Regular  []BranchInfo
	Ignored  []BranchInfo
}

// Generate writes a shell script file that deletes every branch listed in
// reports and returns its path. It returns "" (with a nil error) when there
// is nothing to delete, so callers can treat an empty path as "no script was
// needed" rather than an error.
//
// The return value uses named results so that a failure to close the script
// file (e.g. a delayed flush error on a lagging filesystem) is reported via
// err instead of being silently discarded, without masking any earlier, more
// specific error.
func Generate(workDir string, reports []RepoReport) (scriptPath string, err error) {
	totalAbandoned, totalIgnored := countBranches(reports)
	if totalAbandoned == 0 && totalIgnored == 0 {
		return "", nil
	}

	timestamp := time.Now().Format("20060102_150405")
	scriptPath = filepath.Join(workDir, fmt.Sprintf("delete_abandoned_branches_%s.sh", timestamp))
	scriptBaseName := filepath.Base(scriptPath)

	file, err := os.Create(scriptPath)
	if err != nil {
		return "", fmt.Errorf("failed to create script file: %w", err)
	}
	defer func() {
		if cerr := file.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("failed to close script file %s: %w", scriptPath, cerr)
		}
	}()

	if err := writeScriptBody(file, workDir, reports, deleteScriptTemplateData{
		GeneratedAt:     time.Now().Format("2006-01-02 15:04:05"),
		TotalAbandoned:  totalAbandoned,
		TotalIgnored:    totalIgnored,
		TotalBranches:   totalAbandoned + totalIgnored,
		RepositoryCount: len(reports),
		ScriptBaseName:  scriptBaseName,
	}); err != nil {
		return scriptPath, err
	}

	if err := os.Chmod(scriptPath, 0755); err != nil {
		return scriptPath, fmt.Errorf("failed to make script executable: %w", err)
	}

	return scriptPath, nil
}

// countBranches sums the regular and ignored branches across every report.
func countBranches(reports []RepoReport) (totalAbandoned, totalIgnored int) {
	for _, r := range reports {
		totalAbandoned += len(r.Regular)
		totalIgnored += len(r.Ignored)
	}
	return totalAbandoned, totalIgnored
}

// writeScriptBody writes the preamble, one block per repository, and the
// footer to writer. It is split out of Generate so that the file-handling
// concerns (create/chmod/close) stay separate from the content-emission
// logic below.
func writeScriptBody(writer io.Writer, workDir string, reports []RepoReport, data deleteScriptTemplateData) error {
	if err := writeDeleteScriptTemplate(writer, "deleteScriptPreamble", data); err != nil {
		return err
	}

	for _, report := range reports {
		if len(report.Regular) == 0 && len(report.Ignored) == 0 {
			continue
		}
		if err := writeRepoBlock(writer, workDir, report); err != nil {
			return err
		}
	}

	return writeDeleteScriptTemplate(writer, "deleteScriptFooter", deleteScriptTemplateData{
		ScriptBaseName: data.ScriptBaseName,
	})
}

// writeRepoBlock writes one repository's banner followed by its regular and
// ignored branch-deletion blocks.
func writeRepoBlock(writer io.Writer, workDir string, report RepoReport) error {
	if err := writeDeleteScriptRepoHeader(writer, workDir, report.RepoName); err != nil {
		return err
	}

	if len(report.Regular) > 0 {
		if _, err := fmt.Fprintf(writer, "# Regular abandoned branches\n"); err != nil {
			return fmt.Errorf("failed to write regular branches header for %s: %w", report.RepoName, err)
		}
		if err := writeBranchDeletionBlock(writer, report.Regular, "regular", "🔸 Deleting branch: "); err != nil {
			return err
		}
	}

	if len(report.Ignored) > 0 {
		if _, err := fmt.Fprintf(writer, "# Ignored abandoned branches\n"); err != nil {
			return fmt.Errorf("failed to write ignored branches header for %s: %w", report.RepoName, err)
		}
		if err := writeBranchDeletionBlock(writer, report.Ignored, "ignored", "🔹 Deleting ignored branch: "); err != nil {
			return err
		}
	}

	return nil
}

func writeDeleteScriptTemplate(writer io.Writer, templateName string, data deleteScriptTemplateData) error {
	if err := deleteScriptTemplate.ExecuteTemplate(writer, templateName, data); err != nil {
		return fmt.Errorf("failed to execute %s template: %w", templateName, err)
	}

	return nil
}

// writeBranchDeletionBlock writes the bash block that deletes a single
// branch: a review-mode diff, a switch away from the branch if it is
// currently checked out, and the actual remote/local delete commands.
func writeBranchDeletionBlock(writer io.Writer, branches []BranchInfo, reviewBranchType, deleteMessagePrefix string) error {
	for _, branch := range branches {
		if err := writeBranchReviewBlock(writer, branch, reviewBranchType); err != nil {
			return err
		}
		if err := writeBranchDeleteBlock(writer, branch, deleteMessagePrefix); err != nil {
			return err
		}
	}

	return nil
}

// writeBranchReviewBlock writes the "if $MODE is review" half of a branch's
// deletion block, which diffs the branch against main instead of deleting it.
func writeBranchReviewBlock(writer io.Writer, branch BranchInfo, reviewBranchType string) error {
	if _, err := fmt.Fprintf(writer, "if [[ \"$MODE\" == \"review\" || \"$MODE\" == \"review-full\" ]]; then\n"); err != nil {
		return fmt.Errorf("failed to write review mode condition for branch %s: %w", branch.Name, err)
	}
	if _, err := fmt.Fprintf(writer, "    if [[ -n \"$main_branch\" ]]; then\n"); err != nil {
		return fmt.Errorf("failed to write main branch check for branch %s: %w", branch.Name, err)
	}
	if _, err := fmt.Fprintf(writer, "        review_branch \"%s\" \"$main_branch\" \"%s\" \"%s\"\n", branch.Name, branch.LastCommit.Format("2006-01-02"), reviewBranchType); err != nil {
		return fmt.Errorf("failed to write review command for branch %s: %w", branch.Name, err)
	}
	if _, err := fmt.Fprintf(writer, "    fi\n"); err != nil {
		return fmt.Errorf("failed to write review branch end for branch %s: %w", branch.Name, err)
	}
	if _, err := fmt.Fprintf(writer, "else\n"); err != nil {
		return fmt.Errorf("failed to write delete branch condition for branch %s: %w", branch.Name, err)
	}

	return nil
}

// writeBranchDeleteBlock writes the "else" half of a branch's deletion
// block: switch off the branch if it is currently checked out, then delete
// it from every remote that has it and locally.
func writeBranchDeleteBlock(writer io.Writer, branch BranchInfo, deleteMessagePrefix string) error {
	if _, err := fmt.Fprintf(writer, "    echo \"  %s%s (last commit: %s)\"\n", deleteMessagePrefix, branch.Name, branch.LastCommit.Format("2006-01-02")); err != nil {
		return fmt.Errorf("failed to write delete message for branch %s: %w", branch.Name, err)
	}
	if err := writeBranchSwitchGuard(writer, branch); err != nil {
		return err
	}
	for _, remote := range branch.RemotesWithBranch {
		if _, err := fmt.Fprintf(writer, "    execute_cmd git push %s --delete \"%s\"\n", remote, branch.Name); err != nil {
			return fmt.Errorf("failed to write remote delete command for branch %s: %w", branch.Name, err)
		}
	}
	if _, err := fmt.Fprintf(writer, "    execute_cmd git branch -D \"%s\"\n", branch.Name); err != nil {
		return fmt.Errorf("failed to write local delete command for branch %s: %w", branch.Name, err)
	}
	if _, err := fmt.Fprintf(writer, "fi\n\n"); err != nil {
		return fmt.Errorf("failed to write branch block end for branch %s: %w", branch.Name, err)
	}

	return nil
}

// writeBranchSwitchGuard writes the bash that switches off the branch being
// deleted (checking out main/master first) if it happens to be the branch
// currently checked out, and skips deletion entirely if no main branch was
// found to switch to.
func writeBranchSwitchGuard(writer io.Writer, branch BranchInfo) error {
	if err := writeBranchSwitchAttempt(writer, branch); err != nil {
		return err
	}
	return writeBranchSwitchSkipIfFailed(writer, branch)
}

// writeBranchSwitchAttempt writes the "if we're on the branch being deleted,
// switch to main/master (or warn if there is none)" block.
func writeBranchSwitchAttempt(writer io.Writer, branch BranchInfo) error {
	if _, err := fmt.Fprintf(writer, "    # Check if we're on the branch to be deleted\n"); err != nil {
		return fmt.Errorf("failed to write current branch comment for branch %s: %w", branch.Name, err)
	}
	if _, err := fmt.Fprintf(writer, "    current_branch=$(git branch --show-current)\n"); err != nil {
		return fmt.Errorf("failed to write current branch command for branch %s: %w", branch.Name, err)
	}
	if _, err := fmt.Fprintf(writer, "    if [[ \"$current_branch\" == \"%s\" ]]; then\n", branch.Name); err != nil {
		return fmt.Errorf("failed to write current branch condition for branch %s: %w", branch.Name, err)
	}
	if _, err := fmt.Fprintf(writer, "        echo \"    Switching from %s to main/master branch before deletion...\"\n", branch.Name); err != nil {
		return fmt.Errorf("failed to write branch switch message for branch %s: %w", branch.Name, err)
	}
	if _, err := fmt.Fprintf(writer, "        main_branch=$(find_main_branch)\n"); err != nil {
		return fmt.Errorf("failed to write find main branch command for branch %s: %w", branch.Name, err)
	}
	if _, err := fmt.Fprintf(writer, "        if [[ -n \"$main_branch\" ]]; then\n"); err != nil {
		return fmt.Errorf("failed to write branch switch check for branch %s: %w", branch.Name, err)
	}
	if _, err := fmt.Fprintf(writer, "            execute_cmd git checkout \"$main_branch\"\n"); err != nil {
		return fmt.Errorf("failed to write checkout command for branch %s: %w", branch.Name, err)
	}
	if _, err := fmt.Fprintf(writer, "        else\n"); err != nil {
		return fmt.Errorf("failed to write missing main branch else block for branch %s: %w", branch.Name, err)
	}
	if _, err := fmt.Fprintf(writer, "            echo \"    ⚠️  No main/master branch found to switch to!\"\n"); err != nil {
		return fmt.Errorf("failed to write missing main branch message for branch %s: %w", branch.Name, err)
	}
	if _, err := fmt.Fprintf(writer, "            echo \"    Skipping deletion of %s\"\n", branch.Name); err != nil {
		return fmt.Errorf("failed to write skip branch message for branch %s: %w", branch.Name, err)
	}
	if _, err := fmt.Fprintf(writer, "        fi\n"); err != nil {
		return fmt.Errorf("failed to write main branch switch end for branch %s: %w", branch.Name, err)
	}
	if _, err := fmt.Fprintf(writer, "    fi\n"); err != nil {
		return fmt.Errorf("failed to write current branch condition end for branch %s: %w", branch.Name, err)
	}

	return nil
}

// writeBranchSwitchSkipIfFailed writes the guard that skips straight to the
// next branch when we were on the branch being deleted but had no
// main/master branch to switch to first.
func writeBranchSwitchSkipIfFailed(writer io.Writer, branch BranchInfo) error {
	if _, err := fmt.Fprintf(writer, "    # Skip to next branch if we couldn't switch\n"); err != nil {
		return fmt.Errorf("failed to write skip comment for branch %s: %w", branch.Name, err)
	}
	if _, err := fmt.Fprintf(writer, "    if [[ \"$current_branch\" == \"%s\" ]] && [[ -z \"$main_branch\" ]]; then\n", branch.Name); err != nil {
		return fmt.Errorf("failed to write skip condition for branch %s: %w", branch.Name, err)
	}
	if _, err := fmt.Fprintf(writer, "        continue\n"); err != nil {
		return fmt.Errorf("failed to write continue for branch %s: %w", branch.Name, err)
	}
	if _, err := fmt.Fprintf(writer, "    fi\n"); err != nil {
		return fmt.Errorf("failed to write skip condition end for branch %s: %w", branch.Name, err)
	}

	return nil
}

// writeDeleteScriptRepoHeader writes the per-repository banner and the
// review-mode main-branch check at the top of each repository's block in
// the generated delete script.
func writeDeleteScriptRepoHeader(writer io.Writer, workDir, repoName string) error {
	lines := []string{
		"# ======================================\n",
		fmt.Sprintf("# Repository: %s\n", repoName),
		"# ======================================\n",
		"echo\n",
		fmt.Sprintf("echo \"📁 Processing repository: %s\"\n", repoName),
		fmt.Sprintf("cd \"%s/%s\" || { echo \"Failed to change to repository directory\"; exit 1; }\n\n", workDir, repoName),
		"if [[ \"$MODE\" == \"review\" || \"$MODE\" == \"review-full\" ]]; then\n",
		"    main_branch=$(find_main_branch)\n",
		"    if [[ -z \"$main_branch\" ]]; then\n",
		fmt.Sprintf("        echo -e \"${RED}⚠️  No main/master branch found in %s${NC}\"\n", repoName),
		"    fi\n",
		"fi\n\n",
	}
	for _, line := range lines {
		if _, err := fmt.Fprint(writer, line); err != nil {
			return fmt.Errorf("failed to write repository header for %s: %w", repoName, err)
		}
	}
	return nil
}
