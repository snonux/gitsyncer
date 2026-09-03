package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/snonux/gitsyncer/internal/forge"
	"github.com/snonux/gitsyncer/internal/httpclient"
)

// Compile-time guarantees that the GitHub client satisfies the forge release
// contracts.
var _ forge.ReleaseClient = (*Client)(nil)

// Release represents a GitHub release as returned by the releases API.
type Release struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Body    string `json:"body"`
}

const (
	githubAccept          = "application/vnd.github.v3+json"
	githubReleasesPerPage = 100
)

// GetReleases fetches the tag names of existing releases for the repository.
// GitHub paginates this endpoint (100 per page here; the API default is 30),
// so every page is walked until a short page. A 404 (repository missing) is
// normalized to an empty result so the caller can treat it as "no releases
// yet". No token is required: when the client has no token the request is
// sent unauthenticated, which still lists releases for public repositories.
func (c *Client) GetReleases(owner, repo string) ([]string, error) {
	var tags []string
	for page := 1; ; page++ {
		pageTags, done, err := c.getReleasesPage(owner, repo, page)
		if err != nil {
			return nil, err
		}
		tags = append(tags, pageTags...)
		if done {
			return tags, nil
		}
	}
}

func (c *Client) getReleasesPage(owner, repo string, page int) (tags []string, done bool, err error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases?page=%d&per_page=%d", owner, repo, page, githubReleasesPerPage)
	headers := map[string]string{"Accept": githubAccept}
	if c.token != "" {
		headers["Authorization"] = "Bearer " + c.token
	}

	result, err := httpclient.DoJSON(http.MethodGet, url, headers, nil)
	if err != nil {
		return nil, true, err
	}

	if result.StatusCode == http.StatusNotFound {
		return nil, true, nil
	}
	if result.StatusCode != http.StatusOK {
		return nil, true, fmt.Errorf("GitHub API error: %s - %s", result.Status, string(result.Body))
	}

	var releases []Release
	if err := json.Unmarshal(result.Body, &releases); err != nil {
		return nil, true, err
	}

	tags = make([]string, 0, len(releases))
	for _, release := range releases {
		tags = append(tags, release.TagName)
	}
	return tags, len(releases) < githubReleasesPerPage, nil
}

// CreateRelease creates a release for the given tag on GitHub.
func (c *Client) CreateRelease(owner, repo, tag, releaseNotes string) error {
	if c.token == "" {
		return fmt.Errorf("GitHub token is required for creating releases")
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases", owner, repo)

	body := releaseNotes
	if body == "" {
		body = fmt.Sprintf("Release %s", tag)
	}

	release := Release{
		TagName: tag,
		Name:    tag,
		Body:    body,
	}

	jsonData, err := json.Marshal(release)
	if err != nil {
		return err
	}

	result, err := httpclient.DoJSON(http.MethodPost, url, map[string]string{
		"Authorization": "Bearer " + c.token,
		"Content-Type":  "application/json",
		"Accept":        githubAccept,
	}, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	if result.StatusCode == http.StatusCreated {
		return nil
	}
	if releaseAlreadyExists(result.StatusCode, result.Body) {
		return forge.ErrReleaseAlreadyExists
	}
	return fmt.Errorf("failed to create GitHub release: %s - %s", result.Status, string(result.Body))
}

func releaseAlreadyExists(status int, body []byte) bool {
	if status != http.StatusUnprocessableEntity {
		return false
	}

	var errResp ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		return false
	}
	for _, e := range errResp.Errors {
		if e.Resource == "Release" && e.Code == "already_exists" && e.Field == "tag_name" {
			return true
		}
	}
	return false
}

// UpdateRelease updates the body of an existing release for the given tag on GitHub.
func (c *Client) UpdateRelease(owner, repo, tag, releaseNotes string) error {
	if c.token == "" {
		return fmt.Errorf("GitHub token is required for updating releases")
	}

	releaseID, err := c.releaseIDForTag(owner, repo, tag)
	if err != nil {
		return err
	}

	return c.patchRelease(owner, repo, releaseID, tag, releaseNotes)
}

// releaseIDForTag looks up the numeric release id for a tag, since GitHub's
// update endpoint is keyed by id rather than tag name.
func (c *Client) releaseIDForTag(owner, repo, tag string) (int64, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/tags/%s", owner, repo, tag)
	result, err := httpclient.DoJSON(http.MethodGet, url, map[string]string{
		"Authorization": "Bearer " + c.token,
		"Accept":        githubAccept,
	}, nil)
	if err != nil {
		return 0, err
	}

	if result.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("failed to get release: %s - %s", result.Status, string(result.Body))
	}

	var releaseInfo struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(result.Body, &releaseInfo); err != nil {
		return 0, err
	}
	return releaseInfo.ID, nil
}

// patchRelease sends the actual release-body update for a known release id.
// Split out of UpdateRelease so the id lookup and the patch each stay close
// to the ~30-line guideline.
func (c *Client) patchRelease(owner, repo string, releaseID int64, tag, releaseNotes string) error {
	updateURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/%d", owner, repo, releaseID)
	release := Release{
		TagName: tag,
		Name:    tag,
		Body:    releaseNotes,
	}

	jsonData, err := json.Marshal(release)
	if err != nil {
		return err
	}

	updateResult, err := httpclient.DoJSON(http.MethodPatch, updateURL, map[string]string{
		"Authorization": "Bearer " + c.token,
		"Content-Type":  "application/json",
		"Accept":        githubAccept,
	}, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	if updateResult.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to update GitHub release: %s - %s", updateResult.Status, string(updateResult.Body))
	}
	return nil
}
