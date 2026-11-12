package cli

import (
	"fmt"
	"strings"

	"github.com/snonux/gitsyncer/internal/config"
	"github.com/snonux/gitsyncer/internal/forge"
)

// githubRepoSelector returns how the GitHub discovery commands narrow or
// extend the list of discovered public repositories before syncing:
//
//   - sync_all_github_repos: every discovered repository, plus the configured
//     repositories as extras (see withConfiguredExtras).
//   - a pushOnly mirror: every discovered repository, untouched (nil).
//   - otherwise: only discovered repositories named in the configured
//     repositories allowlist.
func githubRepoSelector(cfg *config.Config, lister forge.PublicRepoLister) func([]string) []string {
	switch {
	case cfg.SyncsAllGitHubRepos():
		return func(discovered []string) []string {
			return withConfiguredExtras(cfg.Repositories, discovered, lister)
		}
	case cfg.MirrorsAllGitHubRepos():
		return nil
	default:
		return func(discovered []string) []string {
			return applyRepositoryAllowlist(cfg, discovered)
		}
	}
}

// applyRepositoryAllowlist restricts discovered to the configured
// repositories (see Config.FilterSyncRepos) and reports when it did.
func applyRepositoryAllowlist(cfg *config.Config, discovered []string) []string {
	filtered := cfg.FilterSyncRepos(discovered)
	if len(filtered) < len(discovered) {
		fmt.Printf("Restricted to %d configured repositories (allowlist)\n", len(filtered))
	}
	return filtered
}

// withConfiguredExtras appends the configured repositories that discovery
// did not return (e.g. forks, which the public listing skips) to discovered.
//
// Each extra is looked up on GitHub first and only added when it can be
// synced: a repository that does not exist, was renamed, is archived
// (read-only, so every push would fail) or is private (it would be published
// on the other forges) is skipped with a notice. A failed lookup also skips
// the extra rather than failing the whole run. Extras can still be listed
// for the showcase without breaking the sync.
func withConfiguredExtras(configured, discovered []string, lister forge.PublicRepoLister) []string {
	known := make(map[string]bool, len(discovered))
	for _, name := range discovered {
		known[name] = true
	}

	lookuper, _ := lister.(forge.RepoLookuper)
	selected := append([]string(nil), discovered...)
	added := 0
	for _, name := range configured {
		if known[name] {
			continue
		}
		known[name] = true // ignore duplicate config entries

		if reason := extraSkipReason(name, lookuper); reason != "" {
			fmt.Printf("Skipping configured repository %s: %s\n", name, reason)
			continue
		}
		selected = append(selected, name)
		added++
	}

	if added > 0 {
		fmt.Printf("Added %d configured repositories not found by GitHub discovery\n", added)
	}
	return selected
}

// extraSkipReason returns why a configured extra repository must not be
// synced, or "" when it can be. Without a lookuper the extra cannot be
// verified and is skipped, since syncing an unverified name could fail the
// run or create an unintended repository.
func extraSkipReason(name string, lookuper forge.RepoLookuper) string {
	if lookuper == nil {
		return "cannot verify it on GitHub"
	}
	repo, err := lookuper.LookupRepo(name)
	switch {
	case err != nil:
		return fmt.Sprintf("GitHub lookup failed: %v", err)
	case !repo.Exists:
		return "not found on GitHub"
	case !strings.EqualFold(repo.Name, name):
		return fmt.Sprintf("renamed to %s on GitHub", repo.Name)
	case repo.Archived:
		return "archived on GitHub (read-only)"
	case repo.Private:
		return "private on GitHub"
	}
	return ""
}
