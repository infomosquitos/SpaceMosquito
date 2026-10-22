package api

import (
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// BrowsePrefix is the URL prefix under which the saved catalog is served.
const BrowsePrefix = "/browse/"

// BrowseHandler serves the saved catalog (index.html trees, pages, assets)
// read-only under BrowsePrefix, so the browser extension can open it: browsers
// do not let extensions open file:// URLs. Directories are served only via
// their index.html; there are no directory listings.
func BrowseHandler(savedBase string) http.Handler {
	files := http.FileServer(http.Dir(savedBase))
	return http.StripPrefix(strings.TrimSuffix(BrowsePrefix, "/"), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := path.Clean("/" + r.URL.Path)
		full := filepath.Join(savedBase, filepath.FromSlash(name))
		if st, err := os.Stat(full); err == nil && st.IsDir() {
			if _, err := os.Stat(filepath.Join(full, "index.html")); err != nil {
				http.NotFound(w, r)
				return
			}
		}
		files.ServeHTTP(w, r)
	}))
}

// SpaceBrowsePath returns the catalog URL path of a space's page tree, or ""
// when the space has no generated index yet.
func SpaceBrowsePath(savedBase, spaceKey string) string {
	if savedBase == "" || spaceKey == "" {
		return ""
	}
	if _, err := os.Stat(filepath.Join(savedBase, spaceKey, "index.html")); err != nil {
		return ""
	}
	return BrowsePrefix + url.PathEscape(spaceKey) + "/"
}

// PageBrowsePath returns the catalog URL path of a saved page given its
// recorded html_path (only the page directory name is trusted, as the
// catalog may have moved), or "" when the page directory is not under
// savedBase/spaceKey.
func PageBrowsePath(savedBase, spaceKey, htmlPath string) string {
	if savedBase == "" || spaceKey == "" || htmlPath == "" {
		return ""
	}
	dir := filepath.Base(filepath.Dir(htmlPath))
	if st, err := os.Stat(filepath.Join(savedBase, spaceKey, dir)); err != nil || !st.IsDir() {
		return ""
	}
	return BrowsePrefix + url.PathEscape(spaceKey) + "/" + url.PathEscape(dir) + "/"
}
