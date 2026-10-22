package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-rod/rod"
	"github.com/vkh/spacemosquito/internal/confluence"
	"github.com/vkh/spacemosquito/internal/ratelimit"
	"github.com/vkh/spacemosquito/internal/session"
	"path/filepath"
)

// SpaceInfo holds discovered metadata about a Confluence space.
type SpaceInfo struct {
	SpaceKey  string
	SpaceName string
	SpaceURL  string
	Pages     []*Page
	Folders   []Folder // Cloud folders holding pages
}

// discoverSpace attempts API discovery first, falls back to sidebar parsing if needed.
func (s *Scraper) discoverSpace(spaceURL string, sess *session.Session) (*SpaceInfo, error) {
	// Try API discovery first
	info, err := s.discoverSpaceAPI(spaceURL, sess)
	if err == nil && len(info.Pages) > 0 {
		if s.log.Enabled() {
			s.log.Infow("API discovery successful", "space_key", info.SpaceKey, "pages", len(info.Pages))
		}
		return info, nil
	}

	if s.log.Enabled() {
		s.log.Warnw("API space discovery failed, FALLING BACK to browser scraping",
			"space_url", spaceURL,
			"error", err)
	}

	if s.browser == nil {
		if err := s.LaunchBrowser(); err != nil {
			return nil, fmt.Errorf("failed to launch browser for fallback: %w", err)
		}
		if err := s.SetupContextWithSession(sess); err != nil {
			return nil, fmt.Errorf("failed to setup browser context: %w", err)
		}
	}

	return s.discoverSpaceWeb(spaceURL)
}

func (s *Scraper) discoverSpaceAPI(spaceURL string, sess *session.Session) (*SpaceInfo, error) {
	spaceKey := session.GetSpaceKeyFromURL(spaceURL)
	if spaceKey == "" {
		return nil, fmt.Errorf("could not extract space key from %s", spaceURL)
	}

	rootURL := extractConfluenceBaseURL(spaceURL)
	headers := sess.AsHeaders()

	pages, err := s.fetchPageListAPI(rootURL, spaceKey, headers, sess.Flavor)
	if err != nil {
		return nil, err
	}

	return &SpaceInfo{
		SpaceKey:  spaceKey,
		SpaceName: spaceKey, // API doesn't easily give space name without another call
		SpaceURL:  spaceURL,
		Pages:     pages,
		Folders:   s.fetchFolders(rootURL, pages, headers, sess.Flavor),
	}, nil
}

func (s *Scraper) fetchPageListAPI(rootURL, spaceKey string, headers map[string]string, flavor session.SessionFlavor) ([]*Page, error) {
	var allPages []*Page
	limit := 50
	start := 0

	for {
		var apiURL string
		if flavor == session.FlavorCloud {
			// Cloud V1 API
			apiURL = fmt.Sprintf("%s/wiki/rest/api/space/%s/content/page?limit=%d&start=%d&expand=version,ancestors,history", rootURL, spaceKey, limit, start)
		} else {
			// Server/DC API
			apiURL = fmt.Sprintf("%s/rest/api/content?spaceKey=%s&type=page&limit=%d&start=%d&expand=version,ancestors,history", rootURL, spaceKey, limit, start)
		}

		req, err := http.NewRequest("GET", apiURL, nil)
		if err != nil {
			return nil, err
		}

		for k, v := range headers {
			req.Header.Set(k, v)
		}

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := ratelimit.Do(client, req, s.logThrottle)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, session.UnauthorizedHTTP(resp.StatusCode)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("API request failed with status %d", resp.StatusCode)
		}

		var result struct {
			Results []struct {
				ID      string     `json:"id"`
				Title   string     `json:"title"`
				Version apiVersion `json:"version"`
				History apiHistory `json:"history"`
				// Ancestors (root first) give the parent at discovery time,
				// so the page tree holds even when a page later falls back to
				// the browser, which cannot read it.
				Ancestors []struct {
					ID string `json:"id"`
				} `json:"ancestors"`
				Links struct {
					Self  string `json:"self"`
					Webui string `json:"webui"`
				} `json:"_links"`
			} `json:"results"`
			Size int `json:"size"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, err
		}

		if len(result.Results) == 0 {
			break
		}

		for _, r := range result.Results {
			var id int
			fmt.Sscanf(r.ID, "%d", &id)

			url := confluence.WebUIURL(rootURL, r.Links.Webui, flavor == session.FlavorCloud)

			pg := &Page{
				ConfluenceID: id,
				Version:      r.Version.Number,
				Title:        r.Title,
				URL:          url,
				History:      pageHistory(r.Version, r.History),
			}
			if n := len(r.Ancestors); n > 0 {
				var parentID int
				if _, err := fmt.Sscanf(r.Ancestors[n-1].ID, "%d", &parentID); err == nil && parentID > 0 {
					pg.ParentID = &parentID
				}
			}
			allPages = append(allPages, pg)
		}

		if len(result.Results) < limit {
			break
		}
		start += len(result.Results)
	}

	return allPages, nil
}

// discoverSpaceWeb navigates to the space root and discovers all pages via sidebar parsing.
func (s *Scraper) discoverSpaceWeb(spaceURL string) (*SpaceInfo, error) {
	if s.log.Enabled() {
		s.log.Infow("navigating to space", "url", spaceURL)
	}

	page := s.browser.MustPage(spaceURL)
	page.Timeout(90 * time.Second)

	// Wait for the page to load with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		page.MustWaitLoad()
		close(done)
	}()

	select {
	case <-done:
		if s.log.Enabled() {
			s.log.Info("page loaded successfully")
		}
	case <-ctx.Done():
		if s.log.Enabled() {
			s.log.Errorw("page load timeout, taking screenshot for debug", "timeout", 60*time.Second)
		}
		// Take a screenshot for debugging
		page.MustScreenshot(filepath.Join(os.TempDir(), "confluence_timeout.png"))
		return nil, fmt.Errorf("page load timeout after 60s")
	}

	// Wait for dynamic content to render (Confluence is a SPA)
	// Use a timeout to prevent hanging
	stableCtx, stableCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer stableCancel()

	stableDone := make(chan struct{})
	go func() {
		page.MustWaitStable()
		close(stableDone)
	}()

	select {
	case <-stableDone:
		if s.log.Enabled() {
			s.log.Info("page is stable")
		}
	case <-stableCtx.Done():
		if s.log.Enabled() {
			s.log.Warn("page stable timeout, continuing anyway")
		}
	}

	// Wait for sidebar to fully render
	if s.log.Enabled() {
		s.log.Info("waiting for sidebar rendering")
	}
	time.Sleep(10 * time.Second)

	// Print current URL to verify we're on the right page
	currentURL := page.MustInfo().URL
	if s.log.Enabled() {
		s.log.Infow("current page URL", "url", currentURL)
	}

	// Capture the full page HTML
	html, err := page.HTML()
	if err != nil {
		return nil, fmt.Errorf("get space html: %w", err)
	}

	if s.log.Enabled() {
		s.log.Debugw("space root page captured", "html_size", len(html), "url", currentURL)
	}

	// Save HTML for debugging
	debugDir := s.cfg.Storage.BasePath
	if debugDir == "" {
		debugDir = os.TempDir()
	}
	debugPath := filepath.Join(debugDir, ".debug.html")
	if err := os.WriteFile(debugPath, []byte(html), 0644); err == nil {
		if s.log.Enabled() {
			s.log.Debugw("saved debug html", "path", debugPath, "size", len(html))
		}
	}
	timeoutDebug := filepath.Join(os.TempDir(), "confluence_debug.html")
	if err := os.WriteFile(timeoutDebug, []byte(html), 0644); err == nil {
		if s.log.Enabled() {
			s.log.Debugw("saved debug html", "path", timeoutDebug, "size", len(html))
		}
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("parse space html: %w", err)
	}

	spaceInfo := &SpaceInfo{
		SpaceURL: spaceURL,
	}

	spaceInfo.SpaceKey = s.extractSpaceKey(doc, spaceURL)
	spaceInfo.SpaceName = s.extractSpaceName(doc)

	if s.log.Enabled() {
		s.log.Infow("space info extracted",
			"space_key", spaceInfo.SpaceKey,
			"space_name", spaceInfo.SpaceName)
	}

	// Try rod's Element API first for dynamic content
	pages := s.findPagesByRod(page, spaceInfo.SpaceKey, spaceURL)
	if s.log.Enabled() && len(pages) == 0 {
		// Debug: dump all links with /pages/ in the page
		allPages, _ := page.Elements("a[href*='/pages/']")
		if s.log.Enabled() {
			s.log.Infow("debug: raw /pages/ links found", "count", len(allPages))
			for i, el := range allPages {
				if i < 5 {
					href, _ := el.Attribute("href")
					text := el.MustText()
					s.log.Debugw("debug: page link", "index", i, "href", href, "text", strings.TrimSpace(text))
				}
			}
		}
	}

	// Fall back to goquery if rod didn't find any
	if len(pages) == 0 {
		pages, err = s.parseSidebar(doc, spaceInfo.SpaceKey, 0)
		if err != nil {
			return nil, fmt.Errorf("parse sidebar: %w", err)
		}
	}

	seen := make(map[int]bool)
	var unique []*Page
	duplicates := 0
	for _, p := range pages {
		if seen[p.ConfluenceID] {
			duplicates++
			continue
		}
		seen[p.ConfluenceID] = true
		unique = append(unique, p)
	}
	spaceInfo.Pages = unique

	if s.log.Enabled() {
		s.log.Infow("page discovery complete",
			"space_key", spaceInfo.SpaceKey,
			"total_found", len(unique)+duplicates,
			"duplicates_skipped", duplicates,
			"unique_pages", len(unique))
	}

	return spaceInfo, nil
}

// findPagesByRod uses rod's Element API to find page links in the rendered DOM.
func (s *Scraper) findPagesByRod(page *rod.Page, spaceKey, spaceURL string) []*Page {
	// Normalize space URL to ensure trailing slash for URL joining
	if !strings.HasSuffix(spaceURL, "/") {
		spaceURL += "/"
	}
	var pages []*Page

	// Try to find all links with /pages/ in the href, filtered to current space
	if s.log.Enabled() {
		s.log.Infow("searching for page links with /pages/", "space_key", spaceKey)
	}

	elements, err := page.Elements("a[href*='/pages/']")
	if err != nil {
		if s.log.Enabled() {
			s.log.Infow("rod Elements failed", "error", err)
		}
		return nil
	}

	if s.log.Enabled() {
		s.log.Infow("found raw elements", "count", len(elements))
	}

	for _, el := range elements {
		href := el.MustAttribute("href")
		if href == nil || *href == "" || !strings.Contains(*href, "/pages/") {
			continue
		}

		// Only include pages from the current space
		if !strings.Contains(*href, "/spaces/"+spaceKey+"/") {
			continue
		}

		text := el.MustText()
		text = strings.TrimSpace(text)

		confluenceID := s.parseConfluenceID(*href)
		if confluenceID == 0 {
			continue
		}

		title := text
		if title == "" {
			title = fmt.Sprintf("page-%d", confluenceID)
		}

		pages = append(pages, &Page{
			ConfluenceID: confluenceID,
			Title:        title,
			URL:          s.resolveURL(*href, spaceURL),
			Level:        0,
		})
	}

	if s.log.Enabled() && len(pages) > 0 {
		s.log.Debugw("found pages via rod", "count", len(pages))
	}

	return pages
}

func (s *Scraper) extractSpaceKey(doc *goquery.Document, spaceURL string) string {
	// Try extracting from space key element
	doc.Find("#space-heading, .aui-navgroup-label, [data-testid=" +
		"sidebar.navigation.space-name]").Each(func(i int, s *goquery.Selection) {
		text := s.Text()
		if len(text) > 0 {
			// Space key is usually the first part of the display name or a separate element
		}
	})

	// Try extracting from URL pattern: /spaces/<KEY>/...
	re := regexp.MustCompile(`/spaces/([A-Z0-9]+(?:-[A-Z0-9]+)*)/?`)
	matches := re.FindStringSubmatch(spaceURL)
	if len(matches) >= 2 {
		return matches[1]
	}

	// Fallback: try data-space-key attribute
	doc.Find("[data-space-key]").Each(func(i int, s *goquery.Selection) {
		if key, exists := s.Attr("data-space-key"); exists && len(key) > 0 {
			// Use first found
		}
	})

	return "unknown"
}

func (s *Scraper) extractSpaceName(doc *goquery.Document) string {
	// Try various selectors for space name
	selectors := []string{
		"#space-heading .aui-navgroup-label",
		".toolbar-group .active-page a",
		"[data-testid=sidebar.navigation.space-name]",
		".aui-layout .aui-nav .active a",
	}

	for _, sel := range selectors {
		text := doc.Find(sel).Text()
		text = strings.TrimSpace(text)
		if len(text) > 0 {
			return text
		}
	}

	// Try the page title as space name fallback
	title := doc.Find("#title-text, #page-title, h1:first-of-type").Text()
	title = strings.TrimSpace(title)
	return title
}

func (s *Scraper) parseSidebar(doc *goquery.Document, spaceKey string, level int) ([]*Page, error) {
	var pages []*Page

	type linkInfo struct {
		href string
		text string
	}

	var allLinks []linkInfo

	// Collect all sidebar page links using multiple selectors
	sidebarSelectors := []string{
		"[data-testid='page-tree-item'] a[href*='/pages/']",
		"[data-testid*='sidebar'] a[href*='/pages/']",
		"[data-testid*='sidebar'] [data-testid*='tree'] a[href*='/pages/']",
		"[data-testid*='navigation'] a[href*='/pages/']",
		"[data-testid*='page-tree'] a[href*='/pages/']",
		"nav a[href*='/pages/']",
		"a[href*='/spaces/'][href*='/pages/']",
	}

	for _, sel := range sidebarSelectors {
		count := 0
		doc.Find(sel).Each(func(i int, s *goquery.Selection) {
			href, _ := s.Attr("href")
			text := strings.TrimSpace(s.Text())
			if len(href) > 0 && strings.Contains(href, "/pages/") {
				// Only include pages from the current space
				if !strings.Contains(href, "/spaces/"+spaceKey+"/") {
					return
				}
				allLinks = append(allLinks, linkInfo{href: href, text: text})
				count++
			}
		})
		if s.log.Enabled() && count > 0 {
			s.log.Debugw("found links with selector", "selector", sel, "count", count)
		}
	}

	// Deduplicate links by href
	seen := make(map[string]bool)
	for _, link := range allLinks {
		if seen[link.href] {
			continue
		}
		seen[link.href] = true

		// Parse confluence page ID from URL
		confluenceID := s.parseConfluenceID(link.href)
		title := link.text
		if title == "" {
			title = fmt.Sprintf("page-%d", confluenceID)
		}

		pages = append(pages, &Page{
			ConfluenceID: confluenceID,
			Title:        title,
			URL:          s.resolveURL(link.href, spaceKey),
			Level:        level,
		})
	}

	// Try to determine parent-child relationships from sidebar structure
	s.assignParentIDs(doc)

	return pages, nil
}

func (s *Scraper) parseConfluenceID(href string) int {
	// Confluence page URLs contain the page ID: /spaces/key/pages/NNNNNNN
	re := regexp.MustCompile(`/pages/(\d+)`)
	matches := re.FindStringSubmatch(href)
	if len(matches) >= 2 {
		var id int
		fmt.Sscanf(matches[1], "%d", &id)
		return id
	}
	return 0
}

func (s *Scraper) resolveURL(href, baseSpaceURL string) string {
	// Make sure URL is absolute
	if strings.HasPrefix(href, "http") {
		return href
	}

	// Build absolute URL from base space URL
	base := strings.TrimRight(baseSpaceURL, "/")
	return base + href
}

func (s *Scraper) assignParentIDs(doc *goquery.Document) {
	// Confluence sidebar typically uses nested <ul> elements for hierarchy
	// We attempt to infer parent-child from the DOM structure
	// This is a best-effort approach since Confluence uses dynamic rendering
}

// childPageAPI represents a child page from the Confluence REST API.
type childPageAPI struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	HREF  string `json:"_links.webui"`
}

// discoverChildPages uses a single page.Eval to extract all child page links
// from the sidebar DOM (links are present even when sidebar nodes are collapsed).
func (s *Scraper) discoverChildPages(page *rod.Page, parentID int, spaceKey, spaceURL string, discovered map[int]bool) []*Page {
	baseURL := extractConfluenceBaseURL(spaceURL)

	// Single eval to get all page links in the SAME space only
	// This avoids discovering pages from other spaces via the space switcher
	done := make(chan string, 1)
	go func() {
		results, err := page.Eval(`(key) => {
			var links = document.querySelectorAll('a[href*="/spaces/"][href*="/pages/"]');
			return JSON.stringify(Array.from(links)
				.filter(l => l.href.includes('/spaces/' + key + '/'))
				.map(l => ({href: l.href, text: l.textContent.trim()})));
		}`, spaceKey)
		if err == nil {
			done <- results.Value.String()
		} else {
			done <- ""
		}
	}()

	var linkJSON string
	select {
	case linkJSON = <-done:
		if linkJSON == "" {
			if s.log.Enabled() {
				s.log.Warnw("child page discovery eval failed", "parent_id", parentID)
			}
			return nil
		}
	case <-time.After(10 * time.Second):
		if s.log.Enabled() {
			s.log.Warnw("child page discovery timed out", "parent_id", parentID)
		}
		return nil
	}

	type linkInfo struct {
		Href string `json:"href"`
		Text string `json:"text"`
	}
	var links []linkInfo
	if err := json.Unmarshal([]byte(linkJSON), &links); err != nil {
		return nil
	}

	var children []*Page
	for _, link := range links {
		pageID := s.parseConfluenceID(link.Href)
		if pageID == 0 || pageID == parentID {
			continue
		}

		if discovered != nil && discovered[pageID] {
			continue
		}
		if discovered != nil {
			discovered[pageID] = true
		}

		title := strings.TrimSpace(link.Text)
		if title == "" {
			title = fmt.Sprintf("page-%d", pageID)
		}

		url := link.Href
		if !strings.HasPrefix(url, "http") {
			url = baseURL + "/wiki" + url
		}

		children = append(children, &Page{
			ConfluenceID: pageID,
			Title:        title,
			URL:          url,
			Level:        0,
		})
	}

	if s.log.Enabled() && len(children) > 0 {
		s.log.Infow("discovered child pages in sidebar", "parent_id", parentID, "children", len(children))
	}

	return children
}
