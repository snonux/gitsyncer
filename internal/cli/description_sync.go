package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"codeberg.org/snonux/gitsyncer/internal/config"
	"codeberg.org/snonux/gitsyncer/internal/forge"
)

// syncRepoDescriptions ensures both platforms have the canonical description
// Precedence: Codeberg > GitHub; if Codeberg empty and GitHub has one, use GitHub.
// knownCBDesc and knownGHDesc can be empty; the function fetches as needed.
func syncRepoDescriptions(cfg *config.Config, dryRun bool, repoName, knownCBDesc, knownGHDesc string, cache map[string]string) {
	syncRepoDescriptionsWithFactory(cfg, dryRun, repoName, knownCBDesc, knownGHDesc, cache, cliRepoClientFactory)
}

func syncRepoDescriptionsWithFactory(cfg *config.Config, dryRun bool, repoName, knownCBDesc, knownGHDesc string, cache map[string]string, factory repoClientFactory) {
	// Load orgs
	ghOrg := cfg.FindGitHubOrg()
	cbOrg := cfg.FindCodebergOrg()

	var ghClient forge.RepoDescriptionClient
	var cbClient forge.RepoDescriptionClient
	if ghOrg != nil {
		ghClient = factory.NewGitHubDescriptionClient(ghOrg.GitHubToken, ghOrg.Name)
	}
	if cbOrg != nil {
		cbClient = factory.NewCodebergDescriptionClient(cbOrg.CodebergToken, cbOrg.Name)
	}

	// Get current descriptions (use known if provided)
	cbDesc := strings.TrimSpace(knownCBDesc)
	ghDesc := strings.TrimSpace(knownGHDesc)
	var cbExists, ghExists bool

	if cbDesc == "" && cbClient != nil {
		if description, exists, err := cbClient.GetRepoDescription(repoName); err == nil {
			cbExists = exists
			if exists {
				cbDesc = strings.TrimSpace(description)
			}
		} else {
			fmt.Printf("  Warning: Codeberg repo lookup failed: %v\n", err)
		}
	} else if cbClient != nil {
		cbExists = true
	}

	if ghClient != nil {
		if ghDesc == "" || !ghExists {
			if description, exists, err := ghClient.GetRepoDescription(repoName); err == nil {
				ghExists = exists
				if exists {
					ghDesc = strings.TrimSpace(description)
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

	syncBackupDescriptions(cfg, dryRun, repoName, canonical)

	// Update cache
	if cache != nil {
		cache[repoName] = canonical
	}
}

func syncBackupDescriptions(cfg *config.Config, dryRun bool, repoName, canonical string) {
	if cfg == nil || canonical == "" {
		return
	}

	for i := range cfg.Organizations {
		org := &cfg.Organizations[i]
		if !org.BackupLocation {
			continue
		}

		supported, err := syncBackupDescription(org, repoName, canonical, dryRun)
		if err != nil {
			fmt.Printf("  Warning: Failed to update backup description on %s: %v\n", org.Host, err)
			continue
		}
		if supported && !dryRun {
			fmt.Printf("  Updated backup description for %s on %s\n", repoName, org.Host)
		}
	}
}

func syncBackupDescription(org *config.Organization, repoName, description string, dryRun bool) (bool, error) {
	if org == nil || !org.BackupLocation {
		return false, nil
	}

	description = strings.TrimSpace(description)
	if description == "" {
		return false, nil
	}

	if strings.HasPrefix(org.Host, "file://") {
		descriptionPath := filepath.Join(strings.TrimPrefix(org.Host, "file://"), repoName+".git", "description")
		if dryRun {
			fmt.Printf("  [DRY RUN] Would update backup description for %s on %s -> %q\n", repoName, org.Host, description)
			return true, nil
		}
		return true, os.WriteFile(descriptionPath, []byte(description+"\n"), 0644)
	}

	if strings.TrimSpace(org.DescriptionSyncHost) == "" || strings.TrimSpace(org.DescriptionSyncRoot) == "" {
		return false, nil
	}

	descriptionPath := path.Join(org.DescriptionSyncRoot, repoName+".git", "description")
	if dryRun {
		fmt.Printf("  [DRY RUN] Would update backup description for %s on %s -> %q\n", repoName, org.Host, description)
		return true, nil
	}

	cmd := exec.Command("ssh", org.DescriptionSyncHost, fmt.Sprintf("cat > %s", shellSingleQuote(descriptionPath)))
	cmd.Stdin = strings.NewReader(description + "\n")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return true, fmt.Errorf("ssh write failed: %w\n%s", err, strings.TrimSpace(string(output)))
	}

	return true, nil
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
