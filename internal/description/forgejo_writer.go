package description

import (
	"fmt"

	"codeberg.org/snonux/gitsyncer/internal/config"
)

// ForgejoDescriptionClient is the narrow capability ForgejoBackupDescriptionWriter
// needs from a forge client: checking whether a token is configured and
// updating a repository's description over the Forgejo/Gitea API.
// codeberg.Client (constructed via codeberg.NewForgejoClient) satisfies this
// structurally, but this package never imports the codeberg package itself -
// see ForgejoClientFactory.
type ForgejoDescriptionClient interface {
	HasToken() bool
	UpdateRepoDescription(repoName, description string) error
}

// ForgejoClientFactory builds a ForgejoDescriptionClient for a Forgejo
// backup organization. internal/cli (the composition root) injects a factory
// built from codeberg.NewForgejoClient; this package depends only on the
// factory signature and the ForgejoDescriptionClient interface it returns,
// matching the abstraction boundary forge.PublicRepoEnsurer already
// established for internal/sync's own Forgejo backup bootstrapping
// (SetForgejoBackupClientFactory).
type ForgejoClientFactory func(org *config.Organization) ForgejoDescriptionClient

// ForgejoBackupDescriptionWriter writes the canonical description to a
// Forgejo/Gitea instance's repository-description API. It only applies to
// organizations configured as Forgejo backup targets (org.IsForgejo()).
type ForgejoBackupDescriptionWriter struct {
	NewClient ForgejoClientFactory
}

// WriteBackupDescription implements BackupDescriptionWriter.
func (w ForgejoBackupDescriptionWriter) WriteBackupDescription(org *config.Organization, repoName, description string, dryRun bool) (bool, error) {
	if org == nil || !org.IsForgejo() {
		return false, nil
	}

	if dryRun {
		fmt.Printf("  [DRY RUN] Would update Forgejo description for %s on %s -> %q\n", repoName, org.Host, description)
		return true, nil
	}

	client := w.NewClient(org)
	if !client.HasToken() {
		return true, fmt.Errorf("Forgejo token missing: set FORGEJO_TOKEN or create ~/.gitsyncer_forgejo_token")
	}
	return true, client.UpdateRepoDescription(repoName, description)
}
