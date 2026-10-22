package scraper

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vkh/spacemosquito/internal/storage"
	"github.com/vkh/spacemosquito/pkg/logging"
)

// A first crawl has no saved/<KEY> yet; the page tree must still be written
// when the crawl starts so the space can be browsed while it runs.
func TestStartCatalogIndex_newSpace(t *testing.T) {
	db, cfg := openTestDB(t, t.TempDir())
	if _, err := db.CreateSpace(context.Background(), "NEW", "New", "https://example.atlassian.net/wiki/spaces/NEW"); err != nil {
		t.Fatal(err)
	}
	s := New(cfg, db, storage.NewWriter(cfg.Storage.BasePath, logging.Sugar{}), storage.NewAssetDownloader(logging.Sugar{}), logging.Sugar{})

	index := s.startCatalogIndex("NEW")
	for _, f := range []string{filepath.Join("NEW", "index.html"), "index.html"} {
		if _, err := os.Stat(filepath.Join(cfg.Storage.BasePath, f)); err != nil {
			t.Fatalf("%s not written at crawl start: %v", f, err)
		}
	}
	for range catalogIndexRefreshEvery {
		index.pageSaved()
	}
	index.finish()
}
