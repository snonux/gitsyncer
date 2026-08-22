package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/snonux/gitsyncer/internal/forge"
	"github.com/snonux/gitsyncer/internal/httpclient"
)

// Client handles GitHub API operations
type Client struct {
	token string
	org   string
}

var _ forge.RepoClient = (*Client)(nil)
var _ forge.RepoDescriptionClient = (*Client)(nil)

// NewClient creates a new GitHub API client
func NewClient(token, org string) *Client {
	return &Client{
		token: loadToken(token),
		org:   org,
	}
}

// loadToken resolves the GitHub token via the shared config -> GITHUB_TOKEN
// env var -> ~/.gitsyncer_github_token cascade in forge.ResolveToken, which
// is the single source of truth for this precedence (see its doc comment for
// why the cascade lives there rather than being reimplemented per forge).
func loadToken(token string) string {
	return forge.ResolveToken(token, "GITHUB_TOKEN", ".gitsyncer_github_token")
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

	result, err := httpclient.DoJSON(http.MethodGet, url, map[string]string{
		"Authorization": "Bearer " + c.token,
		"Accept":        githubAccept,
	}, nil)
	if err != nil {
		return false, err
	}

	switch result.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	case http.StatusUnauthorized:
		fmt.Printf("  401 Unauthorized - Response: %s\n", string(result.Body))
		return false, fmt.Errorf("authentication failed (401): %s", string(result.Body))
	}

	return false, fmt.Errorf("unexpected status code: %d", result.StatusCode)
}

// CreateRepo creates a new repository
func (c *Client) CreateRepo(repoName, description string, private bool) error {
	if c.token == "" {
		return fmt.Errorf("GitHub token required to create repository")
	}

	fmt.Printf("  Checking if GitHub repo %s/%s exists...\n", c.org, repoName)
	// First check if it already exists
	exists, err := forge.CheckRepoExists(repoName, c.RepoExists)
	if err != nil {
		return err
	}
	if exists {
		fmt.Printf("  GitHub repo already exists, skipping creation\n")
		// Repo already exists, nothing to do
		return nil
	}

	return c.createRepoRequest(repoName, description, private)
}

// createRepoRequest sends the create-repository POST and interprets the
// response. Split out of CreateRepo (which handles the existence check and
// early return) so each function stays close to the ~30-line guideline.
func (c *Client) createRepoRequest(repoName, description string, private bool) error {
	url := "https://api.github.com/user/repos"

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

	result, err := httpclient.DoJSON(http.MethodPost, url, map[string]string{
		"Authorization": "Bearer " + c.token,
		"Accept":        githubAccept,
		"Content-Type":  "application/json",
	}, bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}

	if result.StatusCode == http.StatusCreated {
		var createResp CreateRepoResponse
		if err := json.Unmarshal(result.Body, &createResp); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}
		fmt.Printf("Created GitHub repository: %s\n", createResp.FullName)
		return nil
	}

	return createRepoError(result.StatusCode, result.Body)
}

// createRepoError builds the error for a non-201 create-repository response,
// preferring the GitHub API's own error message when the body decodes.
func createRepoError(statusCode int, body []byte) error {
	var errResp ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		return fmt.Errorf("unexpected status code: %d", statusCode)
	}

	if errResp.Message != "" {
		return fmt.Errorf("GitHub API error: %s", errResp.Message)
	}

	return fmt.Errorf("failed to create repository: status %d", statusCode)
}

// HasToken returns whether a token is configured
func (c *Client) HasToken() bool {
	return c.token != ""
}

// GetRepo fetches a single repository by name
// Returns the repository, a boolean indicating existence, and an error
func (c *Client) GetRepo(repoName string) (Repository, bool, error) {
	var repo Repository
	if c.token == "" {
		return repo, false, fmt.Errorf("GitHub token required")
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s", c.org, repoName)
	result, err := httpclient.DoJSON(http.MethodGet, url, map[string]string{
		"Authorization": "Bearer " + c.token,
		"Accept":        githubAccept,
	}, nil)
	if err != nil {
		return repo, false, err
	}

	if result.StatusCode == http.StatusNotFound {
		return repo, false, nil
	}
	if result.StatusCode != http.StatusOK {
		return repo, false, fmt.Errorf("failed to get repo: status %d: %s", result.StatusCode, string(result.Body))
	}

	if err := json.Unmarshal(result.Body, &repo); err != nil {
		return repo, false, fmt.Errorf("failed to decode repo: %w", err)
	}
	return repo, true, nil
}

// GetRepoDescription fetches a repository description.
func (c *Client) GetRepoDescription(repoName string) (string, bool, error) {
	repo, exists, err := c.GetRepo(repoName)
	if err != nil || !exists {
		return "", exists, err
	}
	return strings.TrimSpace(repo.Description), true, nil
}

// UpdateRepoDescription updates the repository description
func (c *Client) UpdateRepoDescription(repoName, description string) error {
	if c.token == "" {
		return fmt.Errorf("GitHub token required to update repository")
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s", c.org, repoName)
	payload := map[string]interface{}{
		"description": description,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	result, err := httpclient.DoJSON(http.MethodPatch, url, map[string]string{
		"Authorization": "Bearer " + c.token,
		"Accept":        githubAccept,
		"Content-Type":  "application/json",
	}, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	if result.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to update GitHub description: %s - %s", result.Status, string(result.Body))
	}
	return nil
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

		repos, err := c.listPublicReposPage(url)
		if err != nil {
			return nil, err
		}

		// Filter for public, non-fork, non-archived
		for _, repo := range repos {
			if !repo.Private && !repo.Fork && !repo.Archived && !repo.Disabled {
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

func (c *Client) listPublicReposPage(url string) ([]Repository, error) {
	result, err := httpclient.DoJSON(http.MethodGet, url, map[string]string{
		"Authorization": "Bearer " + c.token,
		"Accept":        githubAccept,
	}, nil)
	if err != nil {
		return nil, err
	}

	if result.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list repos: status %d: %s", result.StatusCode, string(result.Body))
	}

	var repos []Repository
	if err := json.Unmarshal(result.Body, &repos); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return repos, nil
}

// GetRepoNames extracts repository names from a list of repos
func GetRepoNames(repos []Repository) []string {
	names := make([]string, len(repos))
	for i, repo := range repos {
		names[i] = repo.Name
	}
	return names
}

// DeleteRepo deletes a repository from GitHub
func (c *Client) DeleteRepo(repoName string) error {
	if c.token == "" {
		return fmt.Errorf("GitHub token required to delete repository")
	}

	if err := forge.EnsureRepoExists(c.org, repoName, c.RepoExists); err != nil {
		return err
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s", c.org, repoName)

	result, err := httpclient.DoJSON(http.MethodDelete, url, map[string]string{
		"Authorization": "Bearer " + c.token,
		"Accept":        githubAccept,
	}, nil)
	if err != nil {
		return err
	}

	return forge.DeleteStatusError(result.StatusCode, string(result.Body))
}
