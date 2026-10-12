package description

import (
	"fmt"
	"strings"

	"github.com/snonux/gitsyncer/internal/config"
	"github.com/snonux/gitsyncer/internal/forge"
)

// ForgeClientResolver builds the forge.RepoDescriptionClient used to read and
// update a repository's canonical description on GitHub and backups.
// internal/cli's ForgeClientResolver (the composition root's single client
// construction point, see forge_client.go) satisfies this structurally, so
// this package never imports concrete forge packages.
type ForgeClientResolver interface {
	ClientFor(org *config.Organization) (forge.RepoDescriptionClient, bool)
}

// SyncRepoDescriptions ensures GitHub has the canonical description and pushes
// it to any active backup destinations. knownGHDesc can be empty; the function
// fetches the description when needed.
// writers is tried in order for each active backup organization (see
// writeBackupDescription); internal/cli constructs and injects the concrete
// list (Forgejo/file/SSH) at its composition root.
func SyncRepoDescriptions(cfg *config.Config, dryRun bool, backupActive func(*config.Organization) bool, disableBackup func(*config.Organization, error), repoName, knownGHDesc string, cache map[string]string, resolver ForgeClientResolver, writers []BackupDescriptionWriter) {
	// Load orgs
	ghOrg := cfg.FindGitHubOrg()

	var ghClient forge.RepoDescriptionClient
	if ghOrg != nil {
		ghClient, _ = resolver.ClientFor(ghOrg)
	}

	// Get current descriptions (use known if provided)
	ghDesc := strings.TrimSpace(knownGHDesc)
	var ghExists bool

	if ghClient != nil {
		if ghDesc == "" || !ghExists {
			if desc, exists, err := ghClient.GetRepoDescription(repoName); err == nil {
				ghExists = exists
				if exists {
					ghDesc = strings.TrimSpace(desc)
				}
			} else {
				fmt.Printf("  Warning: GitHub repo lookup failed: %v\n", err)
			}
		}
	}

	canonical := strings.TrimSpace(ghDesc)

	// If nothing to sync, bail
	if canonical == "" {
		return
	}

	// Update GitHub if needed
	if ghClient != nil && ghExists {
		if ghDesc != canonical {
			if dryRun {
				fmt.Printf("  [DRY RUN] Would update GitHub description for %s -> %q\n", repoName, canonical)
			} else if ghClient.HasToken() {
				if err := ghClient.UpdateRepoDescription(repoName, canonical); err != nil {
					fmt.Printf("  Warning: Failed to update GitHub description: %v\n", err)
				} else {
					fmt.Printf("  Updated GitHub description for %s\n", repoName)
				}
			} else {
				fmt.Println("  Warning: No GitHub token; cannot update description")
			}
		}
	}

	syncBackupDescriptions(cfg, dryRun, backupActive, disableBackup, repoName, canonical, writers)

	// Update cache
	if cache != nil {
		cache[repoName] = canonical
	}
}

// syncBackupDescriptions pushes canonical to every active backup
// organization in cfg, disabling (via disableBackup) any destination whose
// write fails so later repos in the same session skip it.
func syncBackupDescriptions(cfg *config.Config, dryRun bool, backupActive func(*config.Organization) bool, disableBackup func(*config.Organization, error), repoName, canonical string, writers []BackupDescriptionWriter) {
	if cfg == nil || canonical == "" {
		return
	}

	for i := range cfg.Organizations {
		org := &cfg.Organizations[i]
		if backupActive == nil || !backupActive(org) {
			continue
		}

		supported, err := writeBackupDescription(writers, org, repoName, canonical, dryRun)
		if err != nil {
			fmt.Printf("  Warning: Failed to update backup description on %s: %v\n", org.Host, err)
			if disableBackup != nil {
				disableBackup(org, err)
			}
			continue
		}
		if supported && !dryRun {
			fmt.Printf("  Updated backup description for %s on %s\n", repoName, org.Host)
		}
	}
}

// writeBackupDescription tries each writer in order and returns the result
// of the first one that reports the organization as supported, matching the
// original single-function if/else cascade (Forgejo, then file://, then
// SSH) that this writer list replaces. An org with BackupLocation unset, or
// an empty description, is unsupported without consulting any writer.
func writeBackupDescription(writers []BackupDescriptionWriter, org *config.Organization, repoName, description string, dryRun bool) (bool, error) {
	if org == nil || !org.BackupLocation {
		return false, nil
	}

	description = strings.TrimSpace(description)
	if description == "" {
		return false, nil
	}

	for _, writer := range writers {
		if supported, err := writer.WriteBackupDescription(org, repoName, description, dryRun); supported {
			return true, err
		}
	}

	return false, nil
}
