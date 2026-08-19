package showcase

import (
	"fmt"
	"os/exec"
	"strings"

	"codeberg.org/snonux/gitsyncer/internal/config"
)

// ProjectLinker builds the outbound links (Codeberg/GitHub) shown alongside
// each project in the showcase. It owns all URL construction so Generator
// itself doesn't need to know anything about org config shape or forge URL
// conventions.
type ProjectLinker struct {
	config *config.Config
}

// NewProjectLinker creates a ProjectLinker for the given config. A nil
// config is replaced with a zero-value one so callers (and tests) can
// construct a linker without a fully populated configuration.
func NewProjectLinker(cfg *config.Config) *ProjectLinker {
	if cfg == nil {
		cfg = &config.Config{}
	}
	return &ProjectLinker{config: cfg}
}

// BuildLinks returns the Codeberg and GitHub URLs for repoName, whose local
// working copy lives at repoPath. Any link is left empty when the
// corresponding organization isn't configured (or, for Codeberg, when the
// repository isn't actually set up to sync there).
func (l *ProjectLinker) BuildLinks(repoName, repoPath string) (codebergURL, githubURL string) {
	codebergURL = l.codebergLink(repoName, repoPath)
	githubURL = l.githubLink(repoName)
	return codebergURL, githubURL
}

// codebergLink returns the Codeberg URL for repoName, but only when Codeberg
// syncing is enabled, the repository is part of the configured sync set (the
// repositories allowlist), and the repository is actually wired up to sync
// with Codeberg (a Codeberg remote is configured in its local working copy).
// This prevents repos that merely have a leftover Codeberg remote (e.g. from
// before they were removed from the sync set) from displaying a stale link.
func (l *ProjectLinker) codebergLink(repoName, repoPath string) string {
	cfg := l.config
	if !cfg.CodebergSyncEnabled() || !cfg.IsSyncRepo(repoName) || !repoHasCodebergRemote(repoPath) {
		return ""
	}

	codebergOrg := cfg.FindCodebergOrg()
	if codebergOrg == nil {
		return ""
	}

	return fmt.Sprintf("https://codeberg.org/%s/%s", codebergOrg.Name, repoName)
}

// githubLink returns the GitHub URL for repoName when a GitHub organization
// is configured.
func (l *ProjectLinker) githubLink(repoName string) string {
	githubOrg := l.config.FindGitHubOrg()
	if githubOrg == nil {
		return ""
	}
	return fmt.Sprintf("https://github.com/%s/%s", githubOrg.Name, repoName)
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
