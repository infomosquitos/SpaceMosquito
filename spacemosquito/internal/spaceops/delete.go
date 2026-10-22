// Package spaceops holds space-level operations shared by the REST API and
// the CLI.
package spaceops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/vkh/spacemosquito/internal/catalogindex"
	"github.com/vkh/spacemosquito/internal/store"
)

var (
	ErrInvalidKey = errors.New("invalid space key")
	ErrNotFound   = errors.New("space not found")
)

// spaceKeyPattern accepts Confluence space keys (letters, digits, "_", "-",
// optional leading "~" for personal spaces). It keeps keys that are used as
// directory names from reaching outside the saved catalog.
var spaceKeyPattern = regexp.MustCompile(`^~?[A-Za-z0-9_-]+$`)

// ValidKey reports whether key is a safe Confluence space key.
func ValidKey(key string) bool {
	return spaceKeyPattern.MatchString(key)
}

// CronConfigs removes a space's per-space cron schedule (cron.Manager).
type CronConfigs interface {
	Delete(spaceKey string) error
}

// DeleteOptions controls what Delete removes besides the catalog rows.
type DeleteOptions struct {
	// DeleteFiles also removes saved/<KEY> (pages, assets, indexes).
	DeleteFiles bool
}

// DeleteResult reports what Delete removed.
type DeleteResult struct {
	// Deleted is true once the space is removed from the catalog; an error
	// returned with Deleted set only reports leftovers (cron, files, index).
	Deleted      bool   `json:"deleted"`
	SpaceKey     string `json:"space_key"`
	Pages        int    `json:"pages_removed"`
	FilesDeleted bool   `json:"files_deleted"`
	FilesDir     string `json:"files_dir,omitempty"`
}

// Delete removes a space from the catalog: the space row, its pages and their
// search entries (DB cascade + FTS trigger), its cron schedule, and its entry
// in the root catalog index. Saved files are kept unless opts.DeleteFiles.
// cron may be nil. The caller ensures no crawl of the space is running.
func Delete(ctx context.Context, db store.Store, savedBase string, cron CronConfigs, key string, opts DeleteOptions) (DeleteResult, error) {
	res := DeleteResult{SpaceKey: key}
	if !ValidKey(key) {
		return res, ErrInvalidKey
	}
	sp, err := db.GetSpaceByKey(ctx, key)
	if err != nil || sp == nil {
		return res, ErrNotFound
	}
	if res.Pages, err = db.CountPagesBySpaceID(ctx, sp.ID); err != nil {
		return res, fmt.Errorf("count pages: %w", err)
	}
	if err := db.DeleteSpace(ctx, key); err != nil {
		return res, fmt.Errorf("delete space: %w", err)
	}
	res.Deleted = true

	// The space is gone from the catalog; failures below leave leftovers
	// that are reported but do not undo the delete.
	var errs []error
	if cron != nil {
		if err := cron.Delete(key); err != nil {
			errs = append(errs, fmt.Errorf("remove cron schedule: %w", err))
		}
	}
	if savedBase != "" {
		dir := filepath.Join(savedBase, key)
		res.FilesDir = dir
		if opts.DeleteFiles {
			if filepath.Dir(dir) != filepath.Clean(savedBase) {
				errs = append(errs, fmt.Errorf("refusing to delete %s: not directly under %s", dir, savedBase))
			} else if err := os.RemoveAll(dir); err != nil {
				errs = append(errs, fmt.Errorf("delete saved files: %w", err))
			} else {
				res.FilesDeleted = true
			}
		}
		if st, err := os.Stat(savedBase); err == nil && st.IsDir() {
			if err := catalogindex.WriteRoot(ctx, db, savedBase); err != nil {
				errs = append(errs, fmt.Errorf("update catalog index: %w", err))
			}
		}
	}
	return res, errors.Join(errs...)
}
