package api

import (
	"net/url"
	"os"
	"path/filepath"

	"github.com/vkh/spacemosquito/internal/browse"
)

// SpaceBrowsePath returns the browse URL path of a space's page tree, or ""
// when the space has no saved directory yet. A crawl creates the directory
// when it starts, so the space can be browsed while it runs.
func SpaceBrowsePath(savedBase, spaceKey string) string {
	if savedBase == "" || spaceKey == "" {
		return ""
	}
	if st, err := os.Stat(filepath.Join(savedBase, spaceKey)); err != nil || !st.IsDir() {
		return ""
	}
	return browse.Prefix + url.PathEscape(spaceKey) + "/"
}

// PageBrowsePath returns the browse URL path of a saved page given the path
// of a file in its directory (search file_path, html_path); only the page
// directory name is trusted, as the catalog may have moved. Returns "" when
// the page directory is not under savedBase/spaceKey.
func PageBrowsePath(savedBase, spaceKey, filePath string) string {
	if savedBase == "" || spaceKey == "" || filePath == "" {
		return ""
	}
	dir := filepath.Base(filepath.Dir(filePath))
	if st, err := os.Stat(filepath.Join(savedBase, spaceKey, dir)); err != nil || !st.IsDir() {
		return ""
	}
	return browse.Prefix + url.PathEscape(spaceKey) + "/" + url.PathEscape(dir) + "/"
}
