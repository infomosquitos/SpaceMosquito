package scraper

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/vkh/spacemosquito/internal/session"
	"github.com/vkh/spacemosquito/pkg/logging"
)

func testScraper() *Scraper {
	return New(nil, nil, nil, nil, logging.Sugar{})
}

func loadFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestParseVersionResponse(t *testing.T) {
	jsonData := loadFixture(t, "confluence_page_list.json")

	var result struct {
		Results []struct {
			ID      string `json:"id"`
			Title   string `json:"title"`
			Version struct {
				Number int `json:"number"`
			} `json:"version"`
		} `json:"results"`
	}

	if err := json.Unmarshal([]byte(jsonData), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(result.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result.Results))
	}
	if result.Results[0].Version.Number != 11 {
		t.Errorf("first version = %d, want 11", result.Results[0].Version.Number)
	}
}

func TestParseConfluenceID(t *testing.T) {
	s := testScraper()
	tests := []struct {
		href string
		want int
	}{
		{"/wiki/spaces/PROJ/pages/12345/Page", 12345},
		{"/pages/99999/foo", 99999},
		{"/no-page-id", 0},
		{"", 0},
	}
	for _, tc := range tests {
		if got := s.parseConfluenceID(tc.href); got != tc.want {
			t.Errorf("parseConfluenceID(%q) = %d, want %d", tc.href, got, tc.want)
		}
	}
}

func TestResolveURL(t *testing.T) {
	s := testScraper()
	base := "https://example.atlassian.net/wiki/spaces/PROJ"

	if got := s.resolveURL("https://other.example/pages/1", base); got != "https://other.example/pages/1" {
		t.Errorf("absolute href changed: %q", got)
	}
	if got := s.resolveURL("/wiki/spaces/PROJ/pages/1", base); got != base+"/wiki/spaces/PROJ/pages/1" {
		t.Errorf("relative href = %q", got)
	}
}

func TestExtractConfluenceBaseURL(t *testing.T) {
	got := extractConfluenceBaseURL("https://company.atlassian.net/wiki/spaces/PROJ")
	if got != "https://company.atlassian.net" {
		t.Errorf("got %q", got)
	}
	got = extractConfluenceBaseURL("https://wiki.example.com/confluence/display/KEY")
	if got != "https://wiki.example.com/confluence" {
		t.Errorf("context path: got %q", got)
	}
	if extractConfluenceBaseURL("") != "" {
		t.Error("empty url should return empty")
	}
}

func TestExtractSpaceKey_fromURL(t *testing.T) {
	s := testScraper()
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader("<html></html>"))
	got := s.extractSpaceKey(doc, "https://example.atlassian.net/wiki/spaces/MY-SPACE/overview")
	if got != "MY-SPACE" {
		t.Errorf("got %q, want MY-SPACE", got)
	}
}

func TestParseSidebar(t *testing.T) {
	s := testScraper()
	html := loadFixture(t, "sidebar.html")
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}

	pages, err := s.parseSidebar(doc, "PROJ", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 {
		t.Fatalf("expected 2 unique PROJ pages, got %d", len(pages))
	}
	if pages[0].Title != "First Page" {
		t.Errorf("title = %q", pages[0].Title)
	}
	if pages[1].Title != "page-101" {
		t.Errorf("empty link title = %q, want page-101", pages[1].Title)
	}
}

func TestFetchPageListAPI_cloud(t *testing.T) {
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/wiki/rest/api/space/DEMO/content/page") {
			http.NotFound(w, r)
			return
		}
		if !strings.Contains(r.URL.RawQuery, "expand=version,ancestors") {
			t.Error("expected expand=version,ancestors in query")
		}
		page++
		if page == 1 {
			w.Write([]byte(loadFixture(t, "confluence_page_list.json")))
			return
		}
		w.Write([]byte(`{"results":[],"size":0}`))
	}))
	defer srv.Close()

	s := testScraper()
	pages, err := s.fetchPageListAPI(srv.URL, "DEMO", map[string]string{}, session.FlavorCloud)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 {
		t.Fatalf("got %d pages, want 2", len(pages))
	}
	if pages[0].Version != 11 {
		t.Errorf("version = %d, want 11", pages[0].Version)
	}
	if want := srv.URL + "/wiki/spaces/DEMO/pages/100000001/Getting+Started"; pages[0].URL != want {
		t.Errorf("URL = %q, want %q (Cloud webui is relative to /wiki)", pages[0].URL, want)
	}
	if pages[0].ParentID != nil {
		t.Errorf("root page parent = %d, want none", *pages[0].ParentID)
	}
	if pages[1].ParentID == nil || *pages[1].ParentID != 100000001 {
		t.Errorf("parent = %v, want nearest ancestor 100000001", pages[1].ParentID)
	}
}

func TestFetchPageListAPI_server(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/rest/api/content") {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("spaceKey") != "DEMO" {
			t.Errorf("spaceKey = %q", r.URL.Query().Get("spaceKey"))
		}
		w.Write([]byte(loadFixture(t, "confluence_page_list.json")))
	}))
	defer srv.Close()

	s := testScraper()
	pages, err := s.fetchPageListAPI(srv.URL, "DEMO", map[string]string{}, session.FlavorServer)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 {
		t.Fatalf("got %d pages", len(pages))
	}
}

func TestFetchPageListAPI_errors(t *testing.T) {
	s := testScraper()

	t.Run("non-200", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "nope", http.StatusForbidden)
		}))
		defer srv.Close()
		_, err := s.fetchPageListAPI(srv.URL, "DEMO", nil, session.FlavorCloud)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("malformed json", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("not json"))
		}))
		defer srv.Close()
		_, err := s.fetchPageListAPI(srv.URL, "DEMO", nil, session.FlavorCloud)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty results", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"results":[],"size":0}`))
		}))
		defer srv.Close()
		pages, err := s.fetchPageListAPI(srv.URL, "DEMO", nil, session.FlavorCloud)
		if err != nil {
			t.Fatal(err)
		}
		if len(pages) != 0 {
			t.Fatalf("got %d pages", len(pages))
		}
	})
}
