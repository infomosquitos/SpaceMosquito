package scraper

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/vkh/spacemosquito/internal/ratelimit"
	"github.com/vkh/spacemosquito/internal/session"
)

// ContentVersion is live Confluence page metadata (no body).
type ContentVersion struct {
	Number   int       `json:"version"`
	When     time.Time `json:"when,omitempty"`
	Title    string    `json:"title,omitempty"`
	SpaceKey string    `json:"space_key,omitempty"`
}

// FetchContentVersion GETs REST content/{id}?expand=version (and title/space) using the session.
func FetchContentVersion(sess *session.Session, spaceURL string, confluenceID int) (*ContentVersion, error) {
	if sess == nil {
		return nil, fmt.Errorf("session is required")
	}
	if confluenceID <= 0 {
		return nil, fmt.Errorf("confluence id must be positive")
	}

	baseURL := extractConfluenceBaseURL(spaceURL)
	if baseURL == "" {
		baseURL = strings.TrimRight(sess.ConfluenceURL, "/")
	}
	if baseURL == "" {
		return nil, fmt.Errorf("cannot resolve confluence base URL")
	}

	var apiURL string
	if sess.Flavor == session.FlavorCloud || (sess.Flavor == "" && strings.Contains(baseURL, "atlassian.net")) {
		apiURL = fmt.Sprintf("%s/wiki/rest/api/content/%d?expand=version,space", baseURL, confluenceID)
	} else {
		apiURL = fmt.Sprintf("%s/rest/api/content/%d?expand=version,space", baseURL, confluenceID)
	}

	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range sess.AsHeaders() {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := ratelimit.Do(client, req, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("page %d not found", confluenceID)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, session.UnauthorizedHTTP(resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	var result struct {
		Title   string `json:"title"`
		Version struct {
			Number int    `json:"number"`
			When   string `json:"when"`
		} `json:"version"`
		Space struct {
			Key string `json:"key"`
		} `json:"space"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	cv := &ContentVersion{
		Number:   result.Version.Number,
		Title:    result.Title,
		SpaceKey: result.Space.Key,
	}
	if result.Version.When != "" {
		if t, err := time.Parse(time.RFC3339, result.Version.When); err == nil {
			cv.When = t
		} else if t, err := time.Parse(time.RFC3339Nano, result.Version.When); err == nil {
			cv.When = t
		}
	}
	return cv, nil
}
