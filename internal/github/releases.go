package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"codeberg.org/snonux/gitsyncer/internal/forge"
	"codeberg.org/snonux/gitsyncer/internal/httpclient"
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

const githubAccept = "application/vnd.github.v3+json"

// GetReleases fetches the tag names of existing releases for the repository.
// A 404 (repository missing on GitHub) is normalized to an empty result so the
// caller can treat it as "no releases yet" instead of an error. No token is
// required: when the client has no token the request is sent unauthenticated,
// which still lists releases for public repositories (matching GitHub's
// unauthenticated rate-limited access).
func (c *Client) GetReleases(owner, repo string) ([]string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases", owner, repo)
	req, cancel, err := httpclient.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	defer cancel()

	// Add GitHub token if available
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("Accept", githubAccept)

	resp, err := httpclient.Do(req)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(resp)

	if resp.StatusCode == 404 {
		return []string{}, nil
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error: %s - %s", resp.Status, string(body))
	}

	var releases []Release
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}

	tags := make([]string, 0, len(releases))
	for _, release := range releases {
		tags = append(tags, release.TagName)
	}
	return tags, nil
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

	req, cancel, err := httpclient.NewRequest(http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer cancel()

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", githubAccept)

	resp, err := httpclient.Do(req)
	if err != nil {
		return err
	}
	defer closeResponseBody(resp)

	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create GitHub release: %s - %s", resp.Status, string(body))
	}
	return nil
}

// UpdateRelease updates the body of an existing release for the given tag on GitHub.
func (c *Client) UpdateRelease(owner, repo, tag, releaseNotes string) error {
	if c.token == "" {
		return fmt.Errorf("GitHub token is required for updating releases")
	}

	// First, look up the release id by tag.
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/tags/%s", owner, repo, tag)
	req, cancel, err := httpclient.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	defer cancel()

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", githubAccept)

	resp, err := httpclient.Do(req)
	if err != nil {
		return err
	}
	defer closeResponseBody(resp)

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to get release: %s - %s", resp.Status, string(body))
	}

	var releaseInfo struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&releaseInfo); err != nil {
		return err
	}

	updateURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/%d", owner, repo, releaseInfo.ID)
	release := Release{
		TagName: tag,
		Name:    tag,
		Body:    releaseNotes,
	}

	jsonData, err := json.Marshal(release)
	if err != nil {
		return err
	}

	updateReq, updateCancel, err := httpclient.NewRequest(http.MethodPatch, updateURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer updateCancel()

	updateReq.Header.Set("Authorization", "Bearer "+c.token)
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.Header.Set("Accept", githubAccept)

	updateResp, err := httpclient.Do(updateReq)
	if err != nil {
		return err
	}
	defer closeResponseBody(updateResp)

	if updateResp.StatusCode != 200 {
		body, _ := io.ReadAll(updateResp.Body)
		return fmt.Errorf("failed to update GitHub release: %s - %s", updateResp.Status, string(body))
	}
	return nil
}
