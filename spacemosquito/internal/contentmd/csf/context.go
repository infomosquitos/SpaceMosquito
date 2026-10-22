package csf

import "github.com/PuerkitoBio/goquery"

// AssetRequest is a media download scheduled by a rule. The converter returns
// the accumulated requests; the caller (scraper) performs the actual download
// via storage.AssetDownloader. Rules never download inline. Consumed starting
// in Phase 3.
type AssetRequest struct {
	Kind     string // "image" | "diagram"
	Filename string // local filename under the asset dir
	URL      string // absolute download URL
}

// FileLink is where a page attachment is linked from content.md: the local
// copy (assets/attachments/…) or, when not downloaded, Confluence.
type FileLink struct {
	Href string
	Size int64
}

// RenderContext carries per-conversion state and the recursion entry point.
type RenderContext struct {
	// PageID is the Confluence page ID, used to build attachment download URLs.
	PageID int
	// BaseURL is the Confluence site/API base (scheme://host[/context]), used
	// to build download URLs.
	BaseURL string
	// Cloud selects the attachment download URL shape: Cloud inserts a /wiki
	// path prefix, Server/DC does not.
	Cloud bool
	// Files are the page's file attachments (pptx, pdf, …) by Confluence
	// filename. File previews, attachment links and the attachments macro
	// link them; unknown files render as their name.
	Files map[string]FileLink

	registry *Registry
	assets   []AssetRequest

	// listDepth tracks nested ul/ol depth so the html rule can indent nested
	// list items. Mutated during the (single-threaded, depth-first) walk.
	listDepth int
}

// RequestAsset schedules an asset download. The converter returns all requests
// so the caller can fetch them out-of-band.
func (c *RenderContext) RequestAsset(r AssetRequest) {
	c.assets = append(c.assets, r)
}

// Assets returns the asset requests accumulated so far.
func (c *RenderContext) Assets() []AssetRequest {
	return c.assets
}

// RenderChildren renders the child nodes of sel through the registry and
// returns the assembled Markdown. Rules call this for nested content.
func (c *RenderContext) RenderChildren(sel *goquery.Selection) string {
	return renderChildren(sel, c)
}
