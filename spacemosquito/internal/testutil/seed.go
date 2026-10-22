package testutil

import (
	"context"

	"github.com/google/uuid"
	"github.com/vkh/spacemosquito/internal/store"
)

// SeedData holds stable fixture identifiers for integration assertions.
type SeedData struct {
	SpaceKey      string
	SpaceName     string
	SpaceURL      string
	SearchTerm    string
	SearchPageID  int
	PageIDs       []int
	PageTitles    map[int]string
	PageContents  map[int]string
}

// DefaultSeed returns the standard fixture metadata.
func DefaultSeed() *SeedData {
	return &SeedData{
		SpaceKey:     "TST",
		SpaceName:    "Test Space",
		SpaceURL:     "https://example.atlassian.net/wiki/spaces/TST",
		SearchTerm:   "mosquito",
		SearchPageID: 42,
		PageIDs:      []int{10, 20, 42},
		PageTitles: map[int]string{
			10: "Overview",
			20: "Architecture",
			42: "Mosquito Notes",
		},
		PageContents: map[int]string{
			10: "Welcome to the test space.",
			20: "System design overview.",
			42: "The space mosquito lives in integration tests.",
		},
	}
}

// SeedFixtures inserts a space and pages into the store.
func SeedFixtures(ctx context.Context, db store.Store) (*SeedData, error) {
	seed := DefaultSeed()
	spaceID, err := db.CreateSpace(ctx, seed.SpaceKey, seed.SpaceName, seed.SpaceURL)
	if err != nil {
		return nil, err
	}

	for _, id := range seed.PageIDs {
		page := &store.Page{
			SpaceID:      spaceID,
			ConfluenceID: id,
			Version:      1,
			Title:        seed.PageTitles[id],
			Content:      seed.PageContents[id],
			HTMLPath:     "saved/" + seed.SpaceKey + "/" + seed.PageTitles[id] + "/index.html",
			FileDir:      "saved/" + seed.SpaceKey + "/" + seed.PageTitles[id],
		}
		if err := db.UpsertPage(ctx, page); err != nil {
			return nil, err
		}
		_ = page.ID
	}

	if err := db.IndexAllPageContents(ctx); err != nil {
		return nil, err
	}

	return seed, nil
}

// SpaceID is a helper for tests that need the space UUID after seeding.
func SpaceID(ctx context.Context, db store.Store, spaceKey string) (uuid.UUID, error) {
	s, err := db.GetSpaceByKey(ctx, spaceKey)
	if err != nil {
		return uuid.Nil, err
	}
	return s.ID, nil
}
