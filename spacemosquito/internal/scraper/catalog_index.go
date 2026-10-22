package scraper

import (
	"context"
	"os"
	"path/filepath"

	"github.com/vkh/spacemosquito/internal/catalogindex"
)

// catalogIndexRefreshEvery is how many newly saved pages trigger a page tree
// rewrite during a space crawl.
const catalogIndexRefreshEvery = 20

// catalogIndexRefresher keeps a space's Markdown page tree
// (saved/<KEY>/index.md) current during a crawl. Creating saved/<KEY> at the
// start also makes the space browsable (browse renders the tree from the DB)
// while the crawl is still running.
type catalogIndexRefresher struct {
	s        *Scraper
	spaceKey string
	saved    int
}

// startCatalogIndex writes the space's page tree before the first page is
// saved, creating saved/<KEY> for a space crawled for the first time.
func (s *Scraper) startCatalogIndex(spaceKey string) *catalogIndexRefresher {
	if base := s.storage.BasePath(); base != "" && spaceKey != "" && filepath.Base(spaceKey) == spaceKey {
		if err := os.MkdirAll(filepath.Join(base, spaceKey), 0o755); err != nil {
			s.log.Warnw("failed to create space directory", "space_key", spaceKey, "error", err)
		}
	}
	s.writeCatalogIndex(spaceKey)
	return &catalogIndexRefresher{s: s, spaceKey: spaceKey}
}

// pageSaved counts a newly saved page and rewrites the tree every
// catalogIndexRefreshEvery pages.
func (r *catalogIndexRefresher) pageSaved() {
	if r.saved++; r.saved%catalogIndexRefreshEvery == 0 {
		r.s.writeCatalogIndex(r.spaceKey)
	}
}

// finish writes the final tree; it also runs when the crawl is cancelled or
// fails, so pages saved so far are listed.
func (r *catalogIndexRefresher) finish() {
	r.s.writeCatalogIndex(r.spaceKey)
}

// writeCatalogIndex rebuilds the space and root index.md files
// (best-effort: a failure leaves the previous index and is only logged).
// It ignores crawl cancellation so a stopped crawl still updates the index.
func (s *Scraper) writeCatalogIndex(spaceKey string) {
	ctx := context.Background()
	if s.ctx != nil {
		ctx = context.WithoutCancel(s.ctx)
	}
	if err := catalogindex.WriteSpace(ctx, s.db, s.storage.BasePath(), spaceKey); err != nil {
		s.log.Warnw("failed to write catalog index", "space_key", spaceKey, "error", err)
	}
}
