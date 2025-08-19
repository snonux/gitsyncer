package cli

import (
    "fmt"
    "strings"

    "codeberg.org/snonux/gitsyncer/internal/codeberg"
    "codeberg.org/snonux/gitsyncer/internal/config"
    "codeberg.org/snonux/gitsyncer/internal/github"
)

// syncRepoDescriptions ensures both platforms have the canonical description
// Precedence: Codeberg > GitHub; if Codeberg empty and GitHub has one, use GitHub.
// knownCBDesc and knownGHDesc can be empty; the function fetches as needed.
func syncRepoDescriptions(cfg *config.Config, dryRun bool, repoName, knownCBDesc, knownGHDesc string, cache map[string]string) {
    // Load orgs
    ghOrg := cfg.FindGitHubOrg()
    cbOrg := cfg.FindCodebergOrg()

    var ghClient *github.Client
    var cbClient *codeberg.Client
    if ghOrg != nil {
        c := github.NewClient(ghOrg.GitHubToken, ghOrg.Name)
        ghClient = &c
    }
    if cbOrg != nil {
        c := codeberg.NewClient(cbOrg.Name, cbOrg.CodebergToken)
        cbClient = &c
    }

    // Get current descriptions (use known if provided)
    cbDesc := strings.TrimSpace(knownCBDesc)
    ghDesc := strings.TrimSpace(knownGHDesc)
    var cbExists, ghExists bool

    if cbDesc == "" && cbClient != nil {
        if repo, exists, err := cbClient.GetRepo(repoName); err == nil {
            cbExists = exists
            if exists {
                cbDesc = strings.TrimSpace(repo.Description)
            }
        } else {
            fmt.Printf("  Warning: Codeberg repo lookup failed: %v\n", err)
        }
    } else if cbClient != nil {
        cbExists = true
    }

    if ghClient != nil {
        if ghDesc == "" || !ghExists {
            if repo, exists, err := ghClient.GetRepo(repoName); err == nil {
                ghExists = exists
                if exists {
                    ghDesc = strings.TrimSpace(repo.Description)
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

    // Update cache
    if cache != nil {
        cache[repoName] = canonical
    }
}

