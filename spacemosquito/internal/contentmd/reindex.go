package contentmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vkh/spacemosquito/internal/store"
)

// ReindexResult summarizes a content re-extraction run.
type ReindexResult struct {
	Updated int
	Skipped int
	Errors  []string
}

// ReindexAll regenerates content.md and pages.content from saved page files.
func ReindexAll(ctx context.Context, db store.Store, savedBase string) (ReindexResult, error) {
	pages, err := db.ListAllPages(ctx)
	if err != nil {
		return ReindexResult{}, err
	}

	// Display names for @-mentions, collected by crawls.
	names, err := db.UserNames(ctx)
	if err != nil {
		return ReindexResult{}, err
	}
	userName := func(id string) string { return names[id] }

	var result ReindexResult
	for _, page := range pages {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		dir := resolveDir(page, savedBase)
		if dir == "" {
			result.Skipped++
			continue
		}

		md, skip, err := RenderDirMarkdownWithUsers(dir, userName)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", dir, err))
			result.Skipped++
			continue
		}
		if strings.TrimSpace(md) == "" {
			result.Skipped++
			if skip != "" {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %s", dir, skip))
			}
			continue
		}

		if err := WriteFile(dir, md); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: write content.md: %v", dir, err))
		}

		page.Content = md
		if err := db.UpsertPage(ctx, &page); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("confluence_id=%d: %v", page.ConfluenceID, err))
			result.Skipped++
			continue
		}
		result.Updated++
	}
	return result, nil
}

// resolveDir returns the saved page directory. With a savedBase, only
// directories inside it are used: the recorded FileDir may point at another
// catalog (one moved, or the original of a copy), which must not be
// modified. The page is looked up as savedBase/<space>/<page dir>.
func resolveDir(page store.Page, savedBase string) string {
	recorded := []string{page.FileDir}
	if page.HTMLPath != "" {
		recorded = append(recorded, filepath.Dir(page.HTMLPath))
	}
	for _, rec := range recorded {
		if rec == "" {
			continue
		}
		c := rec
		if savedBase != "" {
			c = filepath.Join(savedBase, filepath.Base(filepath.Dir(rec)), filepath.Base(rec))
		}
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c
		}
	}
	return ""
}
