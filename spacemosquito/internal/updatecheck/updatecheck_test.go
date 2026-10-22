package updatecheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewer(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"v0.8.2", "v0.8.1", true},
		{"v0.10.0", "v0.9.9", true},
		{"v1.0.0", "0.99.99", true},
		{"v0.8.1", "v0.8.1", false},
		{"v0.8.0", "v0.8.1", false},
		{"v0.9.0-rc1", "v0.8.1", false},
		{"v0.9.0", "dev", false},
	} {
		if got := Newer(tc.a, tc.b); got != tc.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestChecker_latestAndCache(t *testing.T) {
	t.Setenv(EnvDisable, "")
	hits := 0
	tag := "v0.9.0"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.Header.Get("User-Agent") != "spacemosquito/v0.8.1" {
			t.Errorf("User-Agent = %q", r.Header.Get("User-Agent"))
		}
		_, _ = w.Write([]byte(`{"tag_name":"` + tag + `","name":"x"}`))
	}))
	defer srv.Close()
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	c := &Checker{Current: "v0.8.1", CachePath: filepath.Join(t.TempDir(), CacheFile), APIURL: srv.URL, Now: func() time.Time { return now }}

	if latest, newer, err := c.Latest(context.Background()); err != nil || latest != "v0.9.0" || !newer {
		t.Fatalf("Latest = %q %v %v", latest, newer, err)
	}
	// Within a day: answered from the cache.
	tag = "v0.9.1"
	if latest, _, _ := c.Latest(context.Background()); latest != "v0.9.0" || hits != 1 {
		t.Fatalf("cached: %q after %d requests", latest, hits)
	}
	if latest, newer := c.Cached(); latest != "v0.9.0" || !newer {
		t.Fatalf("Cached = %q %v", latest, newer)
	}
	// After a day: asked again.
	now = now.Add(25 * time.Hour)
	if latest, _, _ := c.Latest(context.Background()); latest != "v0.9.1" || hits != 2 {
		t.Fatalf("refreshed: %q after %d requests", latest, hits)
	}
}

func TestChecker_disabledAndErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	}))
	defer srv.Close()
	cache := filepath.Join(t.TempDir(), CacheFile)

	if _, newer, err := (&Checker{Current: "dev", CachePath: cache, APIURL: srv.URL}).Latest(context.Background()); newer || err != nil {
		t.Fatalf("dev build: newer=%v err=%v", newer, err)
	}
	t.Setenv(EnvDisable, "1")
	if (&Checker{Current: "v0.8.1"}).Enabled() {
		t.Fatal("enabled despite " + EnvDisable)
	}
	t.Setenv(EnvDisable, "")
	c := &Checker{Current: "v0.8.1", CachePath: cache, APIURL: srv.URL}
	if _, newer, err := c.Latest(context.Background()); newer || err == nil {
		t.Fatalf("HTTP error: newer=%v err=%v", newer, err)
	}
	if _, err := os.Stat(cache); !os.IsNotExist(err) {
		t.Fatal("failed check was cached")
	}
}

func TestNotice(t *testing.T) {
	root := t.TempDir()
	cask := filepath.Join(root, "Caskroom", "spacemosquito", "0.8.1", "spacemosquito-darwin-arm64")
	link := filepath.Join(root, "bin", "spacemosquito")
	for _, d := range []string{filepath.Dir(cask), filepath.Dir(link)} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(cask, []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(cask, link); err != nil {
		t.Fatal(err)
	}

	brew := Notice("v0.8.1", "v0.9.0", link)
	for _, want := range []string{
		"A new SpaceMosquito version is available: v0.9.0 (you have v0.8.1).",
		"brew upgrade --cask spacemosquito && spacemosquito version",
		"restart `spacemosquito serve`",
		"https://github.com/infomosquitos/spacemosquito/releases/tag/v0.9.0",
	} {
		if !strings.Contains(brew, want) {
			t.Errorf("brew notice lacks %q:\n%s", want, brew)
		}
	}
	other := Notice("v0.8.1", "v0.9.0", "/usr/local/bin/spacemosquito")
	if strings.Contains(other, "brew") || !strings.Contains(other, "releases/latest") {
		t.Errorf("non-brew notice:\n%s", other)
	}
}
