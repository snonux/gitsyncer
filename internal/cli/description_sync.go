package cli

// GitHub description sync and backup-description writing
// (Forgejo API / local file / SSH) used to live entirely in this file,
// reaching directly into forgejo.NewForgejoClient, os.WriteFile, and
// exec.Command("ssh", ...) from the CLI layer. Task w01 moved that domain
// logic to internal/description, leaving this file as the composition root:
// it builds the concrete Forgejo client factory and the ordered list of
// BackupDescriptionWriter implementations, then hands them to
// description.SyncRepoDescriptions. This mirrors the injection pattern task
// t01 established for forge.PublicRepoEnsurer (SetForgejoBackupClientFactory)
// and task u01 established for ForgeClientResolver.
import (
	"github.com/snonux/gitsyncer/internal/config"
	"github.com/snonux/gitsyncer/internal/description"
	"github.com/snonux/gitsyncer/internal/forgejo"
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
// forge_client.go: both build a forgejo.Client for a Forgejo org so that
// the packages depending on them (internal/description, internal/sync)
// never import the concrete Forgejo package themselves.
func newForgejoDescriptionClient(org *config.Organization) description.ForgejoDescriptionClient {
	return forgejo.NewForgejoClient(org.ForgejoAPIBase, org.ForgejoOwner, org.ForgejoOwnerType)
}

// syncRepoDescriptions synchronizes the GitHub description to active backups.
// knownGHDesc may be empty; the description package fetches it when needed.
func syncRepoDescriptions(cfg *config.Config, dryRun bool, backupActive func(*config.Organization) bool, disableBackup func(*config.Organization, error), repoName, knownGHDesc string, cache map[string]string) {
	syncRepoDescriptionsWithResolver(cfg, dryRun, backupActive, disableBackup, repoName, knownGHDesc, cache, ForgeClientResolver{})
}

func syncRepoDescriptionsWithResolver(cfg *config.Config, dryRun bool, backupActive func(*config.Organization) bool, disableBackup func(*config.Organization, error), repoName, knownGHDesc string, cache map[string]string, resolver forgeClientResolver) {
	description.SyncRepoDescriptions(cfg, dryRun, backupActive, disableBackup, repoName, knownGHDesc, cache, resolver, backupDescriptionWriters)
}
