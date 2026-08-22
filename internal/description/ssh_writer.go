package description

import (
	"fmt"
	"os/exec"
	"path"
	"strings"

	"github.com/snonux/gitsyncer/internal/config"
)

// SSHBackupDescriptionWriter writes the canonical description to a bare
// repository's "description" file on a remote host reachable over SSH. It
// applies to organizations that configure both DescriptionSyncHost (the SSH
// host with shell access) and DescriptionSyncRoot (the filesystem path on
// that host where bare repos live).
type SSHBackupDescriptionWriter struct{}

// WriteBackupDescription implements BackupDescriptionWriter.
func (SSHBackupDescriptionWriter) WriteBackupDescription(org *config.Organization, repoName, description string, dryRun bool) (bool, error) {
	if org == nil || strings.TrimSpace(org.DescriptionSyncHost) == "" || strings.TrimSpace(org.DescriptionSyncRoot) == "" {
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

// shellSingleQuote single-quotes value for safe interpolation into the
// remote shell command executed over ssh.
func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
