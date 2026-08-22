package release

// Release targets: resolving which forges (GitHub/Codeberg) releases should
// be published to, and the per-forge token/applicability rules around that.
// This was internal/cli/release.go's buildReleaseTargets and friends before
// task w01 moved the release-orchestration logic that belongs in this
// package out of the CLI layer.

import (
	"fmt"

	"github.com/snonux/gitsyncer/internal/codeberg"
	"github.com/snonux/gitsyncer/internal/config"
	"github.com/snonux/gitsyncer/internal/forge"
	"github.com/snonux/gitsyncer/internal/github"
)

// Target pairs a forge release client with the owner that releases should be
// created under. The client owns all HTTP/auth/404/409 handling; the caller
// only orchestrates which tags to create or update.
type Target struct {
	Name             string
	Owner            string
	Client           forge.ReleaseClient
	SyncRepoRequired bool
}

// BuildReleaseTargets resolves the GitHub and Codeberg tokens (from config,
// falling back to environment variables and token files) and returns the
// release targets releases should be published to. A forge is included only
// when its organization is configured in cfg; Codeberg is additionally
// skipped when Codeberg sync is disabled.
func BuildReleaseTargets(cfg *config.Config) []Target {
	var targets []Target

	if target, ok := buildGitHubReleaseTarget(cfg); ok {
		targets = append(targets, target)
	}
	if target, ok := buildCodebergReleaseTarget(cfg); ok {
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

// buildCodebergReleaseTarget resolves the Codeberg org and token from cfg and
// returns the corresponding release target. ok is false when no Codeberg org
// is configured, Codeberg sync is disabled, or the configured org has no
// name.
func buildCodebergReleaseTarget(cfg *config.Config) (Target, bool) {
	codebergOrg := cfg.FindCodebergOrg()
	if codebergOrg != nil && !cfg.CodebergSyncEnabled() {
		fmt.Println("Codeberg organization found in config but Codeberg sync is disabled (set \"sync_codeberg\": true to enable Codeberg releases)")
		codebergOrg = nil
	} else if codebergOrg == nil {
		fmt.Println("No Codeberg organization found in config")
	}
	// codebergOrg is nil here either because no Codeberg org is configured or
	// because Codeberg syncing is disabled in the config; the relevant
	// message has already been printed above.
	if codebergOrg == nil {
		return Target{}, false
	}
	fmt.Printf("Found Codeberg org: %s\n", codebergOrg.Name)

	// Try config token first, then fallback to env var and file
	token := resolveCodebergToken(codebergOrg.CodebergToken)
	if token != "" {
		fmt.Printf("  Codeberg token loaded (length: %d)\n", len(token))
	} else {
		fmt.Println("WARNING: No Codeberg token found - cannot create Codeberg releases")
	}

	if codebergOrg.Name == "" {
		return Target{}, false
	}
	return Target{
		Name:             "Codeberg",
		Owner:            codebergOrg.Name,
		Client:           codeberg.NewClient(token, codebergOrg.Name),
		SyncRepoRequired: true,
	}, true
}

// resolveGitHubToken loads the GitHub token from config, the GITHUB_TOKEN env
// var, or ~/.gitsyncer_github_token, in that order. The cascade itself lives
// in forge.ResolveToken - the single source of truth shared with the GitHub
// and Codeberg clients' own loadToken methods - so this is a thin,
// forge-specific wrapper rather than a reimplementation.
func resolveGitHubToken(configToken string) string {
	return forge.ResolveToken(configToken, "GITHUB_TOKEN", ".gitsyncer_github_token")
}

// resolveCodebergToken loads the Codeberg token from config, the
// CODEBERG_TOKEN env var, or ~/.gitsyncer_codeberg_token, in that order, via
// the shared forge.ResolveToken cascade (see resolveGitHubToken).
func resolveCodebergToken(configToken string) string {
	return forge.ResolveToken(configToken, "CODEBERG_TOKEN", ".gitsyncer_codeberg_token")
}

// TargetApplicable reports whether a release target should run for the
// given repository. Targets marked SyncRepoRequired (e.g. Codeberg) only run
// for repos in the configured sync allowlist, so releases are not created on
// Codeberg for repos that are not synced there.
func TargetApplicable(target Target, cfg *config.Config, repoName string) bool {
	if !target.SyncRepoRequired {
		return true
	}
	return cfg.IsSyncRepo(repoName)
}

// releasesEnabler returns the target's ReleasesEnabler implementation, or nil
// if the forge does not need per-repository release enabling (e.g. GitHub).
func releasesEnabler(target Target) forge.ReleasesEnabler {
	if enabler, ok := target.Client.(forge.ReleasesEnabler); ok {
		return enabler
	}
	return nil
}
