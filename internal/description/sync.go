package description

import (
	"fmt"
	"strings"

	"codeberg.org/snonux/gitsyncer/internal/config"
	"codeberg.org/snonux/gitsyncer/internal/forge"
)

// ForgeClientResolver builds the forge.RepoDescriptionClient used to read and
// update a repository's canonical description on GitHub/Codeberg/Forgejo.
// internal/cli's ForgeClientResolver (the composition root's single client
// construction point, see forge_client.go) satisfies this structurally, so
// this package never imports the concrete github/codeberg packages.
type ForgeClientResolver interface {
	ClientFor(org *config.Organization) (forge.RepoDescriptionClient, bool)
}

// SyncRepoDescriptions ensures both platforms have the canonical description
// and pushes it to any active backup destinations.
// Precedence: Codeberg > GitHub; if Codeberg empty and GitHub has one, use GitHub.
// knownCBDesc and knownGHDesc can be empty; the function fetches as needed.
// writers is tried in order for each active backup organization (see
// writeBackupDescription); internal/cli constructs and injects the concrete
// list (Forgejo/file/SSH) at its composition root.
func SyncRepoDescriptions(cfg *config.Config, dryRun bool, backupActive func(*config.Organization) bool, disableBackup func(*config.Organization, error), repoName, knownCBDesc, knownGHDesc string, cache map[string]string, resolver ForgeClientResolver, writers []BackupDescriptionWriter) {
	// Load orgs
	ghOrg := cfg.FindGitHubOrg()
	cbOrg := cfg.FindCodebergOrg()

	var ghClient forge.RepoDescriptionClient
	var cbClient forge.RepoDescriptionClient
	if ghOrg != nil {
		ghClient, _ = resolver.ClientFor(ghOrg)
	}
	if cbOrg != nil && cfg.CodebergSyncEnabled() && cfg.IsSyncRepo(repoName) {
		cbClient, _ = resolver.ClientFor(cbOrg)
	}

	// Get current descriptions (use known if provided)
	cbDesc := strings.TrimSpace(knownCBDesc)
	ghDesc := strings.TrimSpace(knownGHDesc)
	var cbExists, ghExists bool

	if cbDesc == "" && cbClient != nil {
		if desc, exists, err := cbClient.GetRepoDescription(repoName); err == nil {
			cbExists = exists
			if exists {
				cbDesc = strings.TrimSpace(desc)
			}
		} else {
			fmt.Printf("  Warning: Codeberg repo lookup failed: %v\n", err)
		}
	} else if cbClient != nil {
		cbExists = true
	}

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

	// Determine canonical description
	canonical := cbDesc
	if canonical == "" {
		canonical = ghDesc
	}
	canonical = strings.TrimSpace(canonical)

	// If nothing to sync, bail
	if canonical == "" {
		return
	}

	// Update Codeberg if needed
	if cbClient != nil && cbExists {
		if cbDesc != canonical {
			if dryRun {
				fmt.Printf("  [DRY RUN] Would update Codeberg description for %s -> %q\n", repoName, canonical)
			} else if cbClient.HasToken() {
				if err := cbClient.UpdateRepoDescription(repoName, canonical); err != nil {
					fmt.Printf("  Warning: Failed to update Codeberg description: %v\n", err)
				} else {
					fmt.Printf("  Updated Codeberg description for %s\n", repoName)
				}
			} else {
				fmt.Println("  Warning: No Codeberg token; cannot update description")
			}
		}
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
