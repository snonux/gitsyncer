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
	firstOrg, firstOrgIndex, err := s.cloneFromAvailableSource(repoPath)
	if err != nil {
		return err
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

// cloneFromAvailableSource clones repoName from the first usable source and
// returns that organization's position in syncOrgs. A peer can be temporarily
// unusable because a newly discovered repository has not reached it yet, or
// because an optional peer is offline. In either case, another readable peer
// can provide the local checkout. The subsequent fetch still reports a
// mandatory peer's failure instead of silently treating it as optional.
func (s *Syncer) cloneFromAvailableSource(repoPath string) (*config.Organization, int, error) {
	orgs := s.syncOrgs()
	if len(orgs) == 0 {
		return nil, 0, fmt.Errorf("no organizations configured")
	}

	var (
		hasSource bool
		lastErr   error
	)
	for i := range orgs {
		org := &orgs[i]
		if org.IsWriteOnly() || !s.organizationActive(org) {
			continue
		}
		hasSource = true

		if err := s.cloneRepository(org, repoPath); err == nil {
			return org, i, nil
		} else {
			lastErr = err
			if org.Optional {
				s.disableOptionalForSession(s.getRemoteName(org), err)
			} else {
				fmt.Printf("Warning: Failed to clone from %s; trying another source: %v\n", s.getRemoteName(org), err)
			}
			if removeErr := os.RemoveAll(repoPath); removeErr != nil {
				return nil, 0, fmt.Errorf("failed to remove incomplete clone at %s: %w", repoPath, removeErr)
			}
		}
	}

	if !hasSource {
		return nil, 0, fmt.Errorf("no non-backup organizations configured to clone from")
	}
	return nil, 0, fmt.Errorf("failed to clone repository from any available source: %w", lastErr)
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
