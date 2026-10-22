package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vkh/spacemosquito/internal/contentmd"
	"github.com/vkh/spacemosquito/internal/session"
	"github.com/vkh/spacemosquito/internal/storage"
	"github.com/vkh/spacemosquito/internal/store"
	"github.com/vkh/spacemosquito/pkg/logging"
)

const (
	ModeImportSaved = "import_saved"
	defaultLogEvery = 100
)

var confluenceIDRe = regexp.MustCompile(`/pages/(\d+)`)

type Options struct {
	FromDir   string
	ReportDir string
	Force     bool
	DryRun    bool
	LogEvery  int
}

type Report struct {
	Mode           string        `json:"mode"`
	From           string        `json:"from"`
	StartedAt      time.Time     `json:"started_at"`
	FinishedAt     time.Time     `json:"finished_at"`
	ScannedFiles   int           `json:"scanned_files"`
	ImportedPages  int           `json:"imported_pages"`
	ImportedSpaces int           `json:"imported_spaces"`
	Deduplicated   int           `json:"deduplicated"`
	Skipped        []ReportItem  `json:"skipped,omitempty"`
	Errors         []ReportError `json:"errors,omitempty"`
}

type ReportItem struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type ReportError struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

type importRecord struct {
	path         string
	spaceKey     string
	spaceURL     string
	title        string
	confluenceID int
	content      string
	htmlPath     string
	rawHTMLPath  string
	metadataPath string
	fileDir      string
	updatedAt    time.Time
	history      store.PageHistory
	savedAt      time.Time
	modTime      time.Time
}

func ImportSaved(ctx context.Context, db store.Store, opts Options, log logging.Sugar) (Report, error) {
	now := time.Now().UTC()
	report := Report{
		Mode:      ModeImportSaved,
		From:      opts.FromDir,
		StartedAt: now,
	}
	if opts.LogEvery <= 0 {
		opts.LogEvery = defaultLogEvery
	}
	if strings.TrimSpace(opts.FromDir) == "" {
		return report, fmt.Errorf("from directory is required")
	}
	if strings.TrimSpace(opts.ReportDir) == "" {
		return report, fmt.Errorf("report directory is required")
	}

	stats, err := db.GetPageStats(ctx)
	if err != nil {
		return report, fmt.Errorf("check existing pages: %w", err)
	}
	if stats.TotalPages > 0 && !opts.Force {
		return report, fmt.Errorf("database already has %d pages, rerun with --force", stats.TotalPages)
	}
	if opts.Force && !opts.DryRun {
		if err := clearPages(ctx, db); err != nil {
			return report, fmt.Errorf("clear pages with --force: %w", err)
		}
	}

	records, deduped, skipped, errs := collectRecords(opts.FromDir)
	report.Deduplicated = deduped
	report.Skipped = append(report.Skipped, skipped...)
	report.Errors = append(report.Errors, errs...)
	report.ScannedFiles = len(records) + len(report.Skipped) + len(report.Errors)

	if !opts.DryRun {
		createdSpaces, importedPages, applyErrs := applyRecords(ctx, db, records, opts.LogEvery, log)
		report.ImportedSpaces = createdSpaces
		report.ImportedPages = importedPages
		report.Errors = append(report.Errors, applyErrs...)
		if err := db.IndexAllPageContents(ctx); err != nil {
			report.Errors = append(report.Errors, ReportError{Path: "fts", Error: err.Error()})
		}
	}

	report.FinishedAt = time.Now().UTC()
	if err := writeReport(opts.ReportDir, report); err != nil {
		return report, err
	}
	return report, nil
}

func clearPages(ctx context.Context, db store.Store) error {
	spaces, err := db.ListSpaces(ctx)
	if err != nil {
		return err
	}
	cutoff := time.Now().UTC().Add(365 * 24 * time.Hour)
	for _, s := range spaces {
		if _, err := db.DeleteStalePages(ctx, s.ID, cutoff); err != nil {
			return err
		}
	}
	return nil
}

func collectRecords(from string) ([]importRecord, int, []ReportItem, []ReportError) {
	byKey := map[string]importRecord{}
	var deduplicated int
	var skipped []ReportItem
	var errs []ReportError

	_ = filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			errs = append(errs, ReportError{Path: path, Error: err.Error()})
			return nil
		}
		if d.IsDir() || d.Name() != "metadata.json" {
			return nil
		}
		rec, skipReason, parseErr := parseRecord(path)
		if parseErr != nil {
			errs = append(errs, ReportError{Path: path, Error: parseErr.Error()})
			return nil
		}
		if skipReason != "" {
			skipped = append(skipped, ReportItem{Path: path, Reason: skipReason})
			return nil
		}
		key := fmt.Sprintf("%s:%d", rec.spaceKey, rec.confluenceID)
		prev, exists := byKey[key]
		if exists {
			deduplicated++
			if !preferRecord(rec, prev) {
				return nil
			}
		}
		byKey[key] = rec
		return nil
	})

	records := make([]importRecord, 0, len(byKey))
	for _, rec := range byKey {
		records = append(records, rec)
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].spaceKey == records[j].spaceKey {
			return records[i].confluenceID < records[j].confluenceID
		}
		return records[i].spaceKey < records[j].spaceKey
	})
	return records, deduplicated, skipped, errs
}

func parseRecord(metaPath string) (importRecord, string, error) {
	var rec importRecord
	rec.metadataPath = metaPath
	rec.fileDir = filepath.Dir(metaPath)
	rec.path = rec.fileDir

	stat, err := os.Stat(metaPath)
	if err == nil {
		rec.modTime = stat.ModTime().UTC()
	}

	b, err := os.ReadFile(metaPath)
	if err != nil {
		return rec, "", err
	}
	var meta storage.Metadata
	if err := json.Unmarshal(b, &meta); err != nil {
		return rec, "", err
	}

	rec.spaceKey = strings.TrimSpace(meta.SpaceKey)
	if rec.spaceKey == "" {
		rec.spaceKey = filepath.Base(filepath.Dir(rec.fileDir))
	}
	if rec.spaceKey == "" {
		return rec, "missing space_key", nil
	}
	rec.title = strings.TrimSpace(meta.Title)
	if rec.title == "" {
		rec.title = filepath.Base(rec.fileDir)
	}
	rec.spaceURL = deriveSpaceURL(meta.ConfluenceURL, rec.spaceKey)
	rec.updatedAt = firstNonZero(meta.UpdatedAt.UTC(), meta.SavedAt.UTC(), rec.modTime)
	rec.savedAt = firstNonZero(meta.SavedAt.UTC(), rec.modTime)
	rec.history = metadataHistory(meta)

	rec.confluenceID = parseConfluenceID(meta.ConfluenceURL)
	if rec.confluenceID <= 0 {
		return rec, "missing confluence_id in confluence_url", nil
	}

	// Storage Format pages are browsed from content.md (ADR-018); only
	// browser-fallback pages have an index.html of their own.
	if contentmd.DetectBodyFormat(rec.fileDir) != "storage" {
		rec.htmlPath = filepath.Join(rec.fileDir, "index.html")
	}
	rec.rawHTMLPath = filepath.Join(rec.fileDir, "raw.html")
	content, skipReason, err := contentmd.RenderDirMarkdown(rec.fileDir)
	if err != nil {
		return rec, "", err
	}
	if skipReason != "" {
		return rec, skipReason, nil
	}
	rec.content = content
	if strings.TrimSpace(content) != "" {
		_ = contentmd.WriteFile(rec.fileDir, content)
	}

	return rec, "", nil
}

func applyRecords(ctx context.Context, db store.Store, records []importRecord, logEvery int, log logging.Sugar) (int, int, []ReportError) {
	spaceIDs := map[string]string{}
	spaceCreated := map[string]bool{}
	spaceTouched := map[string]bool{}
	var imported int
	var errs []ReportError

	for i, rec := range records {
		if log.Enabled() && (i == 0 || (i+1)%logEvery == 0) {
			log.Infow("import progress", "processed", i+1, "total", len(records))
		}

		space, err := db.GetSpaceByKey(ctx, rec.spaceKey)
		if err != nil {
			id, createErr := db.CreateSpace(ctx, rec.spaceKey, rec.spaceKey, rec.spaceURL)
			if createErr != nil {
				errs = append(errs, ReportError{Path: rec.path, Error: createErr.Error()})
				continue
			}
			spaceIDs[rec.spaceKey] = id.String()
			spaceCreated[rec.spaceKey] = true
		} else {
			spaceIDs[rec.spaceKey] = space.ID.String()
		}

		page := &store.Page{
			ConfluenceID: rec.confluenceID,
			Version:      0,
			Title:        rec.title,
			Content:      rec.content,
			HTMLPath:     rec.htmlPath,
			RawHTMLPath:  rec.rawHTMLPath,
			MetadataPath: rec.metadataPath,
			FileDir:      rec.fileDir,
			BodyFormat:   contentmd.DetectBodyFormat(rec.fileDir),
			UpdatedAt:    rec.updatedAt,
			History:      rec.history,
		}
		if sid, ok := spaceIDs[rec.spaceKey]; ok {
			if parsed, parseErr := uuid.Parse(sid); parseErr == nil {
				page.SpaceID = parsed
			}
		}

		if err := db.UpsertPage(ctx, page); err != nil {
			errs = append(errs, ReportError{Path: rec.path, Error: err.Error()})
			continue
		}
		spaceTouched[rec.spaceKey] = true
		imported++
	}

	for key := range spaceTouched {
		_ = db.UpdateSpaceLastCrawled(ctx, key)
	}

	return len(spaceCreated), imported, errs
}

func parseConfluenceID(confluenceURL string) int {
	m := confluenceIDRe.FindStringSubmatch(confluenceURL)
	if len(m) != 2 {
		return 0
	}
	var id int
	_, _ = fmt.Sscanf(m[1], "%d", &id)
	return id
}

func deriveSpaceURL(confluenceURL, spaceKey string) string {
	if spaceKey == "" {
		return ""
	}
	prefix := strings.Split(confluenceURL, "/spaces/")
	if len(prefix) >= 2 {
		return strings.TrimSuffix(prefix[0], "/") + "/spaces/" + spaceKey
	}
	keyFromURL := session.GetSpaceKeyFromURL(confluenceURL)
	if keyFromURL != "" {
		i := strings.Index(confluenceURL, "/spaces/"+keyFromURL)
		if i > 0 {
			return confluenceURL[:i] + "/spaces/" + spaceKey
		}
	}
	return ""
}

func preferRecord(a, b importRecord) bool {
	if a.updatedAt.After(b.updatedAt) {
		return true
	}
	if b.updatedAt.After(a.updatedAt) {
		return false
	}
	if a.savedAt.After(b.savedAt) {
		return true
	}
	if b.savedAt.After(a.savedAt) {
		return false
	}
	return a.modTime.After(b.modTime)
}

func firstNonZero(times ...time.Time) time.Time {
	for _, t := range times {
		if !t.IsZero() {
			return t
		}
	}
	return time.Now().UTC()
}

func writeReport(dir string, report Report) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create report dir: %w", err)
	}
	name := fmt.Sprintf("bootstrap-import-%s.json", report.FinishedAt.Format("20060102-150405"))
	path := filepath.Join(dir, name)
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}

// metadataHistory reads Confluence history from metadata.json. Versions
// before 0.8.1 wrote the local save time into created_at/updated_at and left
// the names empty, so a time is trusted only together with its name.
func metadataHistory(meta storage.Metadata) store.PageHistory {
	var h store.PageHistory
	if meta.Author != "" {
		h.CreatedBy = meta.Author
		if !meta.CreatedAt.IsZero() {
			t := meta.CreatedAt.UTC()
			h.CreatedAt = &t
		}
	}
	if meta.UpdatedBy != "" {
		h.UpdatedBy = meta.UpdatedBy
		if !meta.UpdatedAt.IsZero() {
			t := meta.UpdatedAt.UTC()
			h.UpdatedAt = &t
		}
	}
	return h
}
