package scraper

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vkh/spacemosquito/internal/session"
)

func TestFetchFolders(t *testing.T) {
	asked := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked[r.URL.Path]++
		switch r.URL.Path {
		case "/wiki/api/v2/folders/100": // folder in a folder
			_, _ = w.Write([]byte(`{"id":"100","title":"Sprint 142","parentId":"200","parentType":"folder"}`))
		case "/wiki/api/v2/folders/200": // folder under page 1
			_, _ = w.Write([]byte(`{"id":"200","title":"Sprints","parentId":"1","parentType":"page"}`))
		default: // 999: a page in another space, not a folder
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	p := func(i int) *int { return &i }
	pages := []*Page{
		{ConfluenceID: 1},
		{ConfluenceID: 2, ParentID: p(100)},
		{ConfluenceID: 3, ParentID: p(100)},
		{ConfluenceID: 4, ParentID: p(1)},
		{ConfluenceID: 5, ParentID: p(999)},
	}
	got := testScraper().fetchFolders(srv.URL, pages, nil, session.FlavorCloud)
	if len(got) != 2 || got[0].Title != "Sprint 142" || *got[0].ParentID != 200 || got[1].Title != "Sprints" || *got[1].ParentID != 1 {
		t.Fatalf("folders = %+v", got)
	}
	if asked["/wiki/api/v2/folders/100"] != 1 || asked["/wiki/api/v2/folders/1"] != 0 {
		t.Fatalf("requests: %v (each folder once, never for known pages)", asked)
	}
	if got := testScraper().fetchFolders(srv.URL, pages, nil, session.FlavorServer); got != nil {
		t.Fatalf("Server/DC has no folders: %+v", got)
	}
}
