package sync

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/snonux/gitsyncer/internal/config"
)

// setupRepository ensures the repository exists and all remotes are configured
func (s *Syncer) setupRepository(repoPath string) error {
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		return s.setupNewRepository(repoPath)
	}
	return s.setupExistingRepository(repoPath)
}

// setupNewRepository clones and configures a new repository
func (s *Syncer) setupNewRepository(repoPath string) error {
	orgs := s.syncOrgs()
	if len(orgs) == 0 {
		return fmt.Errorf("no organizations configured")
	}

	// Find first non-backup organization to clone from
	var firstOrg *config.Organization
	var firstOrgIndex int
	for i := range orgs {
		if !orgs[i].BackupLocation {
			firstOrg = &orgs[i]
			firstOrgIndex = i
			break
		}
	}

	if firstOrg == nil {
		return fmt.Errorf("no non-backup organizations configured to clone from")
	}

	if err := s.cloneRepository(firstOrg, repoPath); err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}

	// Rename origin to the proper remote name
	firstRemoteName := s.getRemoteName(firstOrg)
	cmd := exec.Command("git", "-C", repoPath, "remote", "rename", "origin", firstRemoteName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to rename origin remote: %w", err)
	}

	// Add other organizations as remotes
	for i := range orgs {
		if i == firstOrgIndex {
			continue // Skip the first org we already cloned from
		}
		org := &orgs[i]
		remoteName := s.getRemoteName(org)

		if !s.organizationActive(org) {
			continue
		}

		if err := s.addRemote(repoPath, org); err != nil {
			return fmt.Errorf("failed to add remote %s: %w", remoteName, err)
		}
	}

	return nil
}

// setupExistingRepository ensures all remotes are configured for an existing repository
func (s *Syncer) setupExistingRepository(repoPath string) error {
	fmt.Printf("Using existing repository at %s\n", repoPath)

	orgs := s.syncOrgs()
	// Check and add any missing remotes
	for i := range orgs {
		org := &orgs[i]
		remoteName := s.getRemoteName(org)

		if !s.organizationActive(org) {
			continue
		}

		// Check if remote exists
		cmd := exec.Command("git", "-C", repoPath, "remote", "get-url", remoteName)
		if err := cmd.Run(); err != nil {
			// Remote doesn't exist, add it
			if err := s.addRemote(repoPath, org); err != nil {
				return fmt.Errorf("failed to add remote %s: %w", remoteName, err)
			}
		} else if org.IsForgejo() {
			expected := s.expectedRemoteURL(org)
			if err := verifyRemoteURLs(repoPath, remoteName, expected); err != nil {
				fmt.Printf("Updating stale Forgejo remote %s to %s\n", remoteName, expected)
				if setErr := setRemoteURLs(repoPath, remoteName, expected); setErr != nil {
					s.disableOrganizationForSession(org, setErr)
					continue
				}
			}
		}
	}

	return nil
}

func verifyRemoteURLs(repoPath, remoteName, expected string) error {
	for _, args := range [][]string{
		{"remote", "get-url", "--all", remoteName},
		{"remote", "get-url", "--push", "--all", remoteName},
	} {
		output, err := exec.Command("git", append([]string{"-C", repoPath}, args...)...).Output()
		if err != nil {
			return fmt.Errorf("failed to inspect Forgejo remote %s: %w", remoteName, err)
		}
		for _, configured := range strings.Fields(string(output)) {
			if configured != expected {
				return fmt.Errorf("Forgejo remote %s URL mismatch: configured %q, expected %q", remoteName, configured, expected)
			}
		}
	}
	return nil
}

// setRemoteURLs rewrites both fetch and push URLs for remoteName. Used to
// migrate older Forgejo remotes that were stored without forgejo_owner in the
// path (…/repo.git) to the configured owner path (…/owner/repo.git).
func setRemoteURLs(repoPath, remoteName, url string) error {
	if output, err := exec.Command("git", "-C", repoPath, "remote", "set-url", remoteName, url).CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set fetch URL for %s: %w\n%s", remoteName, err, output)
	}
	if output, err := exec.Command("git", "-C", repoPath, "remote", "set-url", "--push", remoteName, url).CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set push URL for %s: %w\n%s", remoteName, err, output)
	}
	return nil
}

// getRemotesMap creates a map of remote names to organizations
func (s *Syncer) getRemotesMap() map[string]*config.Organization {
	remotes := make(map[string]*config.Organization)
	orgs := s.syncOrgs()
	for i := range orgs {
		org := &orgs[i]
		remoteName := s.getRemoteName(org)

		if !s.organizationActive(org) {
			continue
		}

		remotes[remoteName] = org
	}
	return remotes
}
