package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"codeberg.org/snonux/gitsyncer/internal/forge"
)

var forgejoOwnerPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

// defaultAutoDeleteProtectedBranches lists repo/branch pairs that must never
// be offered for automatic deletion by the abandoned-branch cleanup flow,
// regardless of how stale the branch's history looks. This is a hard-coded
// safety net (not currently exposed via config.json) for branches that carry
// meaning beyond git history, e.g. a branch literally named "hosts". It lives
// here rather than in internal/sync because it is a repository-wide policy
// decision, not something specific to branch analysis.
var defaultAutoDeleteProtectedBranches = map[string]map[string]struct{}{
	"xerl": {
		"hosts": {},
	},
}

// IsProtectedFromAutoDelete reports whether branchName in repoName is exempt
// from automatic abandoned-branch deletion under the built-in policy above.
func IsProtectedFromAutoDelete(repoName, branchName string) bool {
	branches, ok := defaultAutoDeleteProtectedBranches[repoName]
	if !ok {
		return false
	}

	_, ok = branches[branchName]
	return ok
}

// Organization represents a git organization with its host and name
type Organization struct {
	Host                string          `json:"host"`
	Name                string          `json:"name"`
	GitHubToken         string          `json:"github_token,omitempty"`
	CodebergToken       string          `json:"codeberg_token,omitempty"`
	ForgejoAPIBase      string          `json:"forgejo_api_base,omitempty"`    // Gitea-compatible API root, for example https://code.example/api/v1
	ForgejoOwner        string          `json:"forgejo_owner,omitempty"`       // Forgejo user or organization that owns backup repositories
	ForgejoOwnerType    forge.OwnerType `json:"forgejo_owner_type,omitempty"`  // user (default) or organization
	BackupLocation      bool            `json:"backupLocation,omitempty"`      // Mark this as a backup-only destination
	ForcePush           bool            `json:"forcePush,omitempty"`           // Force-update branches and tags at this backup destination
	DescriptionSyncHost string          `json:"descriptionSyncHost,omitempty"` // SSH host with shell access for updating backup descriptions
	DescriptionSyncRoot string          `json:"descriptionSyncRoot,omitempty"` // Filesystem path on DescriptionSyncHost where bare repos live
}

// Config holds the application configuration
type Config struct {
	Organizations         []Organization    `json:"organizations"`
	Repositories          []string          `json:"repositories,omitempty"`
	ExcludeBranches       []string          `json:"exclude_branches,omitempty"`        // Regex patterns for branches to exclude
	WorkDir               string            `json:"work_dir,omitempty"`                // Working directory for cloning repositories
	ExcludeFromShowcase   []string          `json:"exclude_from_showcase,omitempty"`   // Repository names to exclude from showcase
	ShowcaseStatsBranches map[string]string `json:"showcase_stats_branches,omitempty"` // Repository names mapped to the branch used for showcase stats/code snippets
	ShowcaseOutputDir     string            `json:"showcase_output_dir,omitempty"`     // Directory where showcase files and assets are written
	ShowcaseCgitHost      string            `json:"showcase_cgit_host,omitempty"`      // Deprecated: accepted for configuration compatibility but ignored
	// SkipReleases maps a repository name to a list of tag names for which
	// releases should NOT be created on any platform (GitHub/Codeberg)
	SkipReleases map[string][]string `json:"skip_releases,omitempty"`
	// SyncCodeberg opts in to any Codeberg syncing (push/fetch, repo creation,
	// releases, description sync, and the codeberg-to-github / github-to-codeberg
	// subcommands). Codeberg is never synced unless this is explicitly true,
	// even when a Codeberg organization is present in Organizations.
	SyncCodeberg bool `json:"sync_codeberg,omitempty"`
}

// Load reads and parses the configuration file
func Load(path string) (*Config, error) {
	// If no path provided, use default
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		// Use XDG config directory with .json extension
		path = filepath.Join(home, ".config", "gitsyncer", "config.json")
	} else if len(path) >= 2 && path[:2] == "~/" {
		// Expand home directory if needed
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		path = filepath.Join(home, path[2:])
	}

	// Read config file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse JSON
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Set default WorkDir if not specified
	if cfg.WorkDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		cfg.WorkDir = filepath.Join(home, "git", "gitsyncer-workdir")
	}

	// Expand home directory in WorkDir if needed.
	expandedWorkDir, err := expandHomePath(cfg.WorkDir)
	if err != nil {
		return nil, err
	}
	cfg.WorkDir = expandedWorkDir

	// Expand home directory in showcase output directory if configured.
	if strings.TrimSpace(cfg.ShowcaseOutputDir) != "" {
		expandedShowcaseOutputDir, err := expandHomePath(cfg.ShowcaseOutputDir)
		if err != nil {
			return nil, err
		}
		cfg.ShowcaseOutputDir = expandedShowcaseOutputDir
	}

	return &cfg, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if len(c.Organizations) == 0 {
		return fmt.Errorf("no organizations configured")
	}

	for i, org := range c.Organizations {
		if org.Host == "" {
			return fmt.Errorf("organization %d: missing host", i)
		}
		// Name can be empty for file:// URLs or SSH backup locations
		if org.Name == "" && !strings.HasPrefix(org.Host, "file://") && !org.IsSSH() && !org.IsForgejo() {
			return fmt.Errorf("organization %d: missing name", i)
		}
		if org.ForcePush && !org.BackupLocation {
			return fmt.Errorf("organization %d: forcePush requires backupLocation", i)
		}
		if org.IsForgejo() {
			if org.ForgejoOwnerType != "" && !org.ForgejoOwnerType.Valid() {
				return fmt.Errorf("organization %d: forgejo_owner_type must be %q or %q", i, forge.OwnerTypeUser, forge.OwnerTypeOrganization)
			}
			if !org.BackupLocation {
				return fmt.Errorf("organization %d: Forgejo targets must set backupLocation", i)
			}
			if strings.TrimSpace(org.ForgejoOwner) == "" {
				return fmt.Errorf("organization %d: forgejo_owner is required with forgejo_api_base", i)
			}
			if !forgejoOwnerPattern.MatchString(org.ForgejoOwner) || org.ForgejoOwner == "." || org.ForgejoOwner == ".." {
				return fmt.Errorf("organization %d: forgejo_owner must be one safe path segment", i)
			}
			sshBase, err := url.Parse(org.Host)
			if err != nil {
				return fmt.Errorf("organization %d: forgejo host: %v", i, err)
			}
			if err := validateForgejoHost(sshBase); err != nil {
				return fmt.Errorf("organization %d: %w", i, err)
			}
			apiBase, err := url.Parse(org.ForgejoAPIBase)
			if err != nil || (apiBase.Scheme != "http" && apiBase.Scheme != "https") || apiBase.Host == "" || !apiBase.IsAbs() || apiBase.User != nil || apiBase.RawQuery != "" || apiBase.Fragment != "" {
				return fmt.Errorf("organization %d: forgejo_api_base must be an absolute HTTP(S) URL", i)
			}
			if org.DescriptionSyncHost != "" || org.DescriptionSyncRoot != "" {
				return fmt.Errorf("organization %d: Forgejo descriptions are managed through the API; descriptionSyncHost/Root are not allowed", i)
			}
		}
		if org.ForgejoOwner != "" && org.ForgejoAPIBase == "" {
			return fmt.Errorf("organization %d: forgejo_api_base is required with forgejo_owner", i)
		}
		if org.ForgejoOwnerType != "" && org.ForgejoAPIBase == "" {
			return fmt.Errorf("organization %d: forgejo_api_base is required with forgejo_owner_type", i)
		}
		hasDescriptionSyncHost := strings.TrimSpace(org.DescriptionSyncHost) != ""
		hasDescriptionSyncRoot := strings.TrimSpace(org.DescriptionSyncRoot) != ""
		if hasDescriptionSyncHost != hasDescriptionSyncRoot {
			return fmt.Errorf("organization %d: descriptionSyncHost and descriptionSyncRoot must be set together", i)
		}
	}

	for repo, branch := range c.ShowcaseStatsBranches {
		if strings.TrimSpace(repo) == "" {
			return fmt.Errorf("showcase_stats_branches: repository name cannot be empty")
		}
		if strings.TrimSpace(branch) == "" {
			return fmt.Errorf("showcase_stats_branches[%q]: branch name cannot be empty", repo)
		}
	}

	return nil
}

// validateForgejoHost checks that u (the already-parsed org.Host) is an
// absolute ssh:// URL identifying exactly one user and host, with no
// password, path beyond an optional trailing slash, query string, fragment,
// or invalid port. Each condition is checked separately so a misconfigured
// host reports specifically what is wrong instead of one opaque message
// covering roughly ten different possible causes.
func validateForgejoHost(u *url.URL) error {
	if u.Scheme != "ssh" {
		return fmt.Errorf("forgejo host: scheme must be ssh, got %q", u.Scheme)
	}
	if u.Hostname() == "" {
		return fmt.Errorf("forgejo host: missing hostname")
	}
	if u.User == nil {
		return fmt.Errorf("forgejo host: missing user")
	}
	if u.User.Username() == "" {
		return fmt.Errorf("forgejo host: missing username")
	}
	if u.User.String() != u.User.Username() {
		return fmt.Errorf("forgejo host: user must not include a password")
	}
	if u.Path != "" && u.Path != "/" {
		return fmt.Errorf("forgejo host: path must be empty or %q, got %q", "/", u.Path)
	}
	if u.RawQuery != "" {
		return fmt.Errorf("forgejo host: query string is not allowed")
	}
	if u.Fragment != "" {
		return fmt.Errorf("forgejo host: fragment is not allowed")
	}
	if !validPort(u.Port()) {
		return fmt.Errorf("forgejo host: invalid port %q", u.Port())
	}
	return nil
}

func validPort(port string) bool {
	if port == "" {
		return true
	}
	n, err := strconv.Atoi(port)
	return err == nil && n > 0 && n <= 65535
}

// ShouldSkipRelease returns true if the configuration specifies that
// the given repo/tag combination should not have a release created.
func (c *Config) ShouldSkipRelease(repo, tag string) bool {
	if c == nil || c.SkipReleases == nil {
		return false
	}
	tags, ok := c.SkipReleases[repo]
	if !ok {
		return false
	}
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

// GetGitURL returns the git URL for an organization
func (o *Organization) GetGitURL() string {
	// For SSH backup locations with empty name, just return the host
	if o.IsSSH() && o.Name == "" {
		return o.Host
	}
	return fmt.Sprintf("%s:%s", o.Host, o.Name)
}

// FindOrganization finds an organization by host
func (c *Config) FindOrganization(host string) *Organization {
	for i := range c.Organizations {
		if c.Organizations[i].Host == host {
			return &c.Organizations[i]
		}
	}
	return nil
}

// IsCodeberg checks if the organization is Codeberg
func (o *Organization) IsCodeberg() bool {
	return o.Host == "git@codeberg.org" || strings.Contains(o.Host, "codeberg.org")
}

// IsForgejo reports whether the organization is a configured Forgejo backup target.
func (o *Organization) IsForgejo() bool {
	return strings.TrimSpace(o.ForgejoAPIBase) != ""
}

// CodebergSyncEnabled reports whether Codeberg syncing has been explicitly
// enabled in the configuration. Codeberg is never synced unless this returns
// true, regardless of whether a Codeberg organization is configured.
func (c *Config) CodebergSyncEnabled() bool {
	return c != nil && c.SyncCodeberg
}

// FilterSyncRepos restricts a set of discovered repository names to those that
// are explicitly configured for syncing. When Repositories is non-empty it acts
// as an allowlist and only repos in that list are returned; this prevents the
// discovery-based sync commands (codeberg-to-github, github-to-codeberg,
// batch-run, bidirectional) from syncing unintended repos to a platform such as
// Codeberg. When Repositories is empty, discovery mode is preserved and all
// discovered repos are returned untouched.
func (c *Config) FilterSyncRepos(discovered []string) []string {
	if c == nil || len(c.Repositories) == 0 {
		return discovered
	}
	allowed := make(map[string]bool, len(c.Repositories))
	for _, r := range c.Repositories {
		allowed[r] = true
	}
	filtered := make([]string, 0, len(discovered))
	for _, r := range discovered {
		if allowed[r] {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// IsSyncRepo reports whether repoName is part of the configured sync set. When
// Repositories is non-empty, only repos in that list are considered sync repos
// (the allowlist). When Repositories is empty (discovery mode), every repo is
// considered a sync repo.
func (c *Config) IsSyncRepo(repoName string) bool {
	if c == nil {
		return false
	}
	if len(c.Repositories) == 0 {
		return true
	}
	for _, r := range c.Repositories {
		if r == repoName {
			return true
		}
	}
	return false
}

// FindCodebergOrg finds the first Codeberg organization
func (c *Config) FindCodebergOrg() *Organization {
	for i := range c.Organizations {
		if c.Organizations[i].IsCodeberg() {
			return &c.Organizations[i]
		}
	}
	return nil
}

// FindForgejoOrg finds the first configured Forgejo organization.
func (c *Config) FindForgejoOrg() *Organization {
	for i := range c.Organizations {
		if c.Organizations[i].IsForgejo() {
			return &c.Organizations[i]
		}
	}
	return nil
}

// SyncOrganizations returns the organizations that should participate in
// repository syncing (clone/fetch/push). It excludes Codeberg organizations
// when Codeberg syncing is not enabled via CodebergSyncEnabled, so that a
// Codeberg org may still be configured (e.g. for showcase links or tokens)
// without becoming an active sync target. Backup locations are returned as
// usual; the sync layer gates them separately via backupActive.
func (c *Config) SyncOrganizations() []Organization {
	if c == nil {
		return nil
	}
	if c.CodebergSyncEnabled() {
		return c.Organizations
	}
	orgs := make([]Organization, 0, len(c.Organizations))
	for i := range c.Organizations {
		if c.Organizations[i].IsCodeberg() {
			continue
		}
		orgs = append(orgs, c.Organizations[i])
	}
	return orgs
}

// IsGitHub checks if the organization is GitHub
func (o *Organization) IsGitHub() bool {
	return o.Host == "git@github.com" || strings.Contains(o.Host, "github.com")
}

// FindGitHubOrg finds the first GitHub organization
func (c *Config) FindGitHubOrg() *Organization {
	for i := range c.Organizations {
		if c.Organizations[i].IsGitHub() {
			return &c.Organizations[i]
		}
	}
	return nil
}

// IsSSH checks if the organization is a plain SSH location
func (o *Organization) IsSSH() bool {
	// Check if it's not a known git hosting service and contains SSH-like syntax
	return !o.IsGitHub() && !o.IsCodeberg() && !o.IsForgejo() && !strings.HasPrefix(o.Host, "file://") &&
		(strings.Contains(o.Host, "@") || strings.Contains(o.Host, ":"))
}

// GetShowcaseOutputDir returns the configured showcase output directory when
// present, otherwise the default output location.
func (c *Config) GetShowcaseOutputDir() (string, error) {
	if c != nil && strings.TrimSpace(c.ShowcaseOutputDir) != "" {
		return expandHomePath(c.ShowcaseOutputDir)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, "git", "foo.zone-content", "gemtext", "about"), nil
}

func expandHomePath(path string) (string, error) {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		return filepath.Join(home, path[2:]), nil
	}

	return path, nil
}
