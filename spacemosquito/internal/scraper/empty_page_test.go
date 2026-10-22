package scraper

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vkh/spacemosquito/internal/session"
	"github.com/vkh/spacemosquito/internal/storage"
	"github.com/vkh/spacemosquito/pkg/logging"
)

// Parent pages that only group children have an empty storage body; they must
// be saved as (empty) Storage Format pages, not sent to the browser fallback.
func TestCrawlPage_emptyBodyParentPage(t *testing.T) {
	dir := t.TempDir()
	db, cfg := openTestDB(t, dir)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/wiki/rest/api/content/43") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "43",
			"title":   "1. TEAM AGREEMENTS",
			"version": map[string]any{"number": 2, "when": "2024-10-03T14:30:00.000Z", "by": map[string]any{"displayName": "Bob"}},
			"history": map[string]any{"createdDate": "2022-03-12T09:00:00.000+01:00", "createdBy": map[string]any{"displayName": "Ann"}},
			"space":   map[string]any{"key": "PROJ"},
			"body":    map[string]any{"storage": map[string]any{"value": ""}},
			// Cloud webui is relative to /wiki.
			"_links": map[string]any{"webui": "/spaces/PROJ/pages/43/1.+TEAM+AGREEMENTS"},
		})
	}))
	defer srv.Close()

	ctx := context.Background()
	if _, err := db.CreateSpace(ctx, "PROJ", "Proj", srv.URL+"/wiki/spaces/PROJ"); err != nil {
		t.Fatal(err)
	}
	s := New(cfg, db, storage.NewWriter(cfg.Storage.BasePath, logging.Sugar{}), storage.NewAssetDownloader(logging.Sugar{}), logging.Sugar{})
	sess := &session.Session{ConfluenceURL: srv.URL, Flavor: session.FlavorCloud}

	if err := s.CrawlPage("PROJ", 43, sess); err != nil {
		t.Fatalf("CrawlPage: %v", err)
	}

	got, err := db.GetPage(ctx, "PROJ", 43)
	if err != nil {
		t.Fatal(err)
	}
	if got.BodyFormat != "storage" || got.Content != "" {
		t.Fatalf("body_format=%q content=%q, want storage + empty", got.BodyFormat, got.Content)
	}
	h := got.History
	if h.CreatedBy != "Ann" || h.CreatedAt == nil || !h.CreatedAt.Equal(time.Date(2022, 3, 12, 8, 0, 0, 0, time.UTC)) ||
		h.UpdatedBy != "Bob" || h.UpdatedAt == nil || !h.UpdatedAt.Equal(time.Date(2024, 10, 3, 14, 30, 0, 0, time.UTC)) {
		t.Fatalf("history = %+v", h)
	}
	meta, err := os.ReadFile(filepath.Join(got.FileDir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	if want := srv.URL + "/wiki/spaces/PROJ/pages/43/"; !strings.Contains(string(meta), want) {
		t.Fatalf("confluence_url lacks /wiki: %s", meta)
	}
	for _, want := range []string{`"author": "Ann"`, `"created_at": "2022-03-12T08:00:00Z"`, `"updated_by": "Bob"`, `"updated_at": "2024-10-03T14:30:00Z"`} {
		if !strings.Contains(string(meta), want) {
			t.Fatalf("metadata.json lacks %s: %s", want, meta)
		}
	}
	// Storage Format pages are rendered by the browse UI from content.md
	// (ADR-018); no index.html is written.
	if _, err := os.Stat(filepath.Join(got.FileDir, "index.html")); !os.IsNotExist(err) {
		t.Fatalf("index.html written for a storage page (err=%v)", err)
	}
	if got.HTMLPath != "" {
		t.Fatalf("html_path = %q, want empty for a storage page", got.HTMLPath)
	}
}

func TestIsErrorPage(t *testing.T) {
	jira404 := `<html lang="en"><head><meta charset="utf-8"/><title>Oops, you&#39;ve found a dead link. - JIRA</title></head><body class=" error-page error404 js-enabled"><h1>Oops, you&#39;ve found a dead link.</h1></body></html>`
	if !isErrorPage(jira404) {
		t.Fatal("Jira dead-link page not detected")
	}
	if isErrorPage(`<html><head><title>Error handling guide</title></head><body class="wiki-content"><p>How we handle a 404 error page</p></body></html>`) {
		t.Fatal("normal page about errors flagged as error page")
	}
}
