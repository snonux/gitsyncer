// Package description holds the domain logic for keeping a repository's
// description in sync across forges and for pushing the canonical
// description to any configured backup destinations. This logic used to
// live in internal/cli/description_sync.go, which reached directly into
// codeberg.NewForgejoClient, os.WriteFile, and exec.Command("ssh", ...) from
// the CLI layer - a Separation of Concerns violation (task w01). Moving it
// here keeps internal/cli as a thin flag-parsing -> function-call layer:
// the composition root (internal/cli/description_sync.go) builds the
// concrete forge/file/ssh writers and injects them, following the same
// pattern task t01 established for forge.PublicRepoEnsurer
// (SetForgejoBackupClientFactory) and task u01 established for
// ForgeClientResolver.
package description

import "github.com/snonux/gitsyncer/internal/config"

// BackupDescriptionWriter writes a repository description to one backup
// mechanism (Forgejo API, local bare-repo file, or SSH-reachable bare-repo
// file). description_sync.go previously supported exactly these three
// mechanisms via an if/else cascade over org fields (org.IsForgejo(), a
// "file://" host prefix, or org.DescriptionSyncHost/Root); each mechanism is
// now its own implementation of this interface instead of a branch in one
// function.
//
// supported reports whether org is configured for this writer's mechanism at
// all, independent of whether the write itself succeeded - callers use it to
// fall through to the next writer in the list (see SyncRepoDescriptions),
// matching the original cascade's behavior where an org matching no known
// mechanism was silently skipped.
type BackupDescriptionWriter interface {
	WriteBackupDescription(org *config.Organization, repoName, description string, dryRun bool) (supported bool, err error)
}
