package localrepos

import (
	"os"
	"path/filepath"
)

// ListLocalRepos returns directory names in workDir that contain a .git path.
// The .git path may be either a directory or a file (for worktree layouts).
func ListLocalRepos(workDir string) ([]string, error) {
	return listLocalRepos(workDir, false)
}

// ListLocalReposWithGitDir returns directory names in workDir that contain
// a .git directory.
func ListLocalReposWithGitDir(workDir string) ([]string, error) {
	return listLocalRepos(workDir, true)
}

func listLocalRepos(workDir string, requireGitDir bool) ([]string, error) {
	entries, err := os.ReadDir(workDir)
	if err != nil {
		return nil, err
	}

	repositories := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		gitDir := filepath.Join(workDir, entry.Name(), ".git")
		if info, err := os.Stat(gitDir); err == nil {
			if !requireGitDir || info.IsDir() {
				repositories = append(repositories, entry.Name())
			}
		}
	}

	return repositories, nil
}
