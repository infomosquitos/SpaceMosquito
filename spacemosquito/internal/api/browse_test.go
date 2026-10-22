package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrowseHandler(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "saved")
	page := filepath.Join(base, "TST", "10-Data mapping > X")
	if err := os.MkdirAll(filepath.Join(page, "assets", "images"), 0o755); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{
		filepath.Join(base, "index.html"):                   "catalog",
		filepath.Join(base, "TST", "index.html"):            "space tree",
		filepath.Join(page, "index.html"):                   "page",
		filepath.Join(page, "assets", "images", "shot.png"): "png",
		filepath.Join(root, "secret.txt"):                   "secret",
	} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	mux := http.NewServeMux()
	mux.Handle("GET "+BrowsePrefix, BrowseHandler(base))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	for _, tc := range []struct {
		path string
		code int
		body string
	}{
		{"/browse/", 200, "catalog"},
		{"/browse/TST/", 200, "space tree"},
		{"/browse/TST/index.html", 200, "space tree"}, // FileServer redirects to ./
		{"/browse/TST/10-Data%20mapping%20%3E%20X/", 200, "page"},
		{"/browse/TST/10-Data%20mapping%20%3E%20X/assets/images/shot.png", 200, "png"},
		{"/browse/TST/10-Data%20mapping%20%3E%20X/assets/", 404, ""}, // no listings
		{"/browse/../secret.txt", 404, ""},
		{"/browse/%2e%2e/secret.txt", 404, ""},
		{"/browse/NOPE/", 404, ""},
	} {
		resp, err := http.Get(srv.URL + tc.path)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != tc.code {
			t.Errorf("%s: status %d, want %d", tc.path, resp.StatusCode, tc.code)
			continue
		}
		if tc.body != "" && !strings.Contains(string(b), tc.body) {
			t.Errorf("%s: body %q, want %q", tc.path, b, tc.body)
		}
	}
}

func TestSpaceBrowsePath(t *testing.T) {
	base := t.TempDir()
	if got := SpaceBrowsePath(base, "TST"); got != "" {
		t.Fatalf("no index yet: got %q", got)
	}
	if err := os.MkdirAll(filepath.Join(base, "TST"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "TST", "index.html"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := SpaceBrowsePath(base, "TST"); got != "/browse/TST/" {
		t.Fatalf("got %q", got)
	}
}

func TestPageBrowsePath(t *testing.T) {
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "TST", "10-Data mapping > X"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Recorded path from another machine: only the page dir name is used.
	got := PageBrowsePath(base, "TST", "/old/home/saved/TST/10-Data mapping > X/index.html")
	if got != "/browse/TST/10-Data%20mapping%20%3E%20X/" {
		t.Fatalf("got %q", got)
	}
	if got := PageBrowsePath(base, "TST", "/x/TST/99-Missing/index.html"); got != "" {
		t.Fatalf("missing dir: got %q", got)
	}
}
