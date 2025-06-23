package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Client handles GitHub API operations
type Client struct {
	token string
	org   string
}

// NewClient creates a new GitHub API client
func NewClient(token, org string) *Client {
	// If no token provided, try other sources
	if token == "" {
		// Try environment variable
		token = os.Getenv("GITHUB_TOKEN")
		
		// If still no token, try reading from file
		if token == "" {
			home, err := os.UserHomeDir()
			if err == nil {
				tokenFile := filepath.Join(home, ".gitsyncer_github_token")
				data, err := os.ReadFile(tokenFile)
				if err == nil {
					token = strings.TrimSpace(string(data))
				}
			}
		}
	}
	return &Client{
		token: token,
		org:   org,
	}
}

// CreateRepoRequest represents the request to create a repository
type CreateRepoRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Private     bool   `json:"private"`
	AutoInit    bool   `json:"auto_init"`
}

// CreateRepoResponse represents the response from creating a repository
type CreateRepoResponse struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Private  bool   `json:"private"`
	SSHURL   string `json:"ssh_url"`
	CloneURL string `json:"clone_url"`
}

// ErrorResponse represents an error response from GitHub API
type ErrorResponse struct {
	Message string `json:"message"`
	Errors  []struct {
		Resource string `json:"resource"`
		Field    string `json:"field"`
		Code     string `json:"code"`
	} `json:"errors,omitempty"`
}

// RepoExists checks if a repository exists
func (c *Client) RepoExists(repoName string) (bool, error) {
	if c.token == "" {
		return false, fmt.Errorf("GitHub token required")
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s", c.org, repoName)
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, err
	}
	
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode == 200 {
		return true, nil
	} else if resp.StatusCode == 404 {
		return false, nil
	}
	
	return false, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
}

// CreateRepo creates a new repository
func (c *Client) CreateRepo(repoName, description string, private bool) error {
	if c.token == "" {
		return fmt.Errorf("GitHub token required to create repository")
	}

	fmt.Printf("  Checking if GitHub repo %s/%s exists...\n", c.org, repoName)
	// First check if it already exists
	exists, err := c.RepoExists(repoName)
	if err != nil {
		return fmt.Errorf("failed to check if repo exists: %w", err)
	}
	if exists {
		fmt.Printf("  GitHub repo already exists, skipping creation\n")
		// Repo already exists, nothing to do
		return nil
	}
	
	url := fmt.Sprintf("https://api.github.com/user/repos")
	
	reqBody := CreateRepoRequest{
		Name:        repoName,
		Description: description,
		Private:     private,
		AutoInit:    false, // Don't auto-init, we'll push content
	}
	
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}
	
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}
	
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")
	
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode == 201 {
		var createResp CreateRepoResponse
		if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}
		fmt.Printf("Created GitHub repository: %s\n", createResp.FullName)
		return nil
	}
	
	// Handle error response
	var errResp ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	
	if errResp.Message != "" {
		return fmt.Errorf("GitHub API error: %s", errResp.Message)
	}
	
	return fmt.Errorf("failed to create repository: status %d", resp.StatusCode)
}

// HasToken returns whether a token is configured
func (c *Client) HasToken() bool {
	return c.token != ""
}