// Package confluence holds small shared helpers for working with Confluence
// URLs, usable by scraper, contentmd/csf, and reindex without import cycles.
package confluence

import (
	"fmt"
	"net/url"
	"strings"
)

// browseMarkers are path prefixes that begin Confluence browse/UI routes.
// Anything before the earliest marker is treated as the install context path
// (e.g. "/confluence"). "/wiki/" is checked first so Cloud URLs like
// /wiki/spaces/KEY keep an empty context (API calls append "/wiki/…" themselves).
var browseMarkers = []string{"/wiki/", "/spaces/", "/display/"}

// BaseURL returns the Confluence API/site base for a browse or space URL:
// scheme://host plus any context path before /wiki/, /spaces/, or /display/.
//
// Examples:
//   - https://x.atlassian.net/wiki/spaces/KEY → https://x.atlassian.net
//   - https://wiki.example.com/display/KEY → https://wiki.example.com
//   - https://wiki.example.com/confluence/display/KEY → https://wiki.example.com/confluence
//
// It returns "" for empty or unparseable input (missing scheme or host).
func BaseURL(urlStr string) string {
	if urlStr == "" {
		return ""
	}
	u, err := url.Parse(urlStr)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	origin := fmt.Sprintf("%s://%s", u.Scheme, u.Host)
	ctx := contextPath(u.Path)
	if ctx == "" {
		return origin
	}
	return origin + ctx
}

// contextPath returns the path prefix before the first browse marker, or "".
func contextPath(path string) string {
	if path == "" || path == "/" {
		return ""
	}
	earliest := -1
	for _, marker := range browseMarkers {
		if i := strings.Index(path, marker); i >= 0 {
			if earliest < 0 || i < earliest {
				earliest = i
			}
		}
	}
	if earliest <= 0 {
		return ""
	}
	return strings.TrimRight(path[:earliest], "/")
}

// DeriveSpaceURL builds a space browse URL from a page/space URL and space key
// when an explicit space URL is missing (e.g. auto-create). Returns "" if the
// base cannot be derived.
func DeriveSpaceURL(pageOrSpaceURL, spaceKey string) string {
	if spaceKey == "" {
		return ""
	}
	base := BaseURL(pageOrSpaceURL)
	if base == "" {
		return ""
	}
	lower := strings.ToLower(pageOrSpaceURL)
	switch {
	case strings.Contains(lower, "/wiki/"):
		return base + "/wiki/spaces/" + spaceKey
	case strings.Contains(lower, "/display/"):
		return base + "/display/" + spaceKey
	default:
		return base + "/spaces/" + spaceKey
	}
}

// WebUIURL resolves a REST `_links.webui` path against the BaseURL result.
// Cloud REST returns webui paths relative to /wiki (e.g. /spaces/KEY/pages/1)
// while BaseURL strips /wiki, so it is re-added for Cloud. Absolute input is
// returned unchanged.
func WebUIURL(baseURL, webui string, cloud bool) string {
	if webui == "" || strings.HasPrefix(webui, "http://") || strings.HasPrefix(webui, "https://") {
		return webui
	}
	base := strings.TrimRight(baseURL, "/")
	if cloud && !strings.HasPrefix(webui, "/wiki/") {
		base += "/wiki"
	}
	return base + webui
}
