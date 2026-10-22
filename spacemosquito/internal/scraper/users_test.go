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

	"github.com/vkh/spacemosquito/internal/session"
	"github.com/vkh/spacemosquito/internal/storage"
	"github.com/vkh/spacemosquito/pkg/logging"
)

// Cloud @-mentions get display names: page authors are known from page
// history, the rest are looked up in one bulk request and cached.
func TestCrawlPage_mentionNames(t *testing.T) {
	dir := t.TempDir()
	db, cfg := openTestDB(t, dir)
	var bulk []string
	mux := http.NewServeMux()
	mux.HandleFunc("/wiki/rest/api/content/60", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "60", "title": "MoM", "space": map[string]any{"key": "PROJ"},
			"version": map[string]any{"number": 1, "by": map[string]any{"accountId": "712020:bob", "displayName": "Bob"}},
			"body": map[string]any{"storage": map[string]any{"value": `<h2>Participants</h2><ul>` +
				`<li><p><ac:link><ri:user ri:account-id="712020:ann" /></ac:link></p></li>` +
				`<li><p><ac:link><ri:user ri:account-id="712020:bob" /></ac:link></p></li>` +
				`<li><p><ac:link><ri:user ri:account-id="712020:gone" /></ac:link></p></li>` +
				`<li><p><ac:link><ri:user ri:userkey="712020:43929ea0-bde0-4d06-96a9-bb3ede102937" /></ac:link></p></li></ul>`}},
			"_links": map[string]any{"webui": "/spaces/PROJ/pages/60/MoM"},
		})
	})
	mux.HandleFunc("/wiki/rest/api/content/60/child/attachment", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[]}`))
	})
	mux.HandleFunc("/wiki/rest/api/user/bulk", func(w http.ResponseWriter, r *http.Request) {
		bulk = append(bulk, r.URL.Query()["accountId"]...)
		_, _ = w.Write([]byte(`{"results":[{"accountId":"712020:ann","displayName":"Ann Larsen"},{"accountId":"712020:43929ea0-bde0-4d06-96a9-bb3ede102937","displayName":"Evariz P (Deactivated)"}]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx := context.Background()
	if _, err := db.CreateSpace(ctx, "PROJ", "Proj", srv.URL+"/wiki/spaces/PROJ"); err != nil {
		t.Fatal(err)
	}
	assets := storage.NewAssetDownloader(logging.Sugar{})
	assets.SetClientForTest(srv.Client())
	s := New(cfg, db, storage.NewWriter(cfg.Storage.BasePath, logging.Sugar{}), assets, logging.Sugar{})
	if err := s.CrawlPage("PROJ", 60, &session.Session{ConfluenceURL: srv.URL, Flavor: session.FlavorCloud}); err != nil {
		t.Fatalf("CrawlPage: %v", err)
	}
	got, err := db.GetPage(ctx, "PROJ", 60)
	if err != nil {
		t.Fatal(err)
	}
	md, _ := os.ReadFile(filepath.Join(got.FileDir, "content.md"))
	if want := "- @Ann Larsen\n- @Bob\n- @unknown user\n- @Evariz P (Deactivated)"; !strings.Contains(string(md), want) {
		t.Fatalf("content.md lacks %q:\n%s", want, md)
	}
	// Bob came from page history; only the unknown accounts were looked up.
	if strings.Join(bulk, ",") != "712020:ann,712020:gone,712020:43929ea0-bde0-4d06-96a9-bb3ede102937" {
		t.Fatalf("bulk lookup asked for %v", bulk)
	}
	names, _ := db.UserNames(ctx)
	if names["712020:ann"] != "Ann Larsen" || names["712020:bob"] != "Bob" {
		t.Fatalf("stored names = %v", names)
	}
}
