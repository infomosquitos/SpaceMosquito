package browse

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vkh/spacemosquito/internal/config"
	"github.com/vkh/spacemosquito/internal/datastore"
	"github.com/vkh/spacemosquito/internal/store"
	"github.com/vkh/spacemosquito/pkg/logger"
	"github.com/vkh/spacemosquito/pkg/logging"
)

const childDir = "11-Child > X"

// newSite saves space TST: an empty parent page (10-Home) with one child
// (11-Child > X, with an image, raw.html and an attachment) and a
// browser-fallback page (12-Rendered).
func newSite(t *testing.T) *Site {
	t.Helper()
	dir := t.TempDir()
	saved := filepath.Join(dir, "saved")
	cfg := &config.Config{
		Database: config.DatabaseConfig{Driver: "sqlite", Path: filepath.Join(dir, "test.db")},
		Storage:  config.StorageConfig{BasePath: saved},
	}
	log, err := logger.NewProduction(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := datastore.MigrateUp(cfg, filepath.Join("..", "..", "migrations"), logging.New("test", log)); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db, err := datastore.Open(cfg, log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	ctx := context.Background()
	spaceID, err := db.CreateSpace(ctx, "TST", "Test Space", "https://example.atlassian.net/wiki/spaces/TST")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"10-Home/metadata.json": `{"title":"Home","body_format":"storage"}`,
		childDir + "/metadata.json": `{"title":"Child > X","confluence_url":"https://example.atlassian.net/wiki/spaces/TST/pages/11","body_format":"storage",` +
			`"attachments":[{"original_url":"https://example.atlassian.net/wiki/download/attachments/11/Deck%201.pptx","local_path":"assets/attachments/Deck 1.pptx","name":"Deck 1.pptx","size":2500000},` +
			`{"original_url":"https://example.atlassian.net/wiki/download/attachments/11/Spec.pdf","local_path":"assets/attachments/Spec.pdf","name":"Spec.pdf","size":900},` +
			`{"original_url":"https://example.atlassian.net/wiki/download/attachments/11/Huge.mp4","local_path":"","name":"Huge.mp4","size":5368709120}]}`,
		childDir + "/content.md":                     "Intro <script>alert(1)</script>\n\n![shot.png](assets/images/shot.png)\n\n📎 [Spec.pdf](<assets/attachments/Spec.pdf> \"attachment\")\n",
		childDir + "/assets/attachments/Deck 1.pptx": "PPTX",
		childDir + "/assets/attachments/Spec.pdf":    "%PDF-1.4",
		childDir + "/raw.html":                       `<p>Intro</p><script>alert(1)</script>`,
		childDir + "/assets/images/shot.png":         "png",
		childDir + "/assets/attachments/a.html":      "<script>fetch('/api/spaces/TST',{method:'DELETE'})</script>",
		"12-Rendered/metadata.json":                  `{"title":"Rendered","body_format":"rendered"}`,
		"12-Rendered/index.html":                     "<p>extracted</p>",
		"12-Rendered/content.md":                     "Rendered body",
	}
	for rel, body := range files {
		p := filepath.Join(saved, "TST", rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	parent := 10
	created := time.Date(2022, 3, 12, 12, 0, 0, 0, time.UTC)
	updated := time.Date(2024, 10, 3, 12, 0, 0, 0, time.UTC)
	for _, p := range []store.Page{
		{SpaceID: spaceID, ConfluenceID: 10, Title: "Home", BodyFormat: "storage", FileDir: filepath.Join(saved, "TST", "10-Home")},
		{SpaceID: spaceID, ConfluenceID: 11, Title: "Child > X", Content: "Intro", ParentConfluenceID: &parent, BodyFormat: "storage", FileDir: filepath.Join(saved, "TST", childDir),
			History: store.PageHistory{CreatedBy: "Ann <A>", CreatedAt: &created, UpdatedBy: "Bob", UpdatedAt: &updated}},
		{SpaceID: spaceID, ConfluenceID: 12, Title: "Rendered", Content: "Rendered body", BodyFormat: "rendered", FileDir: filepath.Join(saved, "TST", "12-Rendered")},
	} {
		if err := db.UpsertPage(ctx, &p); err != nil {
			t.Fatal(err)
		}
	}
	return &Site{DB: db, SavedBase: saved, Log: logging.New("test", log)}
}

func get(t *testing.T, s *Site, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.URL.Path = path // as decoded by the server, incl. ".." segments
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func TestHandler_views(t *testing.T) {
	s := newSite(t)
	for _, tc := range []struct {
		path string
		want []string
		not  []string
	}{
		{"/browse/", []string{
			`<a href="TST/">Test Space (TST)</a>`, "3 pages", `class="sm-fullsearch" data-space=""`,
			`href=".static/browse.css?v=`,
		}, nil},
		{"/browse/TST/", []string{
			`<a href="../">← All spaces</a>`, `<h1>Test Space (TST)</h1>`, "3 pages",
			`<details open><summary><a href="10-Home/">Home</a> <span class="sm-count">(1)</span>`,
			`<a href="11-Child%20%3E%20X/">Child &gt; X</a>`, `class="sm-fullsearch" data-space="TST"`,
			"Expand all", `href="../.static/browse.css?v=`,
		}, nil},
		{"/browse/TST/" + childDir + "/", []string{
			`<title>Child &gt; X</title>`, `<h1>Child &gt; X</h1>`,
			`<a href="https://example.atlassian.net/wiki/spaces/TST/pages/11">View in Confluence</a>`,
			`<img src="assets/images/shot.png" alt="shot.png">`,
			// Sidebar: path to the current page open, current highlighted.
			`<details open><summary><a href="../10-Home/">Home</a>`,
			`<a href="../11-Child%20%3E%20X/" class="sm-current" aria-current="page">Child &gt; X</a>`,
			`class="sm-quick" data-space="TST" data-all="../"`, "3 pages",
			`<p class="sm-meta sm-byline">Created by Ann &lt;A&gt; on 12 Mar 2022 · Last updated by Bob on 3 Oct 2024</p>`,
			// File preview (inline PDF via browse.js) and the Attachments section.
			`<a href="assets/attachments/Spec.pdf" title="attachment">Spec.pdf</a>`,
			`<h2>Attachments (3)</h2>`,
			`<a href="assets/attachments/Deck%201.pptx" download>Deck 1.pptx</a> <span class="sm-meta">2.4 MB</span>`,
			`<a href="assets/attachments/Spec.pdf">Spec.pdf</a> <span class="sm-meta">900 B</span>`,
			`<a href="https://example.atlassian.net/wiki/download/attachments/11/Huge.mp4">Huge.mp4</a> <span class="sm-meta">5.0 GB · not saved locally, opens in Confluence</span>`,
			`src="../../.static/browse.js?v=`,
		}, []string{"<script>alert"}},
		{"/browse/TST/10-Home/", []string{
			"This page has no content of its own. Child pages:",
			`<li><a href="../11-Child%20%3E%20X/">Child &gt; X</a></li>`,
		}, nil},
		{"/browse/TST/12-Rendered/", []string{"<p>Rendered body</p>", "<h1>Rendered</h1>"}, nil},
	} {
		rec := get(t, s, tc.path)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status %d", tc.path, rec.Code)
			continue
		}
		if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
			t.Errorf("%s: content type %q", tc.path, ct)
		}
		body := rec.Body.String()
		for _, w := range tc.want {
			if !strings.Contains(body, w) {
				t.Errorf("%s: missing %q in:\n%s", tc.path, w, body)
			}
		}
		for _, n := range tc.not {
			if strings.Contains(body, n) {
				t.Errorf("%s: unexpected %q", tc.path, n)
			}
		}
	}
}

func TestHandler_filesAndErrors(t *testing.T) {
	s := newSite(t)
	page := "/browse/TST/" + childDir + "/"
	for _, tc := range []struct {
		path     string
		code     int
		location string
		ctype    string
		body     string
	}{
		{"/browse/TST", http.StatusMovedPermanently, "/browse/TST/", "", ""},
		{"/browse/TST/index.html", http.StatusMovedPermanently, "/browse/TST/", "", ""},
		{"/browse/TST/10-Home", http.StatusMovedPermanently, "/browse/TST/10-Home/", "", ""},
		{"/browse/TST/10-Home/index.html", http.StatusMovedPermanently, "/browse/TST/10-Home/", "", ""},
		{page + "index.html", http.StatusMovedPermanently, "/browse/TST/11-Child%20%3E%20X/", "", ""},
		{"/browse/TST/" + childDir, http.StatusMovedPermanently, "/browse/TST/11-Child%20%3E%20X/", "", ""},
		{page + "assets/images/shot.png", http.StatusOK, "", "image/png", "png"},
		{page + "content.md", http.StatusOK, "", "text/markdown; charset=utf-8", "Intro"},
		{page + "raw.html", http.StatusOK, "", "text/plain; charset=utf-8", "<script>"},
		{page + "metadata.json", http.StatusNotFound, "", "", ""},
		{page + "assets/images", http.StatusNotFound, "", "", ""}, // no listings
		{page + "assets/../../12-Rendered/index.html", http.StatusNotFound, "", "", ""},
		{"/browse/../secret.txt", http.StatusNotFound, "", "", ""},
		{"/browse/TST/../../secret.txt", http.StatusNotFound, "", "", ""},
		{"/browse/TST/99-Missing/", http.StatusNotFound, "", "", ""},
		{"/browse/NOPE/", http.StatusNotFound, "", "", ""},
		{"/browse/a.b/", http.StatusNotFound, "", "", ""},
		{"/browse/.static/browse.css", http.StatusOK, "", "text/css; charset=utf-8", "sm-tree"},
		{"/browse/.static/browse.js", http.StatusOK, "", "text/javascript; charset=utf-8", "sm-quick"},
		{"/browse/.static/nope.js", http.StatusNotFound, "", "", ""},
	} {
		rec := get(t, s, tc.path)
		if rec.Code != tc.code {
			t.Errorf("%s: status %d, want %d", tc.path, rec.Code, tc.code)
			continue
		}
		if tc.location != "" && rec.Header().Get("Location") != tc.location {
			t.Errorf("%s: location %q, want %q", tc.path, rec.Header().Get("Location"), tc.location)
		}
		if tc.ctype != "" && rec.Header().Get("Content-Type") != tc.ctype {
			t.Errorf("%s: content type %q, want %q", tc.path, rec.Header().Get("Content-Type"), tc.ctype)
		}
		if tc.body != "" && !strings.Contains(rec.Body.String(), tc.body) {
			t.Errorf("%s: body %q lacks %q", tc.path, rec.Body.String(), tc.body)
		}
	}
	// Office files download; PDFs open in the browser's viewer (not sandboxed).
	if rec := get(t, s, page+"assets/attachments/Deck 1.pptx"); rec.Code != http.StatusOK ||
		rec.Header().Get("Content-Disposition") != "attachment; filename*=UTF-8''Deck%201.pptx" || rec.Header().Get("Content-Security-Policy") != "sandbox" {
		t.Errorf("pptx: status %d disposition %q CSP %q", rec.Code, rec.Header().Get("Content-Disposition"), rec.Header().Get("Content-Security-Policy"))
	}
	if rec := get(t, s, page+"assets/attachments/Spec.pdf"); rec.Code != http.StatusOK ||
		rec.Header().Get("Content-Disposition") != "" || rec.Header().Get("Content-Security-Policy") != "" || rec.Header().Get("Content-Type") != "application/pdf" {
		t.Errorf("pdf: status %d disposition %q CSP %q type %q", rec.Code, rec.Header().Get("Content-Disposition"), rec.Header().Get("Content-Security-Policy"), rec.Header().Get("Content-Type"))
	}
	// Saved files cannot run script against the local API.
	rec := get(t, s, page+"assets/attachments/a.html")
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Security-Policy") != "sandbox" {
		t.Fatalf("attachment: status %d CSP %q", rec.Code, rec.Header().Get("Content-Security-Policy"))
	}
}

func TestExport(t *testing.T) {
	s := newSite(t)
	out := filepath.Join(t.TempDir(), "export")
	res, err := s.Export(context.Background(), out, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Spaces != 1 || res.Pages != 3 {
		t.Fatalf("exported %+v, want 1 space, 3 pages", res)
	}
	read := func(rel string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(out, rel))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	if root := read("index.html"); !strings.Contains(root, `<a href="TST/index.html">`) || strings.Contains(root, "sm-fullsearch") {
		t.Errorf("root index.html:\n%s", root)
	}
	if sp := read("TST/index.html"); !strings.Contains(sp, `<a href="10-Home/index.html">Home</a>`) {
		t.Errorf("space index.html:\n%s", sp)
	}
	page := read(filepath.Join("TST", childDir, "index.html"))
	for _, want := range []string{`href="../10-Home/index.html"`, `href="../../.static/browse.css?v=`, `<img src="assets/images/shot.png"`} {
		if !strings.Contains(page, want) {
			t.Errorf("page missing %q:\n%s", want, page)
		}
	}
	if strings.Contains(page, "sm-quick") {
		t.Error("static page has content search (needs the server)")
	}
	for _, rel := range []string{".static/browse.js", filepath.Join("TST", childDir, "assets", "images", "shot.png")} {
		if _, err := os.Stat(filepath.Join(out, rel)); err != nil {
			t.Errorf("%s not exported: %v", rel, err)
		}
	}

	if _, err := s.Export(context.Background(), out, ""); err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Errorf("export into non-empty dir: err=%v", err)
	}
	if _, err := s.Export(context.Background(), filepath.Join(s.SavedBase, "x"), ""); err == nil || !strings.Contains(err.Error(), "inside the saved catalog") {
		t.Errorf("export inside saved: err=%v", err)
	}
	if _, err := s.Export(context.Background(), filepath.Join(t.TempDir(), "one"), "NOPE"); err == nil {
		t.Error("export of an unknown space succeeded")
	}
}
