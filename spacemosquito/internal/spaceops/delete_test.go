package spaceops_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vkh/spacemosquito/internal/catalogindex"
	"github.com/vkh/spacemosquito/internal/config"
	"github.com/vkh/spacemosquito/internal/datastore"
	"github.com/vkh/spacemosquito/internal/spaceops"
	"github.com/vkh/spacemosquito/internal/store"
	"github.com/vkh/spacemosquito/pkg/logger"
	"github.com/vkh/spacemosquito/pkg/logging"
)

type fakeCron struct{ deleted []string }

func (f *fakeCron) Delete(key string) error {
	f.deleted = append(f.deleted, key)
	return nil
}

// setup seeds spaces TST (one searchable page, saved dir) and KEEP.
func setup(t *testing.T) (store.Store, string) {
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
	for _, key := range []string{"TST", "KEEP"} {
		id, err := db.CreateSpace(ctx, key, key, "https://x.atlassian.net/wiki/spaces/"+key)
		if err != nil {
			t.Fatal(err)
		}
		pageDir := filepath.Join(saved, key, "1-Page")
		if err := os.MkdirAll(pageDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := db.UpsertPage(ctx, &store.Page{SpaceID: id, ConfluenceID: len(key), Title: "Page", Content: "zanzibar notes", FileDir: pageDir}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.IndexAllPageContents(ctx); err != nil {
		t.Fatal(err)
	}
	if err := catalogindex.WriteAll(ctx, db, saved); err != nil {
		t.Fatal(err)
	}
	return db, saved
}

func TestDelete_keepsFilesByDefault(t *testing.T) {
	db, saved := setup(t)
	ctx := context.Background()
	cron := &fakeCron{}

	res, err := spaceops.Delete(ctx, db, saved, cron, "TST", spaceops.DeleteOptions{})
	if err != nil || !res.Deleted || res.Pages != 1 || res.FilesDeleted {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if _, err := db.GetSpaceByKey(ctx, "TST"); err == nil {
		t.Fatal("space still in DB")
	}
	hits, err := db.SearchPages(ctx, "zanzibar", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hits {
		if h.SpaceKey == "TST" {
			t.Fatalf("deleted space still searchable: %+v", h)
		}
	}
	if len(hits) != 1 {
		t.Fatalf("other space affected: %d hits", len(hits))
	}
	if len(cron.deleted) != 1 || cron.deleted[0] != "TST" {
		t.Fatalf("cron deletes = %v", cron.deleted)
	}
	if _, err := os.Stat(filepath.Join(saved, "TST", "1-Page")); err != nil {
		t.Fatalf("saved files should be kept: %v", err)
	}
	root, _ := os.ReadFile(filepath.Join(saved, "index.html"))
	if strings.Contains(string(root), "TST/index.html") || !strings.Contains(string(root), "KEEP/index.html") {
		t.Fatalf("root index not updated:\n%s", root)
	}
}

func TestDelete_deleteFiles(t *testing.T) {
	db, saved := setup(t)
	res, err := spaceops.Delete(context.Background(), db, saved, nil, "TST", spaceops.DeleteOptions{DeleteFiles: true})
	if err != nil || !res.FilesDeleted {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if _, err := os.Stat(filepath.Join(saved, "TST")); !os.IsNotExist(err) {
		t.Fatalf("saved/TST still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(saved, "KEEP", "1-Page")); err != nil {
		t.Fatalf("other space files touched: %v", err)
	}
}

func TestDelete_rejectsUnsafeAndUnknownKeys(t *testing.T) {
	db, saved := setup(t)
	for _, key := range []string{"..", "../KEEP", "TST/..", "", ".", `..\x`} {
		if _, err := spaceops.Delete(context.Background(), db, saved, nil, key, spaceops.DeleteOptions{DeleteFiles: true}); !errors.Is(err, spaceops.ErrInvalidKey) {
			t.Errorf("key %q: err=%v, want ErrInvalidKey", key, err)
		}
	}
	if _, err := spaceops.Delete(context.Background(), db, saved, nil, "NOPE", spaceops.DeleteOptions{}); !errors.Is(err, spaceops.ErrNotFound) {
		t.Errorf("unknown key: err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(saved, "KEEP", "1-Page")); err != nil {
		t.Fatalf("files touched by rejected deletes: %v", err)
	}
	if !spaceops.ValidKey("~557058abc") || !spaceops.ValidKey("SCCPI") {
		t.Fatal("valid keys rejected")
	}
}
