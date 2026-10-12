package showcase

import (
	"fmt"

	"github.com/snonux/gitsyncer/internal/config"
)

// ProjectLinker builds the GitHub links shown alongside each project in the
// showcase. It owns URL construction so Generator does not need to know about
// the organization configuration format.
type ProjectLinker struct {
	config *config.Config
}

// NewProjectLinker creates a ProjectLinker for the given config. A nil
// config is replaced with a zero-value one so callers and tests can construct
// a linker without a fully populated configuration.
func NewProjectLinker(cfg *config.Config) *ProjectLinker {
	if cfg == nil {
		cfg = &config.Config{}
	}
	return &ProjectLinker{config: cfg}
}

// BuildLink returns the GitHub URL for repoName when a GitHub organization is
// configured.
func (l *ProjectLinker) BuildLink(repoName string) string {
	githubOrg := l.config.FindGitHubOrg()
	if githubOrg == nil {
		return ""
	}
	return fmt.Sprintf("https://github.com/%s/%s", githubOrg.Name, repoName)
}
