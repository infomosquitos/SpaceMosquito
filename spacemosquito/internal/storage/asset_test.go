package storage

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vkh/spacemosquito/pkg/logging"
)

func testDownloader(client *http.Client) *AssetDownloader {
	return &AssetDownloader{
		client:     client,
		maxRetries: 1,
		retryDelay: time.Millisecond,
		rateLimit:  0,
	}
}

func TestAssetDownloader_RewriteURL(t *testing.T) {
	d := NewAssetDownloader(logging.Sugar{})

	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "attachment path",
			url:  "https://example.atlassian.net/wiki/download/attachments/123/file.pdf",
			want: "attachments/",
		},
		{
			name: "confluence-attachments host",
			url:  "https://confluence-attachments.example.com/path/image",
			want: "images/",
		},
		{
			name: "invalid url returns original",
			url:  "://bad",
			want: "://bad",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := d.RewriteURL(tc.url, "assets")
			if tc.name == "invalid url returns original" {
				if got != tc.url {
					t.Errorf("got %q, want original %q", got, tc.url)
				}
				return
			}
			if !strings.Contains(got, tc.want) {
				t.Errorf("RewriteURL(%q) = %q, want substring %q", tc.url, got, tc.want)
			}
		})
	}
}

func TestAssetDownloader_Download_success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("png-bytes"))
	}))
	defer srv.Close()

	d := testDownloader(srv.Client())
	dest := t.TempDir()

	path, err := d.Download(dest, srv.URL+"/file")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("downloaded file missing: %v", err)
	}
}

func TestAssetDownloader_Download_skipsExisting(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Write([]byte("cached"))
	}))
	defer srv.Close()

	d := testDownloader(srv.Client())
	dest := t.TempDir()
	url := srv.URL + "/asset.bin"

	path1, err := d.Download(dest, url)
	if err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Fatalf("hits after first download = %d, want 1", hits)
	}

	path2, err := d.Download(dest, url)
	if err != nil {
		t.Fatal(err)
	}
	if path1 != path2 {
		t.Errorf("paths differ: %q vs %q", path1, path2)
	}
	if hits != 1 {
		t.Errorf("second call should not hit server; hits = %d", hits)
	}
}

func TestAssetDownloader_Download_retriesOnFailure(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	defer srv.Close()

	d := testDownloader(srv.Client())
	_, err := d.Download(t.TempDir(), srv.URL)
	if err == nil {
		t.Fatal("expected error after retries")
	}
	if attempts < 2 {
		t.Errorf("attempts = %d, want at least 2", attempts)
	}
}

func TestAssetDownloader_Download_rejectsHTMLLoginPage(t *testing.T) {
	// SSO-protected instances answer an unauthenticated attachment request with
	// a 200 HTML login/redirect page; that must be a loud failure, not a saved
	// "image".
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><head><meta http-equiv="refresh" content="1;URL='/login'"/></head></html>`))
	}))
	defer srv.Close()

	d := testDownloader(srv.Client())
	dest := t.TempDir()
	if _, err := d.Download(dest, srv.URL+"/img.png"); err == nil {
		t.Fatal("expected error for HTML login page, got nil")
	}
	// No file should have been written.
	entries, _ := os.ReadDir(dest)
	for _, e := range entries {
		t.Fatalf("unexpected file written: %s", e.Name())
	}
}

func TestAssetDownloader_DownloadAs_rejectsHTMLLoginPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html>login</html>`))
	}))
	defer srv.Close()

	d := testDownloader(srv.Client())
	dest := t.TempDir() + "/assets/images/pic.png"
	if err := d.DownloadAs(dest, srv.URL+"/pic.png"); err == nil {
		t.Fatal("expected error for HTML login page")
	}
	if _, err := os.Stat(dest); err == nil {
		t.Fatal("file should not have been written")
	}
}

func TestAssetDownloader_sendsAuthHeaders(t *testing.T) {
	var gotCookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCookie = r.Header.Get("Cookie")
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("png"))
	}))
	defer srv.Close()

	d := testDownloader(srv.Client())
	d.SetAuthHeaders(map[string]string{"Cookie": "JSESSIONID=abc123"})
	if _, err := d.Download(t.TempDir(), srv.URL+"/i.png"); err != nil {
		t.Fatal(err)
	}
	if gotCookie != "JSESSIONID=abc123" {
		t.Fatalf("auth header not sent, got Cookie=%q", gotCookie)
	}
}

func TestAssetDownloader_Download_contentTypeExtension(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte("jpeg"))
	}))
	defer srv.Close()

	d := testDownloader(srv.Client())
	path, err := d.Download(t.TempDir(), srv.URL+"/noext")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, ".jpg") {
		t.Errorf("path %q should have .jpg extension", path)
	}
}

func TestAssetDownloader_DownloadAs_skipsExisting(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Write([]byte("new-bytes"))
	}))
	defer srv.Close()

	d := testDownloader(srv.Client())
	dest := filepath.Join(t.TempDir(), "assets", "images", "pic.png")
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("cached"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := d.DownloadAs(dest, srv.URL+"/pic.png"); err != nil {
		t.Fatal(err)
	}
	if hits != 0 {
		t.Errorf("expected no HTTP request, hits=%d", hits)
	}
	got, _ := os.ReadFile(dest)
	if string(got) != "cached" {
		t.Errorf("file was overwritten: %q", got)
	}
}

func TestAssetDownloader_DownloadAs_redownloadsZeroByte(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Write([]byte("fresh"))
	}))
	defer srv.Close()

	d := testDownloader(srv.Client())
	dest := filepath.Join(t.TempDir(), "empty.png")
	if err := os.WriteFile(dest, nil, 0644); err != nil {
		t.Fatal(err)
	}

	if err := d.DownloadAs(dest, srv.URL+"/empty.png"); err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Errorf("hits=%d, want 1", hits)
	}
	got, _ := os.ReadFile(dest)
	if string(got) != "fresh" {
		t.Errorf("got %q, want fresh", got)
	}
}

func TestAssetDownloader_DownloadAs_forceOverwrites(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Write([]byte("forced"))
	}))
	defer srv.Close()

	d := testDownloader(srv.Client())
	d.SetForce(true)
	dest := filepath.Join(t.TempDir(), "pic.png")
	if err := os.WriteFile(dest, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := d.DownloadAs(dest, srv.URL+"/pic.png"); err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Errorf("hits=%d, want 1", hits)
	}
	got, _ := os.ReadFile(dest)
	if string(got) != "forced" {
		t.Errorf("got %q, want forced", got)
	}
}

func TestAssetDownloader_Download_skipsExtensionlessViaGlob(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte("jpeg"))
	}))
	defer srv.Close()

	d := testDownloader(srv.Client())
	dest := t.TempDir()
	rawURL := srv.URL + "/noext"
	hash := sha256.Sum256([]byte(rawURL))
	existing := filepath.Join(dest, fmt.Sprintf("%x.jpg", hash[:8]))
	if err := os.WriteFile(existing, []byte("cached"), 0644); err != nil {
		t.Fatal(err)
	}

	path, err := d.Download(dest, rawURL)
	if err != nil {
		t.Fatal(err)
	}
	if path != existing {
		t.Errorf("path=%q, want %q", path, existing)
	}
	if hits != 0 {
		t.Errorf("expected no HTTP request, hits=%d", hits)
	}
}

func TestAssetDownloader_Download_queryStringExt(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Write([]byte("png"))
	}))
	defer srv.Close()

	d := testDownloader(srv.Client())
	dest := t.TempDir()
	rawURL := srv.URL + "/img.png?width=100"
	hash := sha256.Sum256([]byte(rawURL))
	existing := filepath.Join(dest, fmt.Sprintf("%x.png", hash[:8]))
	if err := os.WriteFile(existing, []byte("cached"), 0644); err != nil {
		t.Fatal(err)
	}

	path, err := d.Download(dest, rawURL)
	if err != nil {
		t.Fatal(err)
	}
	if path != existing {
		t.Errorf("path=%q, want %q", path, existing)
	}
	if hits != 0 {
		t.Errorf("expected skip via path ext, hits=%d", hits)
	}
}

func TestURLPathExt(t *testing.T) {
	if got := urlPathExt("https://x/a/b.png?w=1"); got != ".png" {
		t.Errorf("got %q, want .png", got)
	}
	if got := urlPathExt("https://x/a/noext"); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestAssetDownloader_DownloadFile(t *testing.T) {
	body := "v1-data"
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path == "/login" {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html>login</html>"))
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	d := NewAssetDownloader(logging.Sugar{})
	d.SetClientForTest(srv.Client())
	dest := filepath.Join(t.TempDir(), "assets", "attachments", "Deck 1.pptx")

	if err := d.DownloadFile(dest, srv.URL+"/f", int64(len(body))); err != nil {
		t.Fatal(err)
	}
	// Same size: kept without a request.
	if err := d.DownloadFile(dest, srv.URL+"/f", int64(len(body))); err != nil || hits != 1 {
		t.Fatalf("unchanged file re-downloaded: hits=%d err=%v", hits, err)
	}
	// New version (size changed): replaced.
	body = "version-2-data"
	if err := d.DownloadFile(dest, srv.URL+"/f", int64(len(body))); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dest); string(b) != body || hits != 2 {
		t.Fatalf("got %q after %d hits", b, hits)
	}
	if _, err := os.Stat(dest + ".part"); !os.IsNotExist(err) {
		t.Fatal("partial file left behind")
	}
	// A login page instead of the file is an error, not a saved file.
	login := filepath.Join(filepath.Dir(dest), "Sheet.xlsx")
	if err := d.DownloadFile(login, srv.URL+"/login", 10); err == nil {
		t.Fatal("login page accepted as attachment")
	}
	if _, err := os.Stat(login); !os.IsNotExist(err) {
		t.Fatal("login page saved")
	}
}
