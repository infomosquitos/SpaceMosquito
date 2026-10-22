package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/vkh/spacemosquito/internal/session"
	"github.com/vkh/spacemosquito/internal/spaceops"
	"github.com/vkh/spacemosquito/internal/store"
	"github.com/vkh/spacemosquito/pkg/logging"
)

type spacesHandler struct {
	log       logging.Sugar
	db        store.Store
	savedBase string
}

type addSpaceRequest struct {
	URL string `json:"url"`
}

// SpacesHandler handles space management endpoints. savedBase locates the
// catalog so the list can report each space's browse_path.
func SpacesHandler(db store.Store, savedBase string, log logging.Sugar) http.HandlerFunc {
	return (&spacesHandler{log: log, db: db, savedBase: savedBase}).handle
}

func (h *spacesHandler) handle(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		h.list(w, r)
	case "POST":
		h.add(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (h *spacesHandler) list(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	spaces, err := h.db.ListSpaces(ctx)
	if err != nil {
		h.log.Errorw("list spaces failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list spaces"})
		return
	}

	result := make([]map[string]interface{}, 0, len(spaces))
	for _, s := range spaces {
		var lastCrawledStr string
		if s.LastCrawled != nil {
			lastCrawledStr = s.LastCrawled.Format(time.RFC3339)
		}
		pageCount, err := h.db.CountPagesBySpaceID(ctx, s.ID)
		if err != nil {
			h.log.Warnw("count pages failed", "space_key", s.Key, "error", err)
			pageCount = 0
		}
		formats, err := h.db.CountPagesByBodyFormat(ctx, s.ID)
		if err != nil {
			h.log.Warnw("count body formats failed", "space_key", s.Key, "error", err)
		}

		result = append(result, map[string]interface{}{
			"space_key":      s.Key,
			"space_name":     s.Name,
			"space_url":      s.URL,
			"pages_crawled":  pageCount,
			"pages_total":    s.PagesTotal,
			"pages_storage":  formats.Storage,
			"pages_rendered": formats.Rendered,
			"last_crawled":   lastCrawledStr,
			"created_at":     s.CreatedAt.Format(time.RFC3339),
			"browse_path":    SpaceBrowsePath(h.savedBase, s.Key),
		})
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *spacesHandler) add(w http.ResponseWriter, r *http.Request) {
	var req addSpaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.URL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
		return
	}

	spaceKey := session.GetSpaceKeyFromURL(req.URL)
	spaceName := session.GetSpaceNameFromURL(req.URL)

	if spaceKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid confluence URL: cannot extract space key"})
		return
	}

	ctx := context.Background()
	id, err := h.db.CreateSpace(ctx, spaceKey, spaceName, req.URL)
	if err != nil {
		h.log.Errorw("create space failed", "space_key", spaceKey, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create space"})
		return
	}

	h.log.Infow("space added", "id", id, "key", spaceKey, "name", spaceName)
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"message":   "space added",
		"space_key": spaceKey,
		"space_url": req.URL,
	})
}

// ActiveCrawls reports whether a space is being crawled (scraper.CrawlJobManager).
type ActiveCrawls interface {
	HasActiveJob(spaceKey string) bool
}

// SpaceByIDHandler handles individual space endpoints. savedBase, cron and
// crawls are used by DELETE; cron and crawls may be nil.
func SpaceByIDHandler(db store.Store, savedBase string, cron spaceops.CronConfigs, crawls ActiveCrawls, log logging.Sugar) http.HandlerFunc {
	return (&spaceByIDHandler{log: log, db: db, savedBase: savedBase, cron: cron, crawls: crawls}).handle
}

type spaceByIDHandler struct {
	log       logging.Sugar
	db        store.Store
	savedBase string
	cron      spaceops.CronConfigs
	crawls    ActiveCrawls
}

func (h *spaceByIDHandler) handle(w http.ResponseWriter, r *http.Request) {
	spaceKey := r.PathValue("key")
	if spaceKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "space key is required"})
		return
	}

	switch r.Method {
	case "GET":
		h.getStatus(w, r, spaceKey)
	case "DELETE":
		h.delete(w, r, spaceKey)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (h *spaceByIDHandler) getStatus(w http.ResponseWriter, r *http.Request, spaceKey string) {
	ctx := context.Background()
	space, err := h.db.GetSpaceByKey(ctx, spaceKey)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "space not found"})
		return
	}

	pages, _ := h.db.ListPages(ctx, spaceKey, 0, nil)
	formats, _ := h.db.CountPagesByBodyFormat(ctx, space.ID)

	var lastCrawledStr string
	if space.LastCrawled != nil {
		lastCrawledStr = space.LastCrawled.Format(time.RFC3339)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"space_key":      space.Key,
		"space_name":     space.Name,
		"space_url":      space.URL,
		"pages_crawled":  len(pages),
		"pages_total":    space.PagesTotal,
		"pages_storage":  formats.Storage,
		"pages_rendered": formats.Rendered,
		"last_crawled":   lastCrawledStr,
	})
}

// delete removes the space from the catalog (see spaceops.Delete);
// ?delete_files=1 also removes saved/<KEY>. Refused while the space is
// being crawled, since the crawl would recreate it.
func (h *spaceByIDHandler) delete(w http.ResponseWriter, r *http.Request, spaceKey string) {
	if !spaceops.ValidKey(spaceKey) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid space key"})
		return
	}
	if h.crawls != nil && h.crawls.HasActiveJob(spaceKey) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "space is being crawled; stop the crawl first"})
		return
	}

	opts := spaceops.DeleteOptions{DeleteFiles: r.URL.Query().Get("delete_files") == "1"}
	res, err := spaceops.Delete(r.Context(), h.db, h.savedBase, h.cron, spaceKey, opts)
	if errors.Is(err, spaceops.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "space not found"})
		return
	}
	if errors.Is(err, spaceops.ErrInvalidKey) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid space key"})
		return
	}
	if !res.Deleted {
		h.log.Errorw("delete space failed", "space_key", spaceKey, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete space"})
		return
	}
	resp := map[string]interface{}{
		"message":       "space deleted",
		"space_key":     res.SpaceKey,
		"pages_removed": res.Pages,
		"files_deleted": res.FilesDeleted,
	}
	if err != nil {
		// The space is removed; report leftovers (cron, files, index).
		h.log.Warnw("space deleted with leftovers", "space_key", spaceKey, "error", err)
		resp["warning"] = err.Error()
	}
	h.log.Infow("space deleted", "space_key", spaceKey, "pages", res.Pages, "files_deleted", res.FilesDeleted)
	writeJSON(w, http.StatusOK, resp)
}
