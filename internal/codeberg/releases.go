package codeberg

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"codeberg.org/snonux/gitsyncer/internal/forge"
	"codeberg.org/snonux/gitsyncer/internal/httpclient"
)

// Compile-time guarantees that the Codeberg client satisfies the forge release
// contracts. Codeberg/Gitea is the only built-in forge that needs releases
// enabled per-repository, so it also implements ReleasesEnabler.
var (
	_ forge.ReleaseClient   = (*Client)(nil)
	_ forge.ReleasesEnabler = (*Client)(nil)
)

// codebergRelease is the subset of the Gitea release payload we send and read.
type codebergRelease struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Body    string `json:"body"`
}

// repoReleaseSettings is the repository metadata consulted when deciding
// whether releases need enabling.
type repoReleaseSettings struct {
	HasReleases bool `json:"has_releases"`
}

func (c *Client) releasesURL(owner, repo string) string {
	return fmt.Sprintf("%s/repos/%s/%s/releases", c.baseURL, owner, repo)
}

func (c *Client) repoURL(owner, repo string) string {
	return fmt.Sprintf("%s/repos/%s/%s", c.baseURL, owner, repo)
}

// authRequest attaches the standard Gitea/Codeberg auth + content headers.
func (c *Client) authRequest(req *http.Request, contentType bool) {
	if c.token != "" {
		req.Header.Set("Authorization", "token "+c.token)
	}
	if contentType {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
}

// GetReleases fetches the tag names of existing releases for the repository.
// A 404 (repository missing on Codeberg) is normalized to an empty result so
// the caller can treat it as "no releases yet" instead of an error.
func (c *Client) GetReleases(owner, repo string) ([]string, error) {
	url := c.releasesURL(owner, repo)
	req, cancel, err := httpclient.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	defer cancel()
	c.authRequest(req, false)

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
		return nil, fmt.Errorf("%s API error: %s - %s", c.service, resp.Status, string(body))
	}

	var releases []codebergRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}

	tags := make([]string, 0, len(releases))
	for _, release := range releases {
		tags = append(tags, release.TagName)
	}
	return tags, nil
}

// EnsureReleasesEnabled ensures that the repository has the Releases feature
// enabled, enabling it via API when it is disabled. Codeberg/Gitea exposes
// releases as a per-repository toggle, unlike GitHub where releases are always
// available.
func (c *Client) EnsureReleasesEnabled(owner, repo string) error {
	if c.token == "" {
		return fmt.Errorf("%s token is required to manage repository settings", c.service)
	}

	infoURL := c.repoURL(owner, repo)
	settings, err := c.fetchReleaseSettings(infoURL)
	if err != nil {
		return err
	}
	if settings.HasReleases {
		return nil
	}

	return c.enableReleases(infoURL)
}

// fetchReleaseSettings reads the repository metadata and reports whether
// releases are currently enabled.
func (c *Client) fetchReleaseSettings(infoURL string) (repoReleaseSettings, error) {
	var settings repoReleaseSettings
	req, cancel, err := httpclient.NewRequest(http.MethodGet, infoURL, nil)
	if err != nil {
		return settings, err
	}
	defer cancel()
	c.authRequest(req, false)

	resp, err := httpclient.Do(req)
	if err != nil {
		return settings, err
	}
	defer closeResponseBody(resp)

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return settings, fmt.Errorf("failed to get repo info: %s - %s", resp.Status, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(&settings); err != nil {
		return settings, fmt.Errorf("failed to parse repo info: %w", err)
	}
	return settings, nil
}

// enableReleases patches the repository to enable the Releases feature.
func (c *Client) enableReleases(infoURL string) error {
	payload, err := json.Marshal(map[string]any{"has_releases": true})
	if err != nil {
		return err
	}
	req, cancel, err := httpclient.NewRequest(http.MethodPatch, infoURL, bytes.NewBuffer(payload))
	if err != nil {
		return err
	}
	defer cancel()
	c.authRequest(req, true)

	resp, err := httpclient.Do(req)
	if err != nil {
		return err
	}
	defer closeResponseBody(resp)

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to enable releases: %s - %s", resp.Status, string(body))
	}
	return nil
}

// postRelease sends the create-release request once and returns the HTTP
// status code, the full status string (e.g. "422 Unprocessable Entity"), and
// the raw body. It does not retry; the caller decides how to react.
func (c *Client) postRelease(url string, jsonData []byte) (int, string, []byte, error) {
	req, cancel, err := httpclient.NewRequest(http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return 0, "", nil, err
	}
	defer cancel()
	c.authRequest(req, true)

	resp, err := httpclient.Do(req)
	if err != nil {
		return 0, "", nil, err
	}
	defer closeResponseBody(resp)
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Status, body, nil
}

// CreateRelease creates a release for the given tag on Codeberg/Gitea. The
// Gitea release API returns 404 in two distinct situations (repository missing
// vs. releases disabled) and 409 for a known tag bug; both are handled here so
// callers get an actionable error instead of nested branching at the call
// site.
func (c *Client) CreateRelease(owner, repo, tag, releaseNotes string) error {
	if c.token == "" {
		return fmt.Errorf("%s token is required for creating releases", c.service)
	}

	body := releaseNotes
	if body == "" {
		body = fmt.Sprintf("Release %s", tag)
	}

	// Gitea requires tag_name; name/body/draft/prerelease round out the payload.
	release := map[string]any{
		"tag_name":   tag,
		"name":       tag,
		"body":       body,
		"draft":      false,
		"prerelease": false,
	}
	jsonData, err := json.Marshal(release)
	if err != nil {
		return err
	}

	url := c.releasesURL(owner, repo)
	status, statusText, respBody, err := c.postRelease(url, jsonData)
	if err != nil {
		return err
	}

	if status == 201 {
		return nil
	}

	switch {
	case status == 404:
		return c.handleCreate404(owner, repo, url, jsonData, respBody)
	case status == 409 && strings.Contains(string(respBody), "Release is has no Tag"):
		fmt.Printf("\nWARNING: %s/Gitea returned 'Release is has no Tag' error for tag %s\n", c.service, tag)
		fmt.Printf("This is a known issue with some old tags. The tag exists but cannot have a release created via API.\n")
		fmt.Printf("You may need to create this release manually through the %s web interface.\n\n", c.service)
		return fmt.Errorf("cannot create release for tag %s due to Gitea API limitation", tag)
	default:
		return fmt.Errorf("failed to create %s release: %s - %s", c.service, statusText, string(respBody))
	}
}

// handleCreate404 disambiguates a 404 on release creation. The repository may
// be missing (probe fails or returns non-200), exist with releases disabled
// (then we enable them and retry creation once), or exist with releases
// enabled (which points at a token scope/permission problem). Each case gets a
// distinct, actionable error message so callers do not need to branch.
//
// The probe is best-effort: a failure to reach the API, a non-200 response, or
// a body that fails to decode all route to the "releases disabled" path only
// when the repo is confirmed to exist (200); otherwise the 404 is reported as
// a missing repository. Decode failures on a 200 are swallowed (HasReleases
// stays false) so a malformed repo payload still triggers the enable+retry,
// matching the original inline implementation.
func (c *Client) handleCreate404(owner, repo, url string, jsonData []byte, respBody []byte) error {
	probeURL := c.repoURL(owner, repo)
	probeReq, probeCancel, perr := httpclient.NewRequest(http.MethodGet, probeURL, nil)
	if perr == nil {
		defer probeCancel()
		c.authRequest(probeReq, false)
		probeResp, perr2 := httpclient.Do(probeReq)
		if perr2 == nil {
			defer closeResponseBody(probeResp)
			if probeResp.StatusCode == 200 {
				var repoInfo struct {
					HasReleases bool `json:"has_releases"`
				}
				if data, rerr := io.ReadAll(probeResp.Body); rerr == nil {
					_ = json.Unmarshal(data, &repoInfo) // best-effort; ignore decode errors
				}
				if !repoInfo.HasReleases {
					// Releases are disabled: enable them and retry creation once.
					if err := c.enableReleases(probeURL); err != nil {
						return fmt.Errorf(
							"failed to create %s release: releases are disabled for %s/%s and enabling via API failed: %w. Raw response: %s",
							c.service, owner, repo, err, string(respBody),
						)
					}
					retryStatus, retryStatusText, retryBody, err := c.postRelease(url, jsonData)
					if err != nil {
						return err
					}
					if retryStatus != 201 {
						return fmt.Errorf("failed to create %s release after enabling releases: %s - %s", c.service, retryStatusText, string(retryBody))
					}
					return nil
				}
				// Repo exists and has releases enabled; a 404 here points at a
				// permission scope problem rather than a missing repository.
				return fmt.Errorf(
					"failed to create %s release: repo %s/%s exists but returned 404 on release creation. This usually indicates the token lacks write permissions to this repository or owner. Ensure the token belongs to '%s' (or a collaborator/maintainer) and has repository write access. Raw response: %s",
					c.service, owner, repo, owner, string(respBody),
				)
			}
		}
	}

	// The probe either failed to reach the API or returned a non-200 (most
	// commonly a 404 for the repository itself), so the create-release 404
	// means the repository is not visible to this token/owner.
	return fmt.Errorf(
		"failed to create %s release: repository %s/%s not found (404). Verify your %s owner ('organizations[].name') matches the actual owner for this repo and that the repository exists. If needed, create it first, e.g.: gitsyncer sync repo %s --create-%s-repos. Raw response: %s",
		c.service, owner, repo, c.service, repo, strings.ToLower(c.service), string(respBody),
	)
}

// UpdateRelease updates the body of an existing release for the given tag on
// Codeberg/Gitea.
func (c *Client) UpdateRelease(owner, repo, tag, releaseNotes string) error {
	if c.token == "" {
		return fmt.Errorf("%s token is required for updating releases", c.service)
	}

	// First, look up the release id by tag.
	url := fmt.Sprintf("%s/repos/%s/%s/releases/tags/%s", c.baseURL, owner, repo, tag)
	req, cancel, err := httpclient.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	defer cancel()
	c.authRequest(req, false)

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

	updateURL := fmt.Sprintf("%s/repos/%s/%s/releases/%d", c.baseURL, owner, repo, releaseInfo.ID)
	release := codebergRelease{
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
	c.authRequest(updateReq, true)

	updateResp, err := httpclient.Do(updateReq)
	if err != nil {
		return err
	}
	defer closeResponseBody(updateResp)

	if updateResp.StatusCode != 200 {
		body, _ := io.ReadAll(updateResp.Body)
		return fmt.Errorf("failed to update %s release: %s - %s", c.service, updateResp.Status, string(body))
	}
	return nil
}
