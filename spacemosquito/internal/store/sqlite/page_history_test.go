package sqlite_test

import (
	"testing"
	"time"

	"github.com/vkh/spacemosquito/internal/store"
)

func TestPageHistory(t *testing.T) {
	db, ctx := openTestDB(t)
	defer db.Close()
	spaceID, err := db.CreateSpace(ctx, "TST", "Test", "https://example.atlassian.net/wiki/spaces/TST")
	if err != nil {
		t.Fatal(err)
	}
	created := time.Date(2022, 3, 12, 9, 0, 0, 0, time.UTC)
	updated := time.Date(2024, 10, 3, 14, 30, 0, 0, time.UTC)
	page := &store.Page{SpaceID: spaceID, ConfluenceID: 7, Title: "Decision log", Content: "decision words",
		History: store.PageHistory{CreatedBy: "Ann", CreatedAt: &created, UpdatedBy: "Bob", UpdatedAt: &updated}}
	if err := db.UpsertPage(ctx, page); err != nil {
		t.Fatal(err)
	}
	check := func(want store.PageHistory) {
		t.Helper()
		got, err := db.GetPage(ctx, "TST", 7)
		if err != nil {
			t.Fatal(err)
		}
		h := got.History
		if h.CreatedBy != want.CreatedBy || h.UpdatedBy != want.UpdatedBy ||
			h.CreatedAt == nil || !h.CreatedAt.Equal(*want.CreatedAt) ||
			h.UpdatedAt == nil || !h.UpdatedAt.Equal(*want.UpdatedAt) {
			t.Fatalf("history = %+v, want %+v", h, want)
		}
	}
	check(page.History)

	// A save without history (browser fallback) keeps what is stored.
	page.History = store.PageHistory{}
	if err := db.UpsertPage(ctx, page); err != nil {
		t.Fatal(err)
	}
	check(store.PageHistory{CreatedBy: "Ann", CreatedAt: &created, UpdatedBy: "Bob", UpdatedAt: &updated})

	// A crawl refreshes the last update of an unchanged page.
	later := updated.Add(48 * time.Hour)
	if err := db.UpdatePageHistory(ctx, "TST", 7, store.PageHistory{UpdatedBy: "Cy", UpdatedAt: &later}); err != nil {
		t.Fatal(err)
	}
	check(store.PageHistory{CreatedBy: "Ann", CreatedAt: &created, UpdatedBy: "Cy", UpdatedAt: &later})
	if err := db.UpdatePageHistory(ctx, "TST", 999, store.PageHistory{UpdatedBy: "Nobody"}); err != nil {
		t.Fatalf("unsaved page: %v", err)
	}

	hits, err := db.SearchPages(ctx, "decision", "TST", 5)
	if err != nil || len(hits) != 1 {
		t.Fatalf("search: %v %d hits", err, len(hits))
	}
	if hits[0].UpdatedBy != "Cy" || hits[0].UpdatedAt == nil || !hits[0].UpdatedAt.Equal(later) {
		t.Fatalf("search hit history = %q %v", hits[0].UpdatedBy, hits[0].UpdatedAt)
	}
}
