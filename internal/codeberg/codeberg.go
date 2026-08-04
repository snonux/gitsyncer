package codeberg

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"codeberg.org/snonux/gitsyncer/internal/forge"
	"codeberg.org/snonux/gitsyncer/internal/httpclient"
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
	service string
}

var _ forge.RepoClient = (*Client)(nil)
var _ forge.RepoDescriptionClient = (*Client)(nil)

// closeResponseBody closes an HTTP response body. By the time this runs the
// body has already been fully read (or the caller is bailing out on an
// earlier error), so a close failure here cannot change the outcome that was
// already determined - the error is intentionally discarded rather than
// treated as actionable.
func closeResponseBody(resp *http.Response) {
	_ = resp.Body.Close()
}

// NewClient creates a new Codeberg API client
func NewClient(token, org string) *Client {
	c := &Client{
		baseURL: "https://codeberg.org/api/v1",
		org:     org,
		service: "Codeberg",
	}
	c.loadToken(token)
	return c
}

// NewGiteaClient creates a client for a Gitea-compatible service such as Forgejo.
func NewGiteaClient(baseURL, token, owner, service string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		org:     owner,
		token:   strings.TrimSpace(token),
		service: service,
	}
}

// NewForgejoClient creates a Forgejo client using protected token sources.
// FORGEJO_TOKEN takes precedence over ~/.gitsyncer_forgejo_token.
func NewForgejoClient(baseURL, owner string) *Client {
	return NewGiteaClient(baseURL, loadForgejoToken(), owner, "Forgejo")
}

func loadForgejoToken() string {
	if token, ok := os.LookupEnv("FORGEJO_TOKEN"); ok {
		if token = strings.TrimSpace(token); token != "" {
			return token
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	tokenPath := filepath.Join(home, ".gitsyncer_forgejo_token")
	info, err := os.Lstat(tokenPath)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return ""
	}
	tokenFile, err := os.Open(tokenPath)
	if err != nil {
		return ""
	}
	defer func() { _ = tokenFile.Close() }()

	data, err := io.ReadAll(tokenFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
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

// GetRepo fetches a repository by name
func (c *Client) GetRepo(repoName string) (Repository, bool, error) {
	var repo Repository
	url := fmt.Sprintf("%s/repos/%s/%s", c.baseURL, c.org, repoName)
	req, cancel, err := httpclient.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return repo, false, err
	}
	defer cancel()
	if c.HasToken() {
		req.Header.Set("Authorization", "token "+c.token)
	}

	resp, err := httpclient.Do(req)
	if err != nil {
		return repo, false, err
	}
	defer closeResponseBody(resp)

	if resp.StatusCode == 404 {
		return repo, false, nil
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return repo, false, fmt.Errorf("failed to get repo: status %d: %s", resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(&repo); err != nil {
		return repo, false, fmt.Errorf("failed to parse response: %w", err)
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

// UpdateRepoDescription updates a repository description on Codeberg
func (c *Client) UpdateRepoDescription(repoName, description string) error {
	if !c.HasToken() {
		return fmt.Errorf("Codeberg token required to update repository")
	}

	url := fmt.Sprintf("%s/repos/%s/%s", c.baseURL, c.org, repoName)
	payload := map[string]interface{}{
		"description": description,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, cancel, err := httpclient.NewRequest(http.MethodPatch, url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	defer cancel()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "token "+c.token)

	resp, err := httpclient.Do(req)
	if err != nil {
		return err
	}
	defer closeResponseBody(resp)

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to update Codeberg description: %s - %s", resp.Status, string(b))
	}
	return nil
}

// ListPublicRepos lists all public repositories for an organization
func (c *Client) ListPublicRepos() ([]Repository, error) {
	var allRepos []Repository
	page := 1
	perPage := 50

	for {
		url := fmt.Sprintf("%s/orgs/%s/repos?page=%d&limit=%d", c.baseURL, c.org, page, perPage)

		repos, err := c.listReposPage(url)
		if err != nil {
			return nil, err
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

		repos, err := c.listReposPage(url)
		if err != nil {
			return nil, err
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

func (c *Client) listReposPage(url string) ([]Repository, error) {
	req, cancel, err := httpclient.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	defer cancel()

	resp, err := httpclient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch repositories: %w", err)
	}
	defer closeResponseBody(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var repos []Repository
	if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return repos, nil
}

// RepoExists checks if a repository exists on Codeberg
func (c *Client) RepoExists(repoName string) (bool, error) {
	url := fmt.Sprintf("%s/repos/%s/%s", c.baseURL, c.org, repoName)
	req, cancel, err := httpclient.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	defer cancel()

	if c.HasToken() {
		req.Header.Set("Authorization", "token "+c.token)
	}

	resp, err := httpclient.Do(req)
	if err != nil {
		return false, err
	}
	defer closeResponseBody(resp)

	return resp.StatusCode == 200, nil
}

// CreateRepo creates a new repository on Codeberg
func (c *Client) CreateRepo(repoName, description string, private bool) error {
	if !c.HasToken() {
		return fmt.Errorf("%s token required to create repository", c.service)
	}
	exists, err := forge.CheckRepoExists(repoName, c.RepoExists)
	if err != nil {
		return err
	}
	if exists {
		return nil // Repository already exists
	}

	url := fmt.Sprintf("%s/user/repos", c.baseURL)

	payload := map[string]interface{}{
		"name":        repoName,
		"description": description,
		"private":     private,
		"auto_init":   false,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, cancel, err := httpclient.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	defer cancel()

	req.Header.Set("Content-Type", "application/json")
	if c.HasToken() {
		req.Header.Set("Authorization", "token "+c.token)
	}

	resp, err := httpclient.Do(req)
	if err != nil {
		return err
	}
	defer closeResponseBody(resp)

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

	var created Repository
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return fmt.Errorf("failed to validate created %s repository: %w", c.service, err)
	}
	if created.Name != repoName || created.FullName != c.org+"/"+repoName {
		return fmt.Errorf("%s repository created under wrong owner: API returned %q, expected %q", c.service, created.FullName, c.org+"/"+repoName)
	}
	if created.Private {
		return fmt.Errorf("created %s repository %s is unexpectedly private", c.service, created.FullName)
	}

	return nil
}

// EnsurePublicRepo creates an absent public repository and rejects unsafe collisions.
func (c *Client) EnsurePublicRepo(repoName, description string) error {
	if !c.HasToken() {
		return fmt.Errorf("%s token required to manage backup repository", c.service)
	}
	repo, exists, err := c.GetRepo(repoName)
	if err != nil {
		return fmt.Errorf("check %s repository %s/%s: %w", c.service, c.org, repoName, err)
	}
	if !exists {
		return c.CreateRepo(repoName, description, false)
	}
	if repo.Name != repoName || (repo.FullName != "" && repo.FullName != c.org+"/"+repoName) {
		return fmt.Errorf("%s repository collision: API returned %q for %s/%s", c.service, repo.FullName, c.org, repoName)
	}
	if repo.Private {
		return fmt.Errorf("%s repository %s/%s is unexpectedly private", c.service, c.org, repoName)
	}
	return nil
}

// DeleteRepo deletes a repository from Codeberg
func (c *Client) DeleteRepo(repoName string) error {
	if !c.HasToken() {
		return fmt.Errorf("Codeberg token required to delete repository")
	}

	if err := forge.EnsureRepoExists(c.org, repoName, c.RepoExists); err != nil {
		return err
	}

	url := fmt.Sprintf("%s/repos/%s/%s", c.baseURL, c.org, repoName)

	req, cancel, err := httpclient.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	defer cancel()

	req.Header.Set("Authorization", "token "+c.token)

	resp, err := httpclient.Do(req)
	if err != nil {
		return err
	}
	defer closeResponseBody(resp)

	body, _ := io.ReadAll(resp.Body)
	return forge.DeleteStatusError(resp.StatusCode, string(body))
}
