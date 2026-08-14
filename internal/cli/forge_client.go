package cli

import (
	"codeberg.org/snonux/gitsyncer/internal/codeberg"
	"codeberg.org/snonux/gitsyncer/internal/config"
	"codeberg.org/snonux/gitsyncer/internal/forge"
	"codeberg.org/snonux/gitsyncer/internal/github"
)

func newRepoClientForOrg(org config.Organization) (forge.RepoClient, bool) {
	switch {
	case org.IsGitHub():
		client := github.NewClient(org.GitHubToken, org.Name)
		return client, true
	case org.IsCodeberg():
		client := codeberg.NewClient(org.CodebergToken, org.Name)
		return client, true
	case org.IsForgejo():
		client := codeberg.NewForgejoClient(org.ForgejoAPIBase, org.ForgejoOwner, org.ForgejoOwnerType)
		return client, true
	default:
		return nil, false
	}
}

// newForgejoBackupClient constructs a Forgejo client for the given backup
// organization, satisfying forge.PublicRepoEnsurer. It is injected into
// sync.Syncer via SetForgejoBackupClientFactory so internal/sync can ensure
// backup repositories exist without importing the concrete codeberg package
// itself; internal/cli is the composition root that knows which concrete
// forge client to build, same as newRepoClientForOrg above.
func newForgejoBackupClient(org *config.Organization) forge.PublicRepoEnsurer {
	return codeberg.NewForgejoClient(org.ForgejoAPIBase, org.ForgejoOwner, org.ForgejoOwnerType)
}
