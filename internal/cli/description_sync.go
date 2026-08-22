package cli

// Description precedence (Codeberg > GitHub) and backup-description writing
// (Forgejo API / local file / SSH) used to live entirely in this file,
// reaching directly into codeberg.NewForgejoClient, os.WriteFile, and
// exec.Command("ssh", ...) from the CLI layer. Task w01 moved that domain
// logic to internal/description, leaving this file as the composition root:
// it builds the concrete Forgejo client factory and the ordered list of
// BackupDescriptionWriter implementations, then hands them to
// description.SyncRepoDescriptions. This mirrors the injection pattern task
// t01 established for forge.PublicRepoEnsurer (SetForgejoBackupClientFactory)
// and task u01 established for ForgeClientResolver.
import (
	"github.com/snonux/gitsyncer/internal/codeberg"
	"github.com/snonux/gitsyncer/internal/config"
	"github.com/snonux/gitsyncer/internal/description"
)

// backupDescriptionWriters is the ordered list of backup mechanisms
// description.SyncRepoDescriptions tries for each active backup
// organization: Forgejo API first, then a local "file://" bare repo, then an
// SSH-reachable bare repo. This is the single place (selected once, at
// composition-root construction rather than re-derived per call) where the
// concrete writer implementations are chosen; internal/description only
// depends on the BackupDescriptionWriter interface.
var backupDescriptionWriters = []description.BackupDescriptionWriter{
	description.ForgejoBackupDescriptionWriter{NewClient: newForgejoDescriptionClient},
	description.FileBackupDescriptionWriter{},
	description.SSHBackupDescriptionWriter{},
}

// newForgejoDescriptionClient constructs the Forgejo client used to write
// backup descriptions, satisfying description.ForgejoDescriptionClient. It
// is the description-package analogue of newForgejoBackupClient in
// forge_client.go: both build a codeberg.Client for a Forgejo org so that
// the packages depending on them (internal/description, internal/sync)
// never import the concrete codeberg package themselves.
func newForgejoDescriptionClient(org *config.Organization) description.ForgejoDescriptionClient {
	return codeberg.NewForgejoClient(org.ForgejoAPIBase, org.ForgejoOwner, org.ForgejoOwnerType)
}

// syncRepoDescriptions ensures both platforms have the canonical description
// Precedence: Codeberg > GitHub; if Codeberg empty and GitHub has one, use GitHub.
// knownCBDesc and knownGHDesc can be empty; the function fetches as needed.
func syncRepoDescriptions(cfg *config.Config, dryRun bool, backupActive func(*config.Organization) bool, disableBackup func(*config.Organization, error), repoName, knownCBDesc, knownGHDesc string, cache map[string]string) {
	syncRepoDescriptionsWithResolver(cfg, dryRun, backupActive, disableBackup, repoName, knownCBDesc, knownGHDesc, cache, ForgeClientResolver{})
}

func syncRepoDescriptionsWithResolver(cfg *config.Config, dryRun bool, backupActive func(*config.Organization) bool, disableBackup func(*config.Organization, error), repoName, knownCBDesc, knownGHDesc string, cache map[string]string, resolver forgeClientResolver) {
	description.SyncRepoDescriptions(cfg, dryRun, backupActive, disableBackup, repoName, knownCBDesc, knownGHDesc, cache, resolver, backupDescriptionWriters)
}
