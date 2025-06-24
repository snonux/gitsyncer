package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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
func NewClient(token, org string) Client {
	// If no token provided, try other sources
	if token == "" {
		fmt.Println("  No token in config, trying environment variable...")
		// Try environment variable
		token = os.Getenv("GITHUB_TOKEN")
		
		// If still no token, try reading from file
		if token == "" {
			fmt.Println("  No GITHUB_TOKEN env var, trying ~/.gitsyncer_github_token file...")
			home, err := os.UserHomeDir()
			if err == nil {
				tokenFile := filepath.Join(home, ".gitsyncer_github_token")
				data, err := os.ReadFile(tokenFile)
				if err == nil {
					token = strings.TrimSpace(string(data))
					fmt.Printf("  Loaded token from file (length: %d)\n", len(token))
					// Check for common issues
					if strings.Contains(token, "\n") || strings.Contains(token, "\r") {
						fmt.Println("  Warning: Token contains newline characters")
					}
					if strings.HasPrefix(token, " ") || strings.HasSuffix(token, " ") {
						fmt.Println("  Warning: Token has leading/trailing spaces")
					}
				} else {
					fmt.Printf("  Could not read token file: %v\n", err)
				}
			}
		} else {
			fmt.Printf("  Loaded token from env var (length: %d)\n", len(token))
		}
	} else {
		fmt.Printf("  Using token from config (length: %d)\n", len(token))
	}
	return Client{
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
	fmt.Printf("  Checking URL: %s\n", url)
	fmt.Printf("  Token present: %v (length: %d)\n", c.token != "", len(c.token))
	
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
	} else if resp.StatusCode == 401 {
		// Read the response body for 401 errors
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("  401 Unauthorized - Response: %s\n", string(body))
		fmt.Printf("  Authorization header: %s\n", req.Header.Get("Authorization"))
		return false, fmt.Errorf("authentication failed (401): %s", string(body))
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

// Repository represents a GitHub repository
type Repository struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Private     bool   `json:"private"`
	Fork        bool   `json:"fork"`
	Archived    bool   `json:"archived"`
	Disabled    bool   `json:"disabled"`
	Size        int    `json:"size"`
}

// ListPublicRepos lists all public repositories for the user/org
func (c *Client) ListPublicRepos() ([]Repository, error) {
	if c.token == "" {
		return nil, fmt.Errorf("GitHub token required to list repositories")
	}

	var allRepos []Repository
	page := 1
	perPage := 100

	for {
		url := fmt.Sprintf("https://api.github.com/users/%s/repos?page=%d&per_page=%d&type=owner", c.org, page, perPage)
		fmt.Printf("  Fetching page %d...\n", page)
		
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Accept", "application/vnd.github.v3+json")
		
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != 200 {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("failed to list repos: status %d: %s", resp.StatusCode, string(body))
		}
		
		var repos []Repository
		if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
		
		// Filter for public, non-fork, non-archived, non-empty repos
		for _, repo := range repos {
			if !repo.Private && !repo.Fork && !repo.Archived && !repo.Disabled && repo.Size > 0 {
				allRepos = append(allRepos, repo)
			}
		}
		
		// Check if there are more pages
		if len(repos) < perPage {
			break
		}
		page++
	}
	
	return allRepos, nil
}

// GetRepoNames extracts repository names from a list of repos
func GetRepoNames(repos []Repository) []string {
	names := make([]string, len(repos))
	for i, repo := range repos {
		names[i] = repo.Name
	}
	return names
}