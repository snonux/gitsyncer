# GitSyncer API Reference

This document provides a complete reference for all packages, types, and functions in GitSyncer.

## Table of Contents

- [Package main](#package-main)
- [Package cli](#package-cli)
- [Package codeberg](#package-codeberg)
- [Package config](#package-config)
- [Package github](#package-github)
- [Package sync](#package-sync)
- [Package version](#package-version)

---

## Package main

**Location**: `cmd/gitsyncer/main.go`

The main package provides the application entry point.

### Functions

#### func main()
Application entry point that:
- Parses command-line flags
- Routes to appropriate handlers
- Manages exit codes

---

## Package cli

**Location**: `internal/cli/`

The cli package handles all command-line interface operations.

### Types

#### type Flags
```go
type Flags struct {
    VersionFlag         bool   // Show version information
    ConfigPath          string // Path to configuration file
    ListOrgs            bool   // List configured organizations
    ListRepos           bool   // List configured repositories
    SyncRepo            string // Single repository to sync
    SyncAll             bool   // Sync all configured repositories
    SyncCodebergPublic  bool   // Sync all public Codeberg repos
    SyncGitHubPublic    bool   // Sync all public GitHub repos
    FullSync            bool   // Full bidirectional sync
    CreateGitHubRepos   bool   // Auto-create GitHub repositories
    CreateCodebergRepos bool   // Auto-create Codeberg repositories
    DryRun              bool   // Preview mode without changes
    WorkDir             string // Working directory for operations
    TestGitHubToken     bool   // Test GitHub authentication
}
```

### Functions

#### func ParseFlags() *Flags
Parses command-line arguments and returns a Flags struct with all options.

#### func HandleVersion() int
Displays version information and returns exit code 0.

#### func HandleTestGitHubToken() int
Tests GitHub token authentication:
- Loads token from config/env/file
- Validates token with API call
- Returns 0 on success, 1 on failure

#### func LoadConfig(configPath string) (*config.Config, error)
Loads configuration from specified path or default locations:
- `./gitsyncer.json`
- `~/.config/gitsyncer/config.json`
- `~/.gitsyncer.json`

#### func ShowConfigHelp()
Displays help for creating configuration files with example.

#### func HandleListOrgs(cfg *config.Config) int
Lists all configured organizations from config.

#### func HandleListRepos(cfg *config.Config) int
Lists all configured repositories from config.

#### func ShowUsage(cfg *config.Config)
Displays comprehensive usage information.

#### func HandleSync(cfg *config.Config, flags *Flags) int
Synchronizes a single repository specified by `--sync` flag.

#### func HandleSyncAll(cfg *config.Config, flags *Flags) int
Synchronizes all repositories listed in configuration.

#### func HandleSyncCodebergPublic(cfg *config.Config, flags *Flags) int
Discovers and syncs all public Codeberg repositories to other platforms.

#### func HandleSyncGitHubPublic(cfg *config.Config, flags *Flags) int
Discovers and syncs all public GitHub repositories to other platforms.

#### func ShowFullSyncMessage()
Displays information about full sync mode.

### Helper Functions (sync_handlers.go)

#### func createGitHubRepoIfNeeded(cfg *config.Config, repoName string) error
Creates GitHub repository if it doesn't exist and token is available.

#### func initGitHubClient(cfg *config.Config) *github.Client
Initializes GitHub client with token from configuration.

#### func createRepoWithClient(client *github.Client, repoName, description string) error
Creates repository using provided GitHub client.

#### func showReposToSync(repoNames []string)
Displays list of repositories that will be synced.

#### func syncCodebergRepos(cfg *config.Config, flags *Flags, repos []codeberg.Repository, repoNames []string) int
Synchronizes discovered Codeberg repositories.

#### func syncGitHubRepos(cfg *config.Config, flags *Flags, repos []github.Repository, repoNames []string) int
Synchronizes discovered GitHub repositories.

---

## Package codeberg

**Location**: `internal/codeberg/codeberg.go`

The codeberg package provides a client for interacting with Codeberg's Gitea API.

### Types

#### type Repository
```go
type Repository struct {
    ID          int64     `json:"id"`          // Repository ID
    Name        string    `json:"name"`        // Repository name
    FullName    string    `json:"full_name"`   // Full name (org/repo)
    Description string    `json:"description"` // Repository description
    Private     bool      `json:"private"`     // Is private repository
    Fork        bool      `json:"fork"`        // Is fork
    CreatedAt   time.Time `json:"created_at"`  // Creation timestamp
    UpdatedAt   time.Time `json:"updated_at"`  // Last update timestamp
    CloneURL    string    `json:"clone_url"`   // HTTPS clone URL
    SSHURL      string    `json:"ssh_url"`     // SSH clone URL
    Size        int       `json:"size"`        // Repository size
    Archived    bool      `json:"archived"`    // Is archived
    Empty       bool      `json:"empty"`       // Is empty repository
}
```

#### type Client
```go
type Client struct {
    baseURL string // API base URL (https://codeberg.org/api/v1)
    org     string // Organization or username
}
```

### Functions

#### func NewClient(org string) Client
Creates a new Codeberg API client for the specified organization/user.

### Methods

#### func (c *Client) ListPublicRepos() ([]Repository, error)
Lists all public repositories for an organization:
- Handles pagination automatically
- Filters out private, fork, archived, and empty repos
- Returns error on API failure

#### func (c *Client) ListUserPublicRepos() ([]Repository, error)
Lists all public repositories for a user:
- Same filtering as ListPublicRepos
- Use when org endpoint fails (for user accounts)

#### func GetRepoNames(repos []Repository) []string
Extracts repository names from a slice of Repository structs.

---

## Package config

**Location**: `internal/config/config.go`

The config package handles configuration loading and validation.

### Types

#### type Organization
```go
type Organization struct {
    Host        string `json:"host"`         // Git host (e.g., "git@github.com")
    Name        string `json:"name"`         // Organization/username
    GitHubToken string `json:"github_token"` // Optional GitHub API token
}
```

#### type Config
```go
type Config struct {
    Organizations   []Organization `json:"organizations"`    // List of git organizations
    Repositories    []string       `json:"repositories"`     // Specific repos to sync
    ExcludeBranches []string       `json:"exclude_branches"` // Regex patterns for branch exclusion
}
```

### Functions

#### func Load(path string) (*Config, error)
Loads configuration from JSON file:
- Validates JSON structure
- Calls Validate() on loaded config
- Returns error on failure

### Methods

#### func (c *Config) Validate() error
Validates configuration:
- Ensures at least one organization exists
- Returns error if validation fails

#### func (o *Organization) GetGitURL() string
Returns the Git URL for the organization in format `host:name`.

#### func (c *Config) FindOrganization(host string) *Organization
Finds organization by host string.

#### func (o *Organization) IsCodeberg() bool
Returns true if organization host contains "codeberg.org".

#### func (c *Config) FindCodebergOrg() *Organization
Finds first Codeberg organization in config.

#### func (o *Organization) IsGitHub() bool
Returns true if organization host contains "github.com".

#### func (c *Config) FindGitHubOrg() *Organization
Finds first GitHub organization in config.

---

## Package github

**Location**: `internal/github/github.go`

The github package provides a client for GitHub API operations.

### Types

#### type Client
```go
type Client struct {
    token string // GitHub personal access token
    org   string // Organization or username
}
```

#### type Repository
```go
type Repository struct {
    Name        string `json:"name"`        // Repository name
    Description string `json:"description"` // Repository description
    Private     bool   `json:"private"`     // Is private repository
    Fork        bool   `json:"fork"`        // Is fork
    Archived    bool   `json:"archived"`    // Is archived
    Disabled    bool   `json:"disabled"`    // Is disabled
    Size        int    `json:"size"`        // Repository size in KB
}
```

#### type CreateRepoRequest
```go
type CreateRepoRequest struct {
    Name        string `json:"name"`        // Repository name
    Description string `json:"description"` // Repository description
    Private     bool   `json:"private"`     // Create as private
    AutoInit    bool   `json:"auto_init"`   // Initialize with README
}
```

#### type CreateRepoResponse
```go
type CreateRepoResponse struct {
    ID       int64  `json:"id"`        // Repository ID
    Name     string `json:"name"`      // Repository name
    FullName string `json:"full_name"` // Full name (owner/repo)
    Private  bool   `json:"private"`   // Is private
    SSHURL   string `json:"ssh_url"`   // SSH clone URL
    CloneURL string `json:"clone_url"` // HTTPS clone URL
}
```

#### type ErrorResponse
```go
type ErrorResponse struct {
    Message string `json:"message"` // Error message
    Errors  []struct {
        Resource string `json:"resource"` // Resource type
        Field    string `json:"field"`    // Field with error
        Code     string `json:"code"`     // Error code
    } `json:"errors,omitempty"`
}
```

### Functions

#### func NewClient(token, org string) Client
Creates new GitHub API client:
- If token is empty, tries GITHUB_TOKEN env var
- If still empty, tries ~/.gitsyncer_github_token file
- Returns client with loaded token

### Methods

#### func (c *Client) HasToken() bool
Returns true if client has a token configured.

#### func (c *Client) RepoExists(repoName string) (bool, error)
Checks if repository exists:
- Returns (true, nil) if exists
- Returns (false, nil) if not found (404)
- Returns (false, error) for other errors

#### func (c *Client) CreateRepo(repoName, description string, private bool) error
Creates a new repository:
- Checks if repo already exists first
- Creates with provided settings
- Returns nil if already exists or created successfully

#### func (c *Client) ListPublicRepos() ([]Repository, error)
Lists all public repositories:
- Handles pagination automatically
- Filters out private, fork, archived, disabled, and empty repos
- Requires authentication token

#### func GetRepoNames(repos []Repository) []string
Extracts repository names from Repository slice.

---

## Package sync

**Location**: `internal/sync/`

The sync package contains the core synchronization logic.

### Types

#### type Syncer
```go
type Syncer struct {
    config           *config.Config                    // Configuration
    workDir          string                            // Working directory
    repoName         string                            // Current repository name
    abandonedReports map[string]*AbandonedBranchReport // Abandoned branch reports
    branchFilter     *BranchFilter                     // Branch exclusion filter
}
```

#### type BranchInfo
```go
type BranchInfo struct {
    Name          string    // Branch name
    LastCommit    time.Time // Last commit timestamp
    Remote        string    // Remote name
    IsAbandoned   bool      // Whether branch is abandoned
    AbandonReason string    // Reason for abandonment
}
```

#### type AbandonedBranchReport
```go
type AbandonedBranchReport struct {
    MainBranchUpdated    bool         // Is main branch active
    MainBranchLastCommit time.Time    // Main branch last commit
    AbandonedBranches    []BranchInfo // List of abandoned branches
    TotalBranches        int          // Total number of branches
}
```

#### type BranchFilter
```go
type BranchFilter struct {
    excludePatterns []*regexp.Regexp // Compiled regex patterns
}
```

### Functions

#### func New(cfg *config.Config, workDir string) *Syncer
Creates new Syncer instance with configuration and working directory.

### Syncer Methods

#### func (s *Syncer) SyncRepository(repoName string) error
Main synchronization method:
1. Creates work directory
2. Sets up repository (clone or configure remotes)
3. Fetches from all remotes
4. Gets and filters branches
5. Syncs each branch
6. Analyzes abandoned branches
7. Returns error on failure

#### func (s *Syncer) GenerateAbandonedBranchSummary() string
Generates summary report of abandoned branches across all synced repositories.

### Branch Filter Functions

#### func NewBranchFilter(excludePatterns []string) (*BranchFilter, error)
Creates new branch filter with compiled regex patterns.

### BranchFilter Methods

#### func (f *BranchFilter) ShouldExclude(branchName string) bool
Returns true if branch matches any exclusion pattern.

#### func (f *BranchFilter) FilterBranches(branches []string) []string
Returns branches that don't match exclusion patterns.

#### func (f *BranchFilter) GetExcludedBranches(branches []string) []string
Returns branches that match exclusion patterns.

#### func FormatExclusionReport(excludedBranches []string, patterns []string) string
Formats a report of excluded branches with patterns used.

### Git Operation Functions (git_operations.go)

#### func checkForMergeConflicts() (bool, string, error)
Checks if repository has merge conflicts.

#### func stashChanges() error
Stashes uncommitted changes.

#### func popStash()
Pops the last stash (called via defer).

#### func getRemotesList() (map[string]bool, error)
Returns map of configured remotes.

#### func getAllUniqueBranches(gitOutput []byte) []string
Parses git branch output and returns unique branch names.

#### func changeToRepoDirectory(repoPath string) (func(), error)
Changes to repository directory and returns restore function.

#### func fetchRemote(remote string) error
Fetches from a specific remote with prune.

#### func checkoutExistingBranch(branch string) error
Checks out an existing local branch.

#### func createTrackingBranch(branch, remoteName string) error
Creates new branch tracking a remote branch.

#### func mergeFromRemotes(branch string, remotesWithBranch map[string]bool) error
Merges changes from all remotes that have the branch.

#### func pushToAllRemotes(branch string, remotes map[string]*config.Organization, remotesWithBranch map[string]bool) error
Pushes branch to all configured remotes.

### Internal Helper Functions

#### func (s *Syncer) setupRepository(repoPath string) error
Sets up repository by cloning or adding remotes.

#### func (s *Syncer) analyzeAbandonedBranches() (*AbandonedBranchReport, error)
Analyzes branches for abandonment (6+ months inactive).

#### func (s *Syncer) findMainBranch(branches []string) string
Finds the main branch (main, master, or develop).

#### func (s *Syncer) trackRemotesWithBranch(branch string, remotes map[string]*config.Organization) map[string]bool
Returns map of remotes that have the specified branch.

---

## Package version

**Location**: `internal/version/version.go`

The version package provides version information.

### Variables

```go
var (
    Version   = "0.1.0"           // Application version
    GitCommit = "unknown"         // Git commit hash (set at build time)
    BuildDate = "unknown"         // Build date (set at build time)
    GoVersion = runtime.Version() // Go version used for build
)
```

### Functions

#### func GetVersion() string
Returns full version string with all metadata:
```
gitsyncer version 0.1.0
  Git commit: abc123
  Built: 2024-01-15
  Go version: go1.21.5
```

#### func GetShortVersion() string
Returns just the version number: `0.1.0`