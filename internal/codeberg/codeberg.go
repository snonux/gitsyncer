package codeberg

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Repository represents a Codeberg/Gitea repository
type Repository struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	FullName    string    `json:"full_name"`
	Description string    `json:"description"`
	Private     bool      `json:"private"`
	Fork        bool      `json:"fork"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	CloneURL    string    `json:"clone_url"`
	SSHURL      string    `json:"ssh_url"`
	Size        int       `json:"size"`
	Archived    bool      `json:"archived"`
	Empty       bool      `json:"empty"`
}

// Client handles Codeberg API operations
type Client struct {
	baseURL string
	org     string
	token   string
}

// NewClient creates a new Codeberg API client
func NewClient(org, token string) Client {
	c := Client{
		baseURL: "https://codeberg.org/api/v1",
		org:     org,
	}
	c.loadToken(token)
	return c
}

// loadToken loads the Codeberg API token from config, env, or file
func (c *Client) loadToken(tokenFromConfig string) {
	if tokenFromConfig != "" {
		c.token = tokenFromConfig
		return
	}

	// Check environment variable
	if token := os.Getenv("CODEBERG_TOKEN"); token != "" {
		c.token = token
		return
	}

	// Check token file
	home, err := os.UserHomeDir()
	if err == nil {
		tokenFile := filepath.Join(home, ".gitsyncer_codeberg_token")
		if data, err := os.ReadFile(tokenFile); err == nil {
			c.token = string(data)
		}
	}
}

// HasToken returns true if a token is loaded
func (c *Client) HasToken() bool {
	return c.token != ""
}

// ListPublicRepos lists all public repositories for an organization
func (c *Client) ListPublicRepos() ([]Repository, error) {
	var allRepos []Repository
	page := 1
	perPage := 50

	for {
		url := fmt.Sprintf("%s/orgs/%s/repos?page=%d&limit=%d", c.baseURL, c.org, page, perPage)

		resp, err := http.Get(url)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch repositories: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
		}

		var repos []Repository
		if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}

		// Filter only public, non-fork, non-archived, non-empty repos
		for _, repo := range repos {
			if !repo.Private && !repo.Fork && !repo.Archived && !repo.Empty {
				allRepos = append(allRepos, repo)
			}
		}

		// If we got fewer repos than requested, we've reached the end
		if len(repos) < perPage {
			break
		}

		page++
	}

	return allRepos, nil
}

// ListUserPublicRepos lists all public repositories for a user
func (c *Client) ListUserPublicRepos() ([]Repository, error) {
	var allRepos []Repository
	page := 1
	perPage := 50

	for {
		url := fmt.Sprintf("%s/users/%s/repos?page=%d&limit=%d", c.baseURL, c.org, page, perPage)

		resp, err := http.Get(url)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch repositories: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
		}

		var repos []Repository
		if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}

		// Filter only public, non-fork, non-archived, non-empty repos
		for _, repo := range repos {
			if !repo.Private && !repo.Fork && !repo.Archived && !repo.Empty {
				allRepos = append(allRepos, repo)
			}
		}

		// If we got fewer repos than requested, we've reached the end
		if len(repos) < perPage {
			break
		}

		page++
	}

	return allRepos, nil
}

// GetRepoNames returns just the repository names
func GetRepoNames(repos []Repository) []string {
	names := make([]string, 0, len(repos))
	for _, repo := range repos {
		names = append(names, repo.Name)
	}
	return names
}

// RepoExists checks if a repository exists on Codeberg
func (c *Client) RepoExists(repoName string) (bool, error) {
	url := fmt.Sprintf("%s/repos/%s/%s", c.baseURL, c.org, repoName)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, err
	}

	if c.HasToken() {
		req.Header.Set("Authorization", "token "+c.token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode == 200, nil
}

// CreateRepo creates a new repository on Codeberg
func (c *Client) CreateRepo(repoName, description string, private bool) error {
	exists, err := c.RepoExists(repoName)
	if err != nil {
		return fmt.Errorf("failed to check if repo exists: %w", err)
	}
	if exists {
		return nil // Repository already exists
	}

	url := fmt.Sprintf("%s/user/repos", c.baseURL)

	payload := map[string]interface{}{
		"name":        repoName,
		"description": description,
		"private":     private,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	if c.HasToken() {
		req.Header.Set("Authorization", "token "+c.token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		// Read the response body to get more detailed error information
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to create repository: status code %d (could not read response)", resp.StatusCode)
		}
		
		// Try to parse as JSON error response
		var errorResp map[string]interface{}
		if err := json.Unmarshal(body, &errorResp); err == nil {
			// If we can parse the JSON, extract the message
			if msg, ok := errorResp["message"].(string); ok {
				return fmt.Errorf("failed to create repository: %s (status code %d)", msg, resp.StatusCode)
			}
		}
		
		// If we can't parse JSON, return the raw response
		return fmt.Errorf("failed to create repository: %s (status code %d)", string(body), resp.StatusCode)
	}

	return nil
}
