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
	Host string `json:"host"`
	Name string `json:"name"`
}

// Config holds the application configuration
type Config struct {
	Organizations []Organization `json:"organizations"`
	Repositories  []string       `json:"repositories,omitempty"`
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
		// Name can be empty for file:// URLs
		if org.Name == "" && !strings.HasPrefix(org.Host, "file://") {
			return fmt.Errorf("organization %d: missing name", i)
		}
	}

	return nil
}

// GetGitURL returns the git URL for an organization
func (o *Organization) GetGitURL() string {
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