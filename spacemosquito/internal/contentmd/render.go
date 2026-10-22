package contentmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/vkh/spacemosquito/internal/contentmd/csf"
)

// RenderDirMarkdown converts a saved page directory to Markdown, routing by
// source format (see DetectBodyFormat):
//
//   - "storage" → the CSF converter over raw.html (the stored Storage Format);
//   - "rendered" → the generic HTML converter over index.html (clean extracted
//     HTML), falling back to raw.html.
//
// It returns ("", reason, nil) when nothing convertible is found; a Storage
// Format page with an empty body yields ("", "", nil). Asset requests
// from CSF conversion are ignored here — reindex/import is offline and only
// regenerates Markdown; the emitted asset links are correct and populate on the
// next crawl.
func RenderDirMarkdown(dir string) (md, skipReason string, err error) {
	if DetectBodyFormat(dir) == "storage" {
		raw, err := os.ReadFile(filepath.Join(dir, "raw.html"))
		if err != nil {
			if os.IsNotExist(err) {
				return "", "missing raw.html", nil
			}
			return "", "", err
		}
		out, _, err := csf.CSFToMarkdown(string(raw), nil)
		if err != nil {
			return "", "", err
		}
		// Empty is valid: pages without own content (e.g. parents of other
		// pages) exist in Confluence; callers keep them with empty Markdown.
		return out, "", nil
	}

	// Rendered HTML: prefer clean index.html, fall back to raw.html.
	md, err = HTMLFileToMarkdown(filepath.Join(dir, "index.html"))
	if err == nil && strings.TrimSpace(md) != "" {
		return md, "", nil
	}
	if err != nil && !os.IsNotExist(err) {
		return "", "", err
	}
	md, err = HTMLFileToMarkdown(filepath.Join(dir, "raw.html"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", "missing index.html and raw.html", nil
		}
		return "", "", err
	}
	if strings.TrimSpace(md) == "" {
		return "", "empty content in index.html and raw.html", nil
	}
	return md, "", nil
}

// DetectBodyFormat decides whether a saved page directory holds Confluence
// Storage Format ("storage") or rendered HTML ("rendered"). It trusts the
// body_format recorded in metadata.json (written by new crawls), and for legacy
// pages that lack it, sniffs raw.html for CSF markers.
func DetectBodyFormat(dir string) string {
	if b, err := os.ReadFile(filepath.Join(dir, "metadata.json")); err == nil {
		var m struct {
			BodyFormat string `json:"body_format"`
		}
		if json.Unmarshal(b, &m) == nil {
			switch m.BodyFormat {
			case "storage", "rendered":
				return m.BodyFormat
			}
		}
	}
	if raw, err := os.ReadFile(filepath.Join(dir, "raw.html")); err == nil && csf.IsStorageFormat(string(raw)) {
		return "storage"
	}
	return "rendered"
}
