package codeberg

import (
	"encoding/json"
	"fmt"
	"net/http"
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
}

// NewClient creates a new Codeberg API client
func NewClient(org string) Client {
	return Client{
		baseURL: "https://codeberg.org/api/v1",
		org:     org,
	}
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
