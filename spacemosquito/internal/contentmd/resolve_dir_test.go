package contentmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vkh/spacemosquito/internal/store"
)

// A copied or moved catalog must never write into the catalog the DB paths
// were recorded in, even when that one still exists.
func TestResolveDir_staysInSavedBase(t *testing.T) {
	root := t.TempDir()
	original := filepath.Join(root, "original", "TST", "1-Page")
	copied := filepath.Join(root, "copy", "TST", "1-Page")
	for _, d := range []string{original, copied} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	page := store.Page{FileDir: original}

	if got := resolveDir(page, filepath.Join(root, "copy")); got != copied {
		t.Fatalf("resolveDir = %q, want the copy %q", got, copied)
	}
	if got := resolveDir(page, filepath.Join(root, "elsewhere")); got != "" {
		t.Fatalf("page missing from savedBase resolved to %q", got)
	}
	if got := resolveDir(page, ""); got != original {
		t.Fatalf("no savedBase: %q, want recorded %q", got, original)
	}
	// Browser-fallback pages recorded only by html_path.
	if got := resolveDir(store.Page{HTMLPath: filepath.Join(original, "index.html")}, filepath.Join(root, "copy")); got != copied {
		t.Fatalf("html_path: %q", got)
	}
}
