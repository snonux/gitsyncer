package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Organization represents a git organization with its host and name
type Organization struct {
	Host           string `json:"host"`
	Name           string `json:"name"`
	GitHubToken    string `json:"github_token,omitempty"`
	CodebergToken  string `json:"codeberg_token,omitempty"`
	BackupLocation bool   `json:"backupLocation,omitempty"` // Mark this as a backup-only destination
}

// Config holds the application configuration
type Config struct {
	Organizations       []Organization `json:"organizations"`
	Repositories        []string       `json:"repositories,omitempty"`
	ExcludeBranches     []string       `json:"exclude_branches,omitempty"`     // Regex patterns for branches to exclude
	WorkDir             string         `json:"work_dir,omitempty"`             // Working directory for cloning repositories
	ExcludeFromShowcase []string       `json:"exclude_from_showcase,omitempty"` // Repository names to exclude from showcase
}

// Load reads and parses the configuration file
func Load(path string) (*Config, error) {
	// Expand home directory if needed
	if path[:2] == "~/" {
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

	// Expand home directory in WorkDir if needed
	if strings.HasPrefix(cfg.WorkDir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		cfg.WorkDir = filepath.Join(home, cfg.WorkDir[2:])
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
		if org.Name == "" && !strings.HasPrefix(org.Host, "file://") && !org.IsSSH() {
			return fmt.Errorf("organization %d: missing name", i)
		}
	}

	return nil
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
	for _, org := range c.Organizations {
		if org.Host == host {
			return &org
		}
	}
	return nil
}

// IsCodeberg checks if the organization is Codeberg
func (o *Organization) IsCodeberg() bool {
	return o.Host == "git@codeberg.org" || strings.Contains(o.Host, "codeberg.org")
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
	return !o.IsGitHub() && !o.IsCodeberg() && !strings.HasPrefix(o.Host, "file://") &&
		(strings.Contains(o.Host, "@") || strings.Contains(o.Host, ":"))
}

