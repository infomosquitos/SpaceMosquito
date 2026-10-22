package scraper

import (
	"time"

	"github.com/vkh/spacemosquito/internal/store"
)

// apiVersion is the "version" object of Confluence REST content (Cloud and
// Data Center): number, last update time and its author.
type apiVersion struct {
	Number int     `json:"number"`
	When   string  `json:"when"`
	By     apiUser `json:"by"`
}

// apiHistory is the "history" object (expand=history): page creation.
type apiHistory struct {
	CreatedDate string  `json:"createdDate"`
	CreatedBy   apiUser `json:"createdBy"`
}

type apiUser struct {
	DisplayName string `json:"displayName"`
	PublicName  string `json:"publicName"`
	Username    string `json:"username"`
}

func (u apiUser) name() string {
	switch {
	case u.DisplayName != "":
		return u.DisplayName
	case u.PublicName != "":
		return u.PublicName
	}
	return u.Username
}

// pageHistory maps Confluence version/history to store.PageHistory;
// unparseable or missing values stay unknown.
func pageHistory(v apiVersion, h apiHistory) store.PageHistory {
	return store.PageHistory{
		CreatedBy: h.CreatedBy.name(),
		CreatedAt: parseAPITime(h.CreatedDate),
		UpdatedBy: v.By.name(),
		UpdatedAt: parseAPITime(v.When),
	}
}

func parseAPITime(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	t = t.UTC()
	return &t
}

// mergeHistory fills fields of dst that src knows.
func mergeHistory(dst *store.PageHistory, src store.PageHistory) {
	if src.CreatedBy != "" {
		dst.CreatedBy = src.CreatedBy
	}
	if src.CreatedAt != nil {
		dst.CreatedAt = src.CreatedAt
	}
	if src.UpdatedBy != "" {
		dst.UpdatedBy = src.UpdatedBy
	}
	if src.UpdatedAt != nil {
		dst.UpdatedAt = src.UpdatedAt
	}
}

// refreshHistory stores the Confluence history discovered for a page that the
// crawl skips as unchanged, so pages saved by earlier versions get it too.
func (s *Scraper) refreshHistory(spaceKey string, pg *Page) {
	if err := s.db.UpdatePageHistory(s.ctx, spaceKey, pg.ConfluenceID, pg.History); err != nil {
		s.log.Warnw("update page history failed", "page_id", pg.ConfluenceID, "error", err)
	}
}

func derefTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}
