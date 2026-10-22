package scraper

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/vkh/spacemosquito/internal/ratelimit"
	"github.com/vkh/spacemosquito/internal/session"
	"github.com/vkh/spacemosquito/internal/store"
)

// Folder is a Confluence Cloud folder found during discovery: pages inside
// it have it as their parent, but the page listing does not return it.
type Folder struct {
	ConfluenceID int
	Title        string
	ParentID     *int // page or folder
}

// maxFolders bounds folder lookups per discovery.
const maxFolders = 1000

// fetchFolders looks up the parents of discovered pages that are not pages
// themselves (Cloud folders), and their parent folders in turn. Unknown
// parents (other spaces, no access, Server/DC without folders) are skipped.
func (s *Scraper) fetchFolders(rootURL string, pages []*Page, headers map[string]string, flavor session.SessionFlavor) []Folder {
	if flavor != session.FlavorCloud {
		return nil
	}
	known := map[int]bool{}
	for _, pg := range pages {
		known[pg.ConfluenceID] = true
	}
	var queue []int
	for _, pg := range pages {
		if pg.ParentID != nil && !known[*pg.ParentID] {
			known[*pg.ParentID] = true
			queue = append(queue, *pg.ParentID)
		}
	}
	client := &http.Client{Timeout: 30 * time.Second}
	var folders []Folder
	for len(queue) > 0 && len(folders) < maxFolders {
		id := queue[0]
		queue = queue[1:]
		f, err := fetchFolder(client, rootURL, id, headers, s.logThrottle)
		if err != nil {
			s.log.Warnw("look up parent folder failed", "id", id, "error", err)
			continue
		}
		if f == nil {
			continue // not a folder
		}
		folders = append(folders, *f)
		if f.ParentID != nil && !known[*f.ParentID] {
			known[*f.ParentID] = true
			queue = append(queue, *f.ParentID)
		}
	}
	return folders
}

// fetchFolder reads a folder through the v2 API; nil, nil when id is not a
// folder.
func fetchFolder(client *http.Client, rootURL string, id int, headers map[string]string, onThrottle func(time.Duration)) (*Folder, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/wiki/api/v2/folders/%d", rootURL, id), nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := ratelimit.Do(client, req, onThrottle)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, nil
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var v struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		ParentID string `json:"parentId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, err
	}
	f := &Folder{ConfluenceID: id, Title: v.Title}
	if p, err := strconv.Atoi(v.ParentID); err == nil && p > 0 {
		f.ParentID = &p
	}
	return f, nil
}

// saveFolders stores a space's folders so the page tree can show them.
func (s *Scraper) saveFolders(spaceKey string, folders []Folder) {
	if len(folders) == 0 {
		return
	}
	sp, err := s.db.GetSpaceByKey(s.ctx, spaceKey)
	if err != nil || sp == nil {
		return
	}
	for _, f := range folders {
		if err := s.db.UpsertFolder(s.ctx, &store.Folder{SpaceID: sp.ID, ConfluenceID: f.ConfluenceID, Title: f.Title, ParentConfluenceID: f.ParentID}); err != nil {
			s.log.Warnw("save folder failed", "id", f.ConfluenceID, "error", err)
		}
	}
}
