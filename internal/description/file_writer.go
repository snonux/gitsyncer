package description

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codeberg.org/snonux/gitsyncer/internal/config"
)

// FileBackupDescriptionWriter writes the canonical description to a local
// bare repository's "description" file. It applies to organizations whose
// Host is a "file://" URL - i.e. a local-filesystem backup location.
type FileBackupDescriptionWriter struct{}

// WriteBackupDescription implements BackupDescriptionWriter.
func (FileBackupDescriptionWriter) WriteBackupDescription(org *config.Organization, repoName, description string, dryRun bool) (bool, error) {
	if org == nil || !strings.HasPrefix(org.Host, "file://") {
		return false, nil
	}

	descriptionPath := filepath.Join(strings.TrimPrefix(org.Host, "file://"), repoName+".git", "description")
	if dryRun {
		fmt.Printf("  [DRY RUN] Would update backup description for %s on %s -> %q\n", repoName, org.Host, description)
		return true, nil
	}

	return true, os.WriteFile(descriptionPath, []byte(description+"\n"), 0644)
}
