package cli

import (
	"codeberg.org/snonux/gitsyncer/internal/codeberg"
	"codeberg.org/snonux/gitsyncer/internal/config"
	"codeberg.org/snonux/gitsyncer/internal/forge"
	"codeberg.org/snonux/gitsyncer/internal/github"
)

func newRepoClientForOrg(org config.Organization) (forge.RepoClient, bool) {
	switch org.Host {
	case "git@github.com":
		client := github.NewClient(org.GitHubToken, org.Name)
		return &client, true
	case "git@codeberg.org":
		client := codeberg.NewClient(org.Name, org.CodebergToken)
		return &client, true
	default:
		return nil, false
	}
}
