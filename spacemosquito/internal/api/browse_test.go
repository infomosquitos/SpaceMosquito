package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/vkh/spacemosquito/pkg/logging"
)

func TestSpaceBrowsePath(t *testing.T) {
	base := t.TempDir()
	if got := SpaceBrowsePath(base, "TST"); got != "" {
		t.Fatalf("no saved dir yet: got %q", got)
	}
	// A crawl creates saved/<KEY> when it starts; browse renders from the DB.
	if err := os.MkdirAll(filepath.Join(base, "TST"), 0o755); err != nil {
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
	// Search file_path (content.md) recorded on another machine: only the
	// page dir name is used.
	got := PageBrowsePath(base, "TST", "/old/home/saved/TST/10-Data mapping > X/content.md")
	if got != "/browse/TST/10-Data%20mapping%20%3E%20X/" {
		t.Fatalf("got %q", got)
	}
	if got := PageBrowsePath(base, "TST", "/x/TST/99-Missing/index.html"); got != "" {
		t.Fatalf("missing dir: got %q", got)
	}
}

type busyCrawls map[string]bool

func (b busyCrawls) HasActiveJob(key string) bool { return b[key] }

func TestSpaceDelete_guards(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/spaces/{key}", SpaceByIDHandler(nil, t.TempDir(), nil, busyCrawls{"BUSY": true}, logging.Sugar{}))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	for _, tc := range []struct {
		key  string
		code int
	}{
		{"BUSY", http.StatusConflict},     // crawl running
		{"%2E%2E", http.StatusBadRequest}, // ".."
		{"a.b", http.StatusBadRequest},
	} {
		req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/spaces/"+tc.key, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != tc.code {
			t.Errorf("%s: status %d, want %d", tc.key, resp.StatusCode, tc.code)
		}
	}
}
