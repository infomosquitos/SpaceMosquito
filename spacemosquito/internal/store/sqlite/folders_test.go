package sqlite_test

import (
	"testing"

	"github.com/vkh/spacemosquito/internal/store"
)

func TestFolders(t *testing.T) {
	db, ctx := openTestDB(t)
	defer db.Close()
	spaceID, err := db.CreateSpace(ctx, "TST", "Test", "https://example.atlassian.net/wiki/spaces/TST")
	if err != nil {
		t.Fatal(err)
	}
	parent := 10
	if err := db.UpsertFolder(ctx, &store.Folder{SpaceID: spaceID, ConfluenceID: 77, Title: "Sprint 1", ParentConfluenceID: &parent}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertFolder(ctx, &store.Folder{SpaceID: spaceID, ConfluenceID: 77, Title: "Sprint 1 (renamed)", ParentConfluenceID: &parent}); err != nil {
		t.Fatal(err)
	}
	got, err := db.ListFolders(ctx, "TST")
	if err != nil || len(got) != 1 || got[0].Title != "Sprint 1 (renamed)" || got[0].ParentConfluenceID == nil || *got[0].ParentConfluenceID != 10 {
		t.Fatalf("folders = %+v, err %v", got, err)
	}
	// Removing the space removes its folders.
	if err := db.DeleteSpace(ctx, "TST"); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.ListFolders(ctx, "TST"); len(got) != 0 {
		t.Fatalf("folders left after space delete: %+v", got)
	}
}
