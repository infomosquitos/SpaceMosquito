package scraper

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vkh/spacemosquito/internal/session"
)

func TestPageIDResolver_cached(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		q := r.URL.Query()
		if r.URL.Path != "/wiki/rest/api/content" || q.Get("spaceKey") != "SCC" || q.Get("type") != "page" {
			t.Errorf("unexpected request %s", r.URL)
		}
		if q.Get("title") == "Import flow" {
			_, _ = w.Write([]byte(`{"results":[{"id":"777","title":"Import flow"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()
	s := testScraper()
	resolve := s.pageIDResolver(srv.URL, "SCC", &session.Session{ConfluenceURL: srv.URL, Flavor: session.FlavorCloud})
	for i := 0; i < 3; i++ {
		if id := resolve("", "Import flow"); id != 777 {
			t.Fatalf("id = %d", id)
		}
	}
	if id := resolve("SCC", "Gone"); id != 0 {
		t.Fatalf("missing page id = %d", id)
	}
	resolve("SCC", "Gone")
	if hits != 2 {
		t.Fatalf("%d requests, want 2 (one per title, misses cached)", hits)
	}
}
