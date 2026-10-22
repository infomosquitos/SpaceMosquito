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

// A page's file attachments are listed via the API, downloaded into
// assets/attachments (within the size limit), linked from content.md and
// recorded in metadata.json; images and draw.io sources are not files.
func TestCrawlPage_attachments(t *testing.T) {
	dir := t.TempDir()
	db, cfg := openTestDB(t, dir)
	cfg.Storage.MaxAttachmentMB = 1

	mux := http.NewServeMux()
	mux.HandleFunc("/wiki/rest/api/content/50", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "50", "title": "W10", "version": map[string]any{"number": 1},
			"space": map[string]any{"key": "PROJ"},
			"body": map[string]any{"storage": map[string]any{"value": `<p>Slides:</p>` +
				`<ac:structured-macro ac:name="view-file"><ac:parameter ac:name="name"><ri:attachment ri:filename="Workshop 10.pptx" /></ac:parameter></ac:structured-macro>` +
				`<ac:structured-macro ac:name="attachments"></ac:structured-macro>`}},
			"_links": map[string]any{"webui": "/spaces/PROJ/pages/50/W10"},
		})
	})
	mux.HandleFunc("/wiki/rest/api/content/50/child/attachment", func(w http.ResponseWriter, r *http.Request) {
		att := func(title, mt string, size int64) map[string]any {
			return map[string]any{"title": title, "extensions": map[string]any{"mediaType": mt, "fileSize": size},
				"_links": map[string]any{"download": "/download/attachments/50/" + strings.ReplaceAll(title, " ", "%20") + "?version=1&api=v2"}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{
			att("Workshop 10.pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation", 5),
			att("Huge.mp4", "video/mp4", 5<<20),
			att("shot.png", "image/png", 3),
			att("diagram", "application/vnd.jgraph.mxfile", 10),
		}})
	})
	mux.HandleFunc("/wiki/download/attachments/50/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte("PPTX!"))
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
	if err := s.CrawlPage("PROJ", 50, &session.Session{ConfluenceURL: srv.URL, Flavor: session.FlavorCloud}); err != nil {
		t.Fatalf("CrawlPage: %v", err)
	}

	got, err := db.GetPage(ctx, "PROJ", 50)
	if err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(got.FileDir, "assets", "attachments", "Workshop 10.pptx")); err != nil || string(b) != "PPTX!" {
		t.Fatalf("pptx not downloaded: %q %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(got.FileDir, "assets", "attachments", "Huge.mp4")); !os.IsNotExist(err) {
		t.Fatalf("file over the size limit downloaded (err=%v)", err)
	}
	md, _ := os.ReadFile(filepath.Join(got.FileDir, "content.md"))
	for _, want := range []string{
		`📎 [Workshop 10.pptx](<assets/attachments/Workshop%2010.pptx> "attachment")`,
		"- 📎 [Huge.mp4](<" + srv.URL + "/wiki/download/attachments/50/Huge.mp4?version=1&api=v2> \"attachment\") (5.0 MB)",
	} {
		if !strings.Contains(string(md), want) {
			t.Errorf("content.md lacks %q:\n%s", want, md)
		}
	}
	if strings.Contains(string(md), "shot.png") || strings.Contains(string(md), "diagram") {
		t.Errorf("image or draw.io source listed as a file:\n%s", md)
	}
	var meta storage.Metadata
	b, _ := os.ReadFile(filepath.Join(got.FileDir, "metadata.json"))
	if err := json.Unmarshal(b, &meta); err != nil {
		t.Fatal(err)
	}
	if len(meta.Attachments) != 2 || meta.Attachments[0].LocalPath != "assets/attachments/Workshop 10.pptx" ||
		meta.Attachments[0].Size != 5 || meta.Attachments[1].LocalPath != "" || meta.Attachments[1].Name != "Huge.mp4" {
		t.Fatalf("metadata attachments = %+v", meta.Attachments)
	}
}
