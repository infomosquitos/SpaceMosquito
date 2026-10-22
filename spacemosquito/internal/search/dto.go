package search

import (
	"time"

	"github.com/google/uuid"
	"github.com/vkh/spacemosquito/internal/store"
)

// SearchHit is the public search result shape (MCP, REST, CLI).
type SearchHit struct {
	ConfluenceID int     `json:"confluence_id"`
	SpaceKey     string  `json:"space_key"`
	Title        string  `json:"title"`
	Excerpt      string  `json:"excerpt"`
	Similarity   float64 `json:"similarity"`
	FilePath     string  `json:"file_path,omitempty"`
	InternalID   string  `json:"internal_id,omitempty"`
	// BrowseURL is the page's path in the served catalog (REST only).
	BrowseURL string `json:"browse_url,omitempty"`
}

// PageDetail is the public full-page shape for MCP get_page.
type PageDetail struct {
	ConfluenceID int       `json:"confluence_id"`
	SpaceKey     string    `json:"space_key"`
	Title        string    `json:"title"`
	Version      int       `json:"version"`
	Content      string    `json:"content"`
	UpdatedAt    time.Time `json:"updated_at"`
	// BodyFormat is the crawl source format: "storage" (API) or "rendered" (browser HTML).
	BodyFormat string `json:"body_format,omitempty"`
	InternalID string `json:"internal_id,omitempty"`
}

// ToSearchHits maps DB rows to API-facing search hits.
func ToSearchHits(results []store.SearchResult, exposeInternalIDs bool) []SearchHit {
	if len(results) == 0 {
		return []SearchHit{}
	}
	hits := make([]SearchHit, len(results))
	for i, r := range results {
		hits[i] = SearchHit{
			ConfluenceID: r.ConfluenceID,
			SpaceKey:     r.SpaceKey,
			Title:        r.Title,
			Excerpt:      NormalizeExcerpt(r.Excerpt, DefaultExcerptMaxRunes),
			Similarity:   r.Similarity,
			FilePath:     r.FilePath,
		}
		if exposeInternalIDs && r.InternalID != uuid.Nil {
			hits[i].InternalID = r.InternalID.String()
		}
	}
	return hits
}

// ToPageDetail maps a DB page to the MCP/REST page detail shape.
func ToPageDetail(page *store.Page, spaceKey string, exposeInternalIDs bool) PageDetail {
	detail := PageDetail{
		ConfluenceID: page.ConfluenceID,
		SpaceKey:     spaceKey,
		Title:        page.Title,
		Version:      page.Version,
		Content:      page.Content,
		UpdatedAt:    page.UpdatedAt,
	}
	if exposeInternalIDs && page.ID != uuid.Nil {
		detail.InternalID = page.ID.String()
	}
	return detail
}
