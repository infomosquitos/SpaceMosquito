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

		md, skip, err := RenderDirMarkdown(dir)
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

// resolveDir returns the saved page directory, trying the recorded FileDir
// first and then remapping under savedBase for catalogs moved between machines.
func resolveDir(page store.Page, savedBase string) string {
	candidates := []string{}
	if page.FileDir != "" {
		candidates = append(candidates, page.FileDir)
	}
	if page.HTMLPath != "" {
		candidates = append(candidates, filepath.Dir(page.HTMLPath))
	}
	if savedBase != "" && page.FileDir != "" {
		candidates = append(candidates, filepath.Join(savedBase, filepath.Base(filepath.Dir(page.FileDir)), filepath.Base(page.FileDir)))
	}

	seen := map[string]bool{}
	for _, c := range candidates {
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c
		}
	}
	return ""
}
