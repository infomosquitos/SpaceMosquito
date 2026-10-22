package scraper

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/vkh/spacemosquito/internal/ratelimit"
	"github.com/vkh/spacemosquito/internal/session"
)

// pageIDCache remembers page IDs looked up by space and title during a crawl
// (0 = not found), so each referenced page costs one request.
type pageIDCache struct {
	mu  sync.Mutex
	ids map[string]int
}

// pageIDResolver returns a lookup of page IDs by space key and title through
// the REST API, for attachments that belong to another page. spaceKey ""
// means defaultSpace. Failures resolve to 0 (the attachment is skipped).
func (s *Scraper) pageIDResolver(baseURL, defaultSpace string, sess *session.Session) func(spaceKey, title string) int {
	return func(spaceKey, title string) int {
		if spaceKey == "" {
			spaceKey = defaultSpace
		}
		key := spaceKey + "\x00" + title
		s.pageIDs.mu.Lock()
		if s.pageIDs.ids == nil {
			s.pageIDs.ids = map[string]int{}
		}
		id, ok := s.pageIDs.ids[key]
		s.pageIDs.mu.Unlock()
		if ok {
			return id
		}
		id, err := lookupPageID(baseURL, spaceKey, title, sess)
		if err != nil {
			s.log.Warnw("look up page by title failed", "space_key", spaceKey, "title", title, "error", err)
		}
		s.pageIDs.mu.Lock()
		s.pageIDs.ids[key] = id
		s.pageIDs.mu.Unlock()
		return id
	}
}

func lookupPageID(baseURL, spaceKey, title string, sess *session.Session) (int, error) {
	api := baseURL + "/rest/api/content"
	if sess.Flavor == session.FlavorCloud {
		api = baseURL + "/wiki/rest/api/content"
	}
	q := url.Values{"spaceKey": {spaceKey}, "title": {title}, "type": {"page"}, "limit": {"1"}}
	req, err := http.NewRequest(http.MethodGet, api+"?"+q.Encode(), nil)
	if err != nil {
		return 0, err
	}
	for k, v := range sess.AsHeaders() {
		req.Header.Set(k, v)
	}
	resp, err := ratelimit.Do(&http.Client{Timeout: 30 * time.Second}, req, nil)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var result struct {
		Results []struct {
			ID string `json:"id"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}
	if len(result.Results) == 0 {
		return 0, nil
	}
	return strconv.Atoi(result.Results[0].ID)
}

// logThrottle records that Confluence asked the crawler to slow down.
func (s *Scraper) logThrottle(wait time.Duration) {
	s.log.Warnw("confluence is throttling requests, waiting", "wait", wait.String())
}
