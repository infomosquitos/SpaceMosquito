package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sync"
	"time"

	"github.com/vkh/spacemosquito/internal/contentmd/csf"
	"github.com/vkh/spacemosquito/internal/ratelimit"
	"github.com/vkh/spacemosquito/internal/session"
)

// userCache holds display names by Confluence account ID for @-mentions,
// backed by the users table so each person is looked up once.
type userCache struct {
	mu     sync.Mutex
	loaded bool
	names  map[string]string
}

// mentionAccountID finds mentioned accounts: ri:account-id, or ri:userkey
// holding an account ID (pages from before Cloud).
var mentionAccountID = regexp.MustCompile(`<ri:user\b[^>]*\bri:(account-id|userkey)="([^"]+)"`)

// userName returns a known display name ("" = unknown).
func (s *Scraper) userName(accountID string) string {
	s.users.mu.Lock()
	defer s.users.mu.Unlock()
	s.loadUsersLocked()
	return s.users.names[accountID]
}

func (s *Scraper) loadUsersLocked() {
	if s.users.loaded {
		return
	}
	s.users.loaded = true
	s.users.names = map[string]string{}
	if s.db == nil {
		return
	}
	if names, err := s.db.UserNames(s.context()); err == nil {
		s.users.names = names
	}
}

// rememberUsers adds names (e.g. page authors from the API) to the cache and
// the database.
func (s *Scraper) rememberUsers(names map[string]string) {
	fresh := map[string]string{}
	s.users.mu.Lock()
	s.loadUsersLocked()
	for id, name := range names {
		if id != "" && name != "" && s.users.names[id] != name {
			s.users.names[id] = name
			fresh[id] = name
		}
	}
	s.users.mu.Unlock()
	if len(fresh) > 0 && s.db != nil {
		if err := s.db.SaveUserNames(s.context(), fresh); err != nil {
			s.log.Warnw("save user names failed", "error", err)
		}
	}
}

// resolveMentions looks up the names of accounts mentioned in storage HTML
// that are not known yet (Cloud bulk API, up to 100 per request).
func (s *Scraper) resolveMentions(baseURL, storageHTML string, sess *session.Session) {
	if sess.Flavor != session.FlavorCloud {
		return
	}
	var missing []string
	seen := map[string]bool{}
	for _, m := range mentionAccountID.FindAllStringSubmatch(storageHTML, -1) {
		id := m[2]
		if m[1] == "userkey" && !csf.IsAccountID(id) {
			continue // Server/DC user key, not a Cloud account
		}
		if !seen[id] && s.userName(id) == "" {
			seen[id] = true
			missing = append(missing, id)
		}
	}
	for start := 0; start < len(missing); start += 100 {
		end := min(start+100, len(missing))
		names, err := fetchUserNames(baseURL, missing[start:end], sess, s.logThrottle)
		if err != nil {
			s.log.Warnw("look up mentioned users failed", "count", end-start, "error", err)
			return
		}
		s.rememberUsers(names)
	}
}

func fetchUserNames(baseURL string, ids []string, sess *session.Session, onThrottle func(time.Duration)) (map[string]string, error) {
	q := url.Values{"limit": {"200"}}
	for _, id := range ids {
		q.Add("accountId", id)
	}
	req, err := http.NewRequest(http.MethodGet, baseURL+"/wiki/rest/api/user/bulk?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	for k, v := range sess.AsHeaders() {
		req.Header.Set(k, v)
	}
	resp, err := ratelimit.Do(&http.Client{Timeout: 30 * time.Second}, req, onThrottle)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var result struct {
		Results []struct {
			AccountID   string `json:"accountId"`
			DisplayName string `json:"displayName"`
			PublicName  string `json:"publicName"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	names := map[string]string{}
	for _, u := range result.Results {
		if name := firstNonEmptyString(u.DisplayName, u.PublicName); name != "" {
			names[u.AccountID] = name
		}
	}
	return names, nil
}

func firstNonEmptyString(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// historyUsers returns the account ID → name pairs that page history carries.
func historyUsers(v apiVersion, h apiHistory) map[string]string {
	out := map[string]string{}
	for _, u := range []apiUser{v.By, h.CreatedBy} {
		if u.AccountID != "" && u.name() != "" {
			out[u.AccountID] = u.name()
		}
	}
	return out
}

// context returns the crawl context, or Background outside a crawl.
func (s *Scraper) context() context.Context {
	if s.ctx != nil {
		return s.ctx
	}
	return context.Background()
}
