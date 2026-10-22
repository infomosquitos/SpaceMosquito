package catalogindex_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vkh/spacemosquito/internal/catalogindex"
	"github.com/vkh/spacemosquito/internal/config"
	"github.com/vkh/spacemosquito/internal/datastore"
	"github.com/vkh/spacemosquito/internal/store"
	"github.com/vkh/spacemosquito/pkg/logger"
	"github.com/vkh/spacemosquito/pkg/logging"
)

func intp(i int) *int { return &i }

func mkdirs(t *testing.T, base string, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := os.MkdirAll(filepath.Join(base, n), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBuildTree(t *testing.T) {
	spaceDir := t.TempDir()
	mkdirs(t, spaceDir, "1-Home", "2-beta", "3-Alpha", "4-Orphan", "5-Loop", "6-Loop")
	pages := []store.Page{
		{ConfluenceID: 1, Title: "Home", FileDir: "/elsewhere/TST/1-Home"},
		{ConfluenceID: 2, Title: "beta", ParentConfluenceID: intp(1), FileDir: "/x/2-beta"},
		{ConfluenceID: 3, Title: "Alpha", ParentConfluenceID: intp(1), FileDir: "/x/3-Alpha"},
		{ConfluenceID: 4, Title: "Orphan", ParentConfluenceID: intp(99), FileDir: "/x/4-Orphan"},
		{ConfluenceID: 5, Title: "Loop A", ParentConfluenceID: intp(6), FileDir: "/x/5-Loop"},
		{ConfluenceID: 6, Title: "Loop B", ParentConfluenceID: intp(5), FileDir: "/x/6-Loop"},
		{ConfluenceID: 7, Title: "Not saved", FileDir: "/x/7-Gone"},
	}
	roots, n := catalogindex.BuildTree(pages, spaceDir)
	if n != 6 {
		t.Fatalf("placed %d pages, want 6 (unsaved page skipped)", n)
	}
	var titles []string
	for _, r := range roots {
		titles = append(titles, r.Title)
	}
	// Orphan (parent not saved) and both cycle members surface as roots.
	if got := strings.Join(titles, ","); got != "Home,Loop A,Loop B,Orphan" {
		t.Fatalf("roots = %s", got)
	}
	home := roots[0]
	if len(home.Children) != 2 || home.Children[0].Title != "Alpha" || home.Children[1].Title != "beta" {
		t.Fatalf("children not sorted case-insensitively: %+v", home.Children)
	}
	if home.Dir != "1-Home" {
		t.Fatalf("dir = %q, want base name", home.Dir)
	}
}

// openDB returns a migrated test database whose saved catalog is saved.
func openDB(t *testing.T) (store.Store, string) {
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
	return db, saved
}

func TestWriteSpace(t *testing.T) {
	db, saved := openDB(t)
	ctx := context.Background()
	spaceID, err := db.CreateSpace(ctx, "TST", "Test Space", "https://example.atlassian.net/wiki/spaces/TST")
	if err != nil {
		t.Fatal(err)
	}
	spaceDir := filepath.Join(saved, "TST")
	child := "11-Data mapping iFlow > [Commerce]"
	mkdirs(t, spaceDir, "10-Home", child)
	for _, p := range []store.Page{
		{SpaceID: spaceID, ConfluenceID: 10, Title: "Home", BodyFormat: "storage", FileDir: filepath.Join(spaceDir, "10-Home")},
		{SpaceID: spaceID, ConfluenceID: 11, Title: "Data mapping iFlow > [Commerce]", ParentConfluenceID: intp(10), FileDir: filepath.Join(spaceDir, child)},
	} {
		if err := db.UpsertPage(ctx, &p); err != nil {
			t.Fatal(err)
		}
	}

	if err := catalogindex.WriteSpace(ctx, db, saved, "TST"); err != nil {
		t.Fatalf("WriteSpace: %v", err)
	}

	read := func(path string) string {
		t.Helper()
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	spaceMD := read(filepath.Join(spaceDir, "index.md"))
	for _, want := range []string{
		"# Test Space (TST)",
		"2 pages",
		"- [Home](10-Home/content.md)\n  - [Data mapping iFlow > \\[Commerce\\]](11-Data%20mapping%20iFlow%20%3E%20%5BCommerce%5D/content.md)",
	} {
		if !strings.Contains(spaceMD, want) {
			t.Errorf("space index.md missing %q:\n%s", want, spaceMD)
		}
	}
	if rootMD := read(filepath.Join(saved, "index.md")); !strings.Contains(rootMD, "- [Test Space (TST)](TST/index.md) — 2 pages") {
		t.Errorf("root index.md:\n%s", rootMD)
	}
	// The browse UI renders HTML on request (ADR-018); nothing is generated.
	for _, f := range []string{filepath.Join(saved, "index.html"), filepath.Join(spaceDir, "index.html"), filepath.Join(spaceDir, "10-Home", "index.html")} {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("%s written (err=%v)", f, err)
		}
	}

	sp, err := catalogindex.LoadSpace(ctx, db, saved, "TST")
	if err != nil {
		t.Fatal(err)
	}
	if sp.Heading() != "Test Space (TST)" || sp.Summary() != "2 pages" || len(sp.Roots) != 1 || len(sp.Roots[0].Children) != 1 {
		t.Fatalf("LoadSpace = %q %q roots=%d", sp.Heading(), sp.Summary(), len(sp.Roots))
	}
	if _, err := catalogindex.LoadSpace(ctx, db, saved, "NOPE"); err == nil {
		t.Fatal("LoadSpace of an unknown space succeeded")
	}
}

func TestRemoveLegacyHTML(t *testing.T) {
	saved := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(saved, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("index.html", "catalog")
	write("index.md", "catalog md")
	write("TST/index.html", "tree")
	write("TST/tree.js", "js")
	write("TST/1-Storage/metadata.json", `{"body_format":"storage"}`)
	write("TST/1-Storage/index.html", "generated")
	write("TST/1-Storage/content.md", "md")
	write("TST/2-Rendered/metadata.json", `{"body_format":"rendered"}`)
	write("TST/2-Rendered/index.html", "source of content.md")

	n, err := catalogindex.RemoveLegacyHTML(saved)
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Fatalf("removed %d files, want 4", n)
	}
	for rel, want := range map[string]bool{
		"index.html": false, "index.md": true, "TST/index.html": false, "TST/tree.js": false,
		"TST/1-Storage/index.html": false, "TST/1-Storage/content.md": true,
		"TST/2-Rendered/index.html": true,
	} {
		_, err := os.Stat(filepath.Join(saved, rel))
		if got := err == nil; got != want {
			t.Errorf("%s exists=%v, want %v", rel, got, want)
		}
	}
}
