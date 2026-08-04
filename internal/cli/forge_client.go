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
		client := codeberg.NewGiteaClient(org.ForgejoAPIBase, org.ForgejoToken(), org.ForgejoOwner, "Forgejo")
		return client, true
	default:
		return nil, false
	}
}
