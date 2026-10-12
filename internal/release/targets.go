package release

// Release targets: resolving which forges (GitHub/Forgejo) releases
// should be published to, and the per-forge token/applicability rules around
// that. This was internal/cli/release.go's buildReleaseTargets and friends
// before task w01 moved the release-orchestration logic that belongs in this
// package out of the CLI layer.

import (
	"fmt"

	"github.com/snonux/gitsyncer/internal/config"
	"github.com/snonux/gitsyncer/internal/forge"
	"github.com/snonux/gitsyncer/internal/forgejo"
	"github.com/snonux/gitsyncer/internal/github"
)

// Target pairs a forge release client with the owner that releases should be
// created under. The client owns all HTTP/auth/404/409 handling; the caller
// only orchestrates which tags to create or update.
type Target struct {
	Name   string
	Owner  string
	Client forge.ReleaseClient
}

// BuildReleaseTargets resolves the GitHub and Forgejo tokens (from
// config, falling back to environment variables and token files) and returns
// the release targets releases should be published to. A forge is included
// only when its organization is configured in cfg.
func BuildReleaseTargets(cfg *config.Config) []Target {
	var targets []Target

	if target, ok := buildGitHubReleaseTarget(cfg); ok {
		targets = append(targets, target)
	}
	if target, ok := buildForgejoReleaseTarget(cfg); ok {
		targets = append(targets, target)
	}

	return targets
}

// buildGitHubReleaseTarget resolves the GitHub org and token from cfg and
// returns the corresponding release target. ok is false when no GitHub org
// is configured, or the configured org has no name.
func buildGitHubReleaseTarget(cfg *config.Config) (Target, bool) {
	githubOrg := cfg.FindGitHubOrg()
	if githubOrg == nil {
		fmt.Println("No GitHub organization found in config")
		return Target{}, false
	}
	fmt.Printf("Found GitHub org: %s\n", githubOrg.Name)

	// Try config token first, then fallback to env var and file
	token := resolveGitHubToken(githubOrg.GitHubToken)
	if token == "" {
		fmt.Println("WARNING: No GitHub token found - cannot create GitHub releases")
	}

	if githubOrg.Name == "" {
		return Target{}, false
	}
	return Target{
		Name:   "GitHub",
		Owner:  githubOrg.Name,
		Client: github.NewClient(token, githubOrg.Name),
	}, true
}

// buildForgejoReleaseTarget resolves the Forgejo org from cfg and returns the
// corresponding release target. ok is false when no Forgejo org is configured
// or the configured org has no forgejo_owner.
func buildForgejoReleaseTarget(cfg *config.Config) (Target, bool) {
	forgejoOrg := cfg.FindForgejoOrg()
	if forgejoOrg == nil {
		fmt.Println("No Forgejo organization found in config")
		return Target{}, false
	}
	fmt.Printf("Found Forgejo org: %s\n", forgejoOrg.ForgejoOwner)

	client := forgejo.NewForgejoClient(forgejoOrg.ForgejoAPIBase, forgejoOrg.ForgejoOwner, forgejoOrg.ForgejoOwnerType)
	if client.HasToken() {
		fmt.Println("  Forgejo token loaded")
	} else {
		fmt.Println("WARNING: No Forgejo token found - cannot create Forgejo releases")
	}

	if forgejoOrg.ForgejoOwner == "" {
		return Target{}, false
	}
	return Target{
		Name:   "Forgejo",
		Owner:  forgejoOrg.ForgejoOwner,
		Client: client,
	}, true
}

// resolveGitHubToken loads the GitHub token from config, the GITHUB_TOKEN env
// var, or ~/.gitsyncer_github_token, in that order. The cascade itself lives
// in forge.ResolveToken - the single source of truth shared with the GitHub
// and Forgejo clients' own token methods - so this is a thin,
// forge-specific wrapper rather than a reimplementation.
func resolveGitHubToken(configToken string) string {
	return forge.ResolveToken(configToken, "GITHUB_TOKEN", ".gitsyncer_github_token")
}

// releasesEnabler returns the target's ReleasesEnabler implementation, or nil
// if the forge does not need per-repository release enabling (e.g. GitHub).
func releasesEnabler(target Target) forge.ReleasesEnabler {
	if enabler, ok := target.Client.(forge.ReleasesEnabler); ok {
		return enabler
	}
	return nil
}
