// Package updatecheck tells the user when a newer SpaceMosquito release is
// available and how to upgrade. It asks GitHub for the latest release at most
// once per CacheTTL (the answer is cached in the data dir), never blocks for
// long, and stays silent on any error or for development builds.
package updatecheck

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// LatestReleaseAPI returns the newest non-prerelease GitHub release.
	LatestReleaseAPI = "https://api.github.com/repos/infomosquitos/spacemosquito/releases/latest"
	releasesURL      = "https://github.com/infomosquitos/spacemosquito/releases"
	// CacheFile is stored in the data dir.
	CacheFile = "update-check.json"
	CacheTTL  = 24 * time.Hour
	// EnvDisable turns the check off (any non-empty value).
	EnvDisable = "SPACEMOSQUITO_NO_UPDATE_CHECK"
)

// Checker looks up the latest release for the running version.
type Checker struct {
	Current   string // running version, e.g. "v0.8.1" ("dev" disables)
	CachePath string // update-check.json; "" disables caching
	APIURL    string // default LatestReleaseAPI
	Client    *http.Client
	Now       func() time.Time
}

type cacheEntry struct {
	CheckedAt time.Time `json:"checked_at"`
	Latest    string    `json:"latest"`
}

// Enabled reports whether checks may run for this build and environment.
func (c *Checker) Enabled() bool {
	_, ok := parseVersion(c.Current)
	return ok && os.Getenv(EnvDisable) == ""
}

// Cached returns the latest version from the cache without network access,
// and whether it is newer than the running one.
func (c *Checker) Cached() (string, bool) {
	if !c.Enabled() {
		return "", false
	}
	e, ok := c.readCache()
	if !ok {
		return "", false
	}
	return e.Latest, Newer(e.Latest, c.Current)
}

// Latest returns the latest released version and whether it is newer than
// the running one, using the cache when it is fresh. Errors mean "unknown".
func (c *Checker) Latest(ctx context.Context) (string, bool, error) {
	if !c.Enabled() {
		return "", false, nil
	}
	if e, ok := c.readCache(); ok && c.now().Sub(e.CheckedAt) < CacheTTL {
		return e.Latest, Newer(e.Latest, c.Current), nil
	}
	latest, err := c.fetch(ctx)
	if err != nil {
		return "", false, err
	}
	c.writeCache(cacheEntry{CheckedAt: c.now(), Latest: latest})
	return latest, Newer(latest, c.Current), nil
}

func (c *Checker) fetch(ctx context.Context) (string, error) {
	api := c.APIURL
	if api == "" {
		api = LatestReleaseAPI
	}
	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "spacemosquito/"+c.Current)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("latest release: HTTP %d", resp.StatusCode)
	}
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", err
	}
	if _, ok := parseVersion(rel.TagName); !ok {
		return "", fmt.Errorf("latest release: unexpected tag %q", rel.TagName)
	}
	return rel.TagName, nil
}

func (c *Checker) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Checker) readCache() (cacheEntry, bool) {
	var e cacheEntry
	if c.CachePath == "" {
		return e, false
	}
	b, err := os.ReadFile(c.CachePath)
	if err != nil || json.Unmarshal(b, &e) != nil {
		return e, false
	}
	_, ok := parseVersion(e.Latest)
	return e, ok
}

func (c *Checker) writeCache(e cacheEntry) {
	if c.CachePath == "" {
		return
	}
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(c.CachePath), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(c.CachePath, b, 0o644)
}

// Newer reports whether version a is newer than b (vMAJOR.MINOR.PATCH).
func Newer(a, b string) bool {
	va, okA := parseVersion(a)
	vb, okB := parseVersion(b)
	if !okA || !okB {
		return false
	}
	for i := range va {
		if va[i] != vb[i] {
			return va[i] > vb[i]
		}
	}
	return false
}

// parseVersion parses "v1.2.3" or "1.2.3"; pre-releases ("-rc1") are not
// versions to upgrade to or from.
func parseVersion(s string) ([3]int, bool) {
	var v [3]int
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(s), "v"), ".")
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

// Notice is the message shown when latest is newer than current. exePath is
// the running binary (os.Executable); Homebrew installs get the brew command.
func Notice(current, latest, exePath string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "A new SpaceMosquito version is available: %s (you have %s).\n", latest, current)
	if InstalledByHomebrew(exePath) {
		b.WriteString("  Upgrade:  brew upgrade --cask spacemosquito && spacemosquito version\n")
	} else {
		fmt.Fprintf(&b, "  Download: %s/latest\n", releasesURL)
	}
	b.WriteString("  Then restart `spacemosquito serve`. ")
	fmt.Fprintf(&b, "What's new: %s/tag/%s\n", releasesURL, latest)
	return b.String()
}

// InstalledByHomebrew reports whether exePath is a Homebrew cask binary
// (…/Caskroom/spacemosquito/<version>/…), following the brew bin symlink.
func InstalledByHomebrew(exePath string) bool {
	if exePath == "" {
		return false
	}
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
		exePath = resolved
	}
	return strings.Contains(filepath.ToSlash(exePath), "/Caskroom/spacemosquito/")
}
