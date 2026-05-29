package forge

import "fmt"

// RepoClient defines shared repository lifecycle operations across forges.
type RepoClient interface {
	HasToken() bool
	RepoExists(repoName string) (bool, error)
	CreateRepo(repoName, description string, private bool) error
	DeleteRepo(repoName string) error
}

// RepoDescriptionClient defines shared description operations across forges.
type RepoDescriptionClient interface {
	RepoClient
	GetRepoDescription(repoName string) (string, bool, error)
	UpdateRepoDescription(repoName, description string) error
}

// RepoExistsFunc checks if a repository exists.
type RepoExistsFunc func(repoName string) (bool, error)

// CheckRepoExists normalizes repo-existence check error handling.
func CheckRepoExists(repoName string, existsFn RepoExistsFunc) (bool, error) {
	exists, err := existsFn(repoName)
	if err != nil {
		return false, fmt.Errorf("failed to check if repo exists: %w", err)
	}

	return exists, nil
}

// EnsureRepoExists validates that a repository exists before destructive actions.
func EnsureRepoExists(org, repoName string, existsFn RepoExistsFunc) error {
	exists, err := CheckRepoExists(repoName, existsFn)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("repository %s/%s does not exist", org, repoName)
	}

	return nil
}

// DeleteStatusError maps common forge delete status codes into stable errors.
func DeleteStatusError(statusCode int, body string) error {
	switch statusCode {
	case 204, 404:
		return nil
	case 403:
		return fmt.Errorf("permission denied (403): %s", body)
	case 401:
		return fmt.Errorf("authentication failed (401): %s", body)
	default:
		return fmt.Errorf("failed to delete repository: status %d: %s", statusCode, body)
	}
}
