package forge

import "fmt"

// OwnerType identifies whether a repository owner is a user or organization.
type OwnerType string

const (
	// OwnerTypeUser creates repositories for the authenticated user.
	OwnerTypeUser OwnerType = "user"
	// OwnerTypeOrganization creates repositories in the named organization.
	OwnerTypeOrganization OwnerType = "organization"
)

// Valid reports whether the owner type is supported.
func (t OwnerType) Valid() bool {
	return t == OwnerTypeUser || t == OwnerTypeOrganization
}

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

// ReleaseClient defines release CRUD operations shared across forges. Each
// forge (GitHub, Codeberg/Gitea) implements this against its own API so the
// release pipeline can talk to any forge through a single abstraction
// instead of hand-rolled per-forge HTTP in the caller.
type ReleaseClient interface {
	// GetReleases returns the tag names of existing releases for the
	// repository. A missing repository is reported as an empty slice and a
	// nil error so callers can treat it as "no releases yet".
	GetReleases(owner, repo string) ([]string, error)
	CreateRelease(owner, repo, tag, releaseNotes string) error
	UpdateRelease(owner, repo, tag, releaseNotes string) error
}

// ReleasesEnabler is optionally implemented by forges that require releases
// to be enabled per-repository before release creation can succeed (e.g.
// Codeberg/Gitea, where the repository's Releases feature may be disabled).
// Callers check for this interface rather than assuming every forge needs it.
type ReleasesEnabler interface {
	EnsureReleasesEnabled(owner, repo string) error
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
