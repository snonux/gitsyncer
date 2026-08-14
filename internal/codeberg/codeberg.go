package codeberg

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
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
	baseURL   string
	org       string
	token     string
	service   string
	ownerType forge.OwnerType
}

var _ forge.RepoClient = (*Client)(nil)
var _ forge.RepoDescriptionClient = (*Client)(nil)
var _ forge.PublicRepoEnsurer = (*Client)(nil)

// NewClient creates a new Codeberg API client
func NewClient(token, org string) *Client {
	c := &Client{
		baseURL:   "https://codeberg.org/api/v1",
		org:       org,
		service:   "Codeberg",
		ownerType: forge.OwnerTypeUser,
	}
	c.loadToken(token)
	return c
}

// NewGiteaClient creates a client for a Gitea-compatible service such as Forgejo.
func NewGiteaClient(baseURL, token, owner, service string, ownerType forge.OwnerType) *Client {
	if ownerType == "" {
		ownerType = forge.OwnerTypeUser
	}
	return &Client{
		baseURL:   strings.TrimRight(baseURL, "/"),
		org:       owner,
		token:     strings.TrimSpace(token),
		service:   service,
		ownerType: ownerType,
	}
}

// NewForgejoClient creates a Forgejo client using protected token sources.
// FORGEJO_TOKEN takes precedence over ~/.gitsyncer_forgejo_token.
func NewForgejoClient(baseURL, owner string, ownerType forge.OwnerType) *Client {
	return NewGiteaClient(baseURL, loadForgejoToken(), owner, "Forgejo", ownerType)
}

// loadForgejoToken resolves the Forgejo token via the shared FORGEJO_TOKEN
// env var -> ~/.gitsyncer_forgejo_token cascade in forge.ResolveToken (there
// is no config-file source for Forgejo tokens, so the config slot is passed
// empty and resolution always falls through to env, then file).
func loadForgejoToken() string {
	return forge.ResolveToken("", "FORGEJO_TOKEN", ".gitsyncer_forgejo_token")
}

// loadToken resolves the Codeberg API token via the shared config ->
// CODEBERG_TOKEN env var -> ~/.gitsyncer_codeberg_token cascade in
// forge.ResolveToken, which is the single source of truth for this
// precedence (see its doc comment for why the cascade lives there rather
// than being reimplemented per forge, and for the trimming rules that used
// to differ between this method and its siblings).
func (c *Client) loadToken(tokenFromConfig string) {
	c.token = forge.ResolveToken(tokenFromConfig, "CODEBERG_TOKEN", ".gitsyncer_codeberg_token")
}

// HasToken returns true if a token is loaded
func (c *Client) HasToken() bool {
	return c.token != ""
}

// GetRepo fetches a repository by name
func (c *Client) GetRepo(repoName string) (Repository, bool, error) {
	var repo Repository
	url := fmt.Sprintf("%s/repos/%s/%s", c.baseURL, c.org, repoName)
	headers := map[string]string{}
	if c.HasToken() {
		headers["Authorization"] = "token " + c.token
	}

	result, err := httpclient.DoJSON(http.MethodGet, url, headers, nil)
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

	result, err := httpclient.DoJSON(http.MethodPatch, url, map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "token " + c.token,
	}, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	if result.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to update Codeberg description: %s - %s", result.Status, string(result.Body))
	}
	return nil
}

// ListPublicRepos lists all public repositories for an organization.
// The pagination loop and public/non-fork/non-archived/non-empty filtering
// live in listReposPaginated; this method only supplies the org endpoint's
// URL template.
func (c *Client) ListPublicRepos() ([]Repository, error) {
	return c.listReposPaginated(func(page, perPage int) string {
		return fmt.Sprintf("%s/orgs/%s/repos?page=%d&limit=%d", c.baseURL, c.org, page, perPage)
	})
}

// ListUserPublicRepos lists all public repositories for a user. Like
// ListPublicRepos, it delegates to listReposPaginated and only differs in
// the URL template (users/ instead of orgs/).
func (c *Client) ListUserPublicRepos() ([]Repository, error) {
	return c.listReposPaginated(func(page, perPage int) string {
		return fmt.Sprintf("%s/users/%s/repos?page=%d&limit=%d", c.baseURL, c.org, page, perPage)
	})
}

// listReposPaginated walks a paginated repo-listing endpoint page by page,
// keeping only public, non-fork, non-archived, non-empty repos. urlBuilder
// receives the page number and page size and returns the URL to fetch for
// that page; it is the only thing that differs between ListPublicRepos
// (orgs/ endpoint) and ListUserPublicRepos (users/ endpoint). Pagination
// stops once a page returns fewer than perPage repos, matching the prior
// per-method loops exactly.
func (c *Client) listReposPaginated(urlBuilder func(page, perPage int) string) ([]Repository, error) {
	var allRepos []Repository
	page := 1
	perPage := 50

	for {
		repos, err := c.listReposPage(urlBuilder(page, perPage))
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
	result, err := httpclient.DoJSON(http.MethodGet, url, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch repositories: %w", err)
	}

	if result.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", result.StatusCode)
	}

	var repos []Repository
	if err := json.Unmarshal(result.Body, &repos); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return repos, nil
}

// RepoExists checks if a repository exists on Codeberg
func (c *Client) RepoExists(repoName string) (bool, error) {
	url := fmt.Sprintf("%s/repos/%s/%s", c.baseURL, c.org, repoName)
	headers := map[string]string{}
	if c.HasToken() {
		headers["Authorization"] = "token " + c.token
	}

	result, err := httpclient.DoJSON(http.MethodGet, url, headers, nil)
	if err != nil {
		return false, err
	}

	return result.StatusCode == http.StatusOK, nil
}

// createRepoURL resolves the create-repository endpoint for this client's
// owner type: a Codeberg/Gitea user account and an organization use
// different endpoints.
func (c *Client) createRepoURL() (string, error) {
	switch c.ownerType {
	case forge.OwnerTypeUser:
		return fmt.Sprintf("%s/user/repos", c.baseURL), nil
	case forge.OwnerTypeOrganization:
		return fmt.Sprintf("%s/orgs/%s/repos", c.baseURL, c.org), nil
	default:
		return "", fmt.Errorf("invalid %s owner type %q", c.service, c.ownerType)
	}
}

// CreateRepo creates a new repository on Codeberg
func (c *Client) CreateRepo(repoName, description string, private bool) error {
	url, err := c.createRepoURL()
	if err != nil {
		return err
	}
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

	return c.postCreateRepo(url, repoName, description, private)
}

// postCreateRepo sends the create-repository POST and validates the
// response: an unexpected owner on the created repo, or an unexpectedly
// private result, are both reported as errors so callers don't have to
// re-check what they asked for. Split out of CreateRepo so the
// existence-check flow and the HTTP round trip each stay close to the
// ~30-line guideline.
func (c *Client) postCreateRepo(url, repoName, description string, private bool) error {
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

	headers := map[string]string{"Content-Type": "application/json"}
	if c.HasToken() {
		headers["Authorization"] = "token " + c.token
	}

	result, err := httpclient.DoJSON(http.MethodPost, url, headers, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	if result.StatusCode != http.StatusCreated {
		return createRepoError(result.StatusCode, result.Body)
	}

	var created Repository
	if err := json.Unmarshal(result.Body, &created); err != nil {
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

// createRepoError builds the error for a non-201 create-repository response,
// preferring the API's own JSON error message when the body decodes.
func createRepoError(statusCode int, body []byte) error {
	var errorResp map[string]interface{}
	if err := json.Unmarshal(body, &errorResp); err == nil {
		if msg, ok := errorResp["message"].(string); ok {
			return fmt.Errorf("failed to create repository: %s (status code %d)", msg, statusCode)
		}
	}

	return fmt.Errorf("failed to create repository: %s (status code %d)", string(body), statusCode)
}

// EnsurePublicRepo creates an absent public repository and rejects unsafe collisions.
func (c *Client) EnsurePublicRepo(repoName, description string) error {
	if !c.ownerType.Valid() {
		return fmt.Errorf("invalid %s owner type %q", c.service, c.ownerType)
	}
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

	result, err := httpclient.DoJSON(http.MethodDelete, url, map[string]string{
		"Authorization": "token " + c.token,
	}, nil)
	if err != nil {
		return err
	}

	return forge.DeleteStatusError(result.StatusCode, string(result.Body))
}
