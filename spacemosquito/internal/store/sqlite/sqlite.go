package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vkh/spacemosquito/internal/config"
	"github.com/vkh/spacemosquito/internal/store"
	"github.com/vkh/spacemosquito/pkg/logging"
	"go.uber.org/zap"

	_ "modernc.org/sqlite"
)

var _ store.Store = (*DB)(nil)

type DB struct {
	sql *sql.DB
	log logging.Sugar
}

func New(cfg *config.Config, log *zap.Logger) (*DB, error) {
	path, err := DBFilePath(cfg)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create sqlite directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	db.SetMaxOpenConns(1)

	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	log.Info("connected to database", zap.String("path", path), zap.String("driver", "sqlite"))

	return &DB{
		sql: db,
		log: logging.New("sqlite", log),
	}, nil
}

func (d *DB) Close() {
	d.sql.Close()
}

func (d *DB) CreateSpace(ctx context.Context, key, name, url string) (uuid.UUID, error) {
	id := uuid.New()
	var out string
	err := d.sql.QueryRowContext(ctx,
		`INSERT INTO spaces (id, key, name, url) VALUES (?, ?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET name=excluded.name, url=excluded.url
		 RETURNING id`,
		id.String(), key, name, url,
	).Scan(&out)
	if err != nil {
		if d.log.Enabled() {
			d.log.Errorw("create space failed", "key", key, "name", name, "error", err)
		}
		return uuid.Nil, err
	}
	parsed, err := uuid.Parse(out)
	if err != nil {
		return uuid.Nil, err
	}
	if d.log.Enabled() {
		d.log.Infow("space created", "id", parsed, "key", key, "name", name)
	}
	return parsed, nil
}

func (d *DB) GetSpaceByKey(ctx context.Context, key string) (*store.Space, error) {
	var s store.Space
	var idStr string
	var lastCrawled sql.NullString
	var createdAt string
	err := d.sql.QueryRowContext(ctx,
		"SELECT id, key, name, url, last_crawled, COALESCE(pages_total, 0), created_at FROM spaces WHERE key = ?", key,
	).Scan(&idStr, &s.Key, &s.Name, &s.URL, &lastCrawled, &s.PagesTotal, &createdAt)
	if err != nil {
		if d.log.Enabled() {
			d.log.Debugw("get space by key: not found", "key", key, "error", err)
		}
		return nil, err
	}
	s.ID, err = uuid.Parse(idStr)
	if err != nil {
		return nil, err
	}
	if lastCrawled.Valid {
		t, err := parseTime(lastCrawled.String)
		if err != nil {
			return nil, err
		}
		s.LastCrawled = &t
	}
	s.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (d *DB) ListSpaces(ctx context.Context) ([]store.Space, error) {
	rows, err := d.sql.QueryContext(ctx,
		"SELECT id, key, name, url, last_crawled, COALESCE(pages_total, 0), created_at FROM spaces ORDER BY name")
	if err != nil {
		if d.log.Enabled() {
			d.log.Errorw("list spaces failed", "error", err)
		}
		return nil, err
	}
	defer rows.Close()

	var spaces []store.Space
	for rows.Next() {
		var s store.Space
		var idStr string
		var lastCrawled sql.NullString
		var createdAt string
		if err := rows.Scan(&idStr, &s.Key, &s.Name, &s.URL, &lastCrawled, &s.PagesTotal, &createdAt); err != nil {
			return nil, err
		}
		s.ID, err = uuid.Parse(idStr)
		if err != nil {
			return nil, err
		}
		if lastCrawled.Valid {
			t, err := parseTime(lastCrawled.String)
			if err != nil {
				return nil, err
			}
			s.LastCrawled = &t
		}
		s.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		spaces = append(spaces, s)
	}
	return spaces, nil
}

func (d *DB) UpdateSpaceLastCrawled(ctx context.Context, spaceKey string) error {
	_, err := d.sql.ExecContext(ctx,
		`UPDATE spaces SET last_crawled = ? WHERE key = ?`,
		time.Now().UTC().Format(time.RFC3339), spaceKey,
	)
	return err
}

func (d *DB) UpdateSpacePagesTotal(ctx context.Context, spaceKey string, pagesTotal int) error {
	_, err := d.sql.ExecContext(ctx,
		`UPDATE spaces SET pages_total = ? WHERE key = ?`,
		pagesTotal, spaceKey,
	)
	return err
}

func (d *DB) CountPagesBySpaceID(ctx context.Context, spaceID uuid.UUID) (int, error) {
	var count int
	err := d.sql.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM pages WHERE space_id = ?", spaceID.String(),
	).Scan(&count)
	return count, err
}

func (d *DB) CountPagesByBodyFormat(ctx context.Context, spaceID uuid.UUID) (store.BodyFormatCounts, error) {
	var c store.BodyFormatCounts
	err := d.sql.QueryRowContext(ctx,
		`SELECT
			COALESCE(SUM(CASE WHEN body_format = 'storage' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN body_format = 'storage' THEN 0 ELSE 1 END), 0)
		 FROM pages WHERE space_id = ?`,
		spaceID.String(),
	).Scan(&c.Storage, &c.Rendered)
	return c, err
}

func (d *DB) UpdatePageBodyFormat(ctx context.Context, pageID uuid.UUID, bodyFormat string) error {
	_, err := d.sql.ExecContext(ctx,
		`UPDATE pages SET body_format = ? WHERE id = ?`,
		bodyFormat, pageID.String(),
	)
	return err
}

func (d *DB) DeleteSpace(ctx context.Context, spaceKey string) error {
	res, err := d.sql.ExecContext(ctx, "DELETE FROM spaces WHERE key = ?", spaceKey)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (d *DB) UpsertPage(ctx context.Context, page *store.Page) error {
	if page.ID == uuid.Nil {
		page.ID = uuid.New()
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := d.sql.ExecContext(ctx,
		`INSERT INTO pages (id, space_id, confluence_id, version, title, parent_confluence_id, content, html_path, raw_html_path, metadata_path, file_dir, body_format, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (space_id, confluence_id) DO UPDATE SET
		   version=excluded.version,
		   title=excluded.title,
		   parent_confluence_id=excluded.parent_confluence_id,
		   content=excluded.content,
		   html_path=excluded.html_path,
		   raw_html_path=excluded.raw_html_path,
		   metadata_path=excluded.metadata_path,
		   file_dir=excluded.file_dir,
		   body_format=excluded.body_format,
		   updated_at=excluded.updated_at`,
		page.ID.String(), page.SpaceID.String(), page.ConfluenceID, page.Version, page.Title, page.ParentConfluenceID,
		page.Content, page.HTMLPath, page.RawHTMLPath, page.MetadataPath, page.FileDir, page.BodyFormat, now, now,
	)
	return err
}

func (d *DB) GetPage(ctx context.Context, spaceKey string, pageID int) (*store.Page, error) {
	var p store.Page
	var idStr, spaceIDStr string
	var createdAt, updatedAt string
	err := d.sql.QueryRowContext(ctx,
		`SELECT p.id, p.space_id, p.confluence_id, p.version, p.title, p.parent_confluence_id,
		        p.content, p.html_path, p.raw_html_path, p.metadata_path, p.file_dir, COALESCE(p.body_format, ''),
		        p.created_at, p.updated_at
		 FROM pages p
		 JOIN spaces s ON s.id = p.space_id
		 WHERE s.key = ? AND p.confluence_id = ?`,
		spaceKey, pageID,
	).Scan(&idStr, &spaceIDStr, &p.ConfluenceID, &p.Version, &p.Title, &p.ParentConfluenceID,
		&p.Content, &p.HTMLPath, &p.RawHTMLPath, &p.MetadataPath, &p.FileDir, &p.BodyFormat,
		&createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	p.ID, err = uuid.Parse(idStr)
	if err != nil {
		return nil, err
	}
	p.SpaceID, err = uuid.Parse(spaceIDStr)
	if err != nil {
		return nil, err
	}
	p.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	p.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (d *DB) GetPageByConfluenceID(ctx context.Context, confluenceID int, spaceKey string) (*store.Page, string, error) {
	if confluenceID <= 0 {
		return nil, "", fmt.Errorf("invalid confluence_id")
	}
	if spaceKey != "" {
		page, err := d.GetPage(ctx, spaceKey, confluenceID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, "", store.ErrPageNotFound
			}
			return nil, "", err
		}
		return page, spaceKey, nil
	}

	rows, err := d.sql.QueryContext(ctx,
		`SELECT p.id, p.space_id, p.confluence_id, p.version, p.title, p.parent_confluence_id,
		        p.content, p.html_path, p.raw_html_path, p.metadata_path, p.file_dir, COALESCE(p.body_format, ''),
		        p.created_at, p.updated_at, s.key
		 FROM pages p
		 JOIN spaces s ON s.id = p.space_id
		 WHERE p.confluence_id = ?
		 ORDER BY s.key`,
		confluenceID,
	)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	var matches []struct {
		page     store.Page
		spaceKey string
	}
	for rows.Next() {
		p, spaceKey, err := scanPageWithSpaceKey(rows)
		if err != nil {
			return nil, "", err
		}
		matches = append(matches, struct {
			page     store.Page
			spaceKey string
		}{page: p, spaceKey: spaceKey})
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	switch len(matches) {
	case 0:
		return nil, "", store.ErrPageNotFound
	case 1:
		return &matches[0].page, matches[0].spaceKey, nil
	default:
		candidates := make([]store.PageCandidate, len(matches))
		for i, m := range matches {
			candidates[i] = store.PageCandidate{
				SpaceKey: m.spaceKey,
				Title:    m.page.Title,
			}
		}
		return nil, "", &store.AmbiguousPageError{
			ConfluenceID: confluenceID,
			Candidates:   candidates,
		}
	}
}

func (d *DB) ListPages(ctx context.Context, spaceKey string, limit int, afterConfluenceID *int) ([]store.Page, error) {
	if limit == 0 {
		limit = 100
	}
	query := `SELECT p.id, p.space_id, p.confluence_id, p.version, p.title, p.parent_confluence_id,
		        p.content, p.html_path, p.raw_html_path, p.metadata_path, p.file_dir, COALESCE(p.body_format, ''),
		        p.created_at, p.updated_at
		 FROM pages p
		 JOIN spaces s ON s.id = p.space_id
		 WHERE s.key = ?`
	args := []any{spaceKey}
	if afterConfluenceID != nil {
		query += " AND p.confluence_id > ?"
		args = append(args, *afterConfluenceID)
	}
	query += " ORDER BY p.confluence_id ASC LIMIT ?"
	args = append(args, limit)

	rows, err := d.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pages []store.Page
	for rows.Next() {
		p, err := scanPage(rows)
		if err != nil {
			return nil, err
		}
		pages = append(pages, p)
	}
	return pages, nil
}

func (d *DB) ListAllPages(ctx context.Context) ([]store.Page, error) {
	rows, err := d.sql.QueryContext(ctx,
		`SELECT p.id, p.space_id, p.confluence_id, p.version, p.title, p.parent_confluence_id,
		        p.content, p.html_path, p.raw_html_path, p.metadata_path, p.file_dir, COALESCE(p.body_format, ''),
		        p.created_at, p.updated_at
		 FROM pages p
		 ORDER BY p.space_id, p.confluence_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pages []store.Page
	for rows.Next() {
		p, err := scanPage(rows)
		if err != nil {
			return nil, err
		}
		pages = append(pages, p)
	}
	return pages, rows.Err()
}

func (d *DB) ListPageSummaries(ctx context.Context, spaceKey string, limit int, afterConfluenceID *int) ([]store.PageSummary, error) {
	if limit == 0 {
		limit = 100
	}
	query := `SELECT p.id, p.confluence_id, p.version, p.title, p.parent_confluence_id,
		        p.created_at, p.updated_at
		 FROM pages p
		 JOIN spaces s ON s.id = p.space_id
		 WHERE s.key = ?`
	args := []any{spaceKey}
	if afterConfluenceID != nil {
		query += " AND p.confluence_id > ?"
		args = append(args, *afterConfluenceID)
	}
	query += " ORDER BY p.confluence_id ASC LIMIT ?"
	args = append(args, limit)

	rows, err := d.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []store.PageSummary
	for rows.Next() {
		var s store.PageSummary
		var idStr string
		var createdAt, updatedAt string
		if err := rows.Scan(&idStr, &s.ConfluenceID, &s.Version, &s.Title, &s.ParentConfluenceID,
			&createdAt, &updatedAt); err != nil {
			return nil, err
		}
		s.ID, err = uuid.Parse(idStr)
		if err != nil {
			return nil, err
		}
		s.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		s.UpdatedAt, err = parseTime(updatedAt)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, s)
	}
	return summaries, nil
}

func (d *DB) DeleteStalePages(ctx context.Context, spaceID uuid.UUID, crawlStart time.Time) (int64, error) {
	res, err := d.sql.ExecContext(ctx,
		`DELETE FROM pages WHERE space_id = ? AND updated_at < ?`,
		spaceID.String(), crawlStart.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (d *DB) SearchPages(ctx context.Context, query string, spaceKey string, limit int) ([]store.SearchResult, error) {
	if limit == 0 {
		limit = 10
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	results, err := d.searchPagesFTS(ctx, query, spaceKey, limit)
	if err != nil {
		return nil, err
	}
	if len(results) > 0 {
		return results, nil
	}
	return d.searchPagesTitleFallback(ctx, query, spaceKey, limit)
}

func (d *DB) searchPagesFTS(ctx context.Context, query, spaceKey string, limit int) ([]store.SearchResult, error) {
	ftsQuery := buildFTSQuery(query)
	baseQuery := `
		SELECT p.confluence_id, s.key, p.title,
		       snippet(pages_fts, 2, char(57344), char(57345), '...', 20) AS excerpt, -- U+E000/U+E001: search.MatchStart/MatchEnd
		       bm25(pages_fts, 0.0, 10.0, 1.0) AS similarity,
		       CASE WHEN p.file_dir <> '' THEN p.file_dir || '/content.md' ELSE p.html_path END, p.id -- file_path: page Markdown (ADR-018)
		FROM pages_fts
		JOIN pages p ON p.id = pages_fts.page_id
		JOIN spaces s ON s.id = p.space_id
		WHERE pages_fts MATCH ?`
	args := []any{ftsQuery}
	if spaceKey != "" {
		baseQuery += " AND s.key = ?"
		args = append(args, spaceKey)
	}
	baseQuery += " ORDER BY similarity LIMIT ?"
	args = append(args, limit)

	rows, err := d.sql.QueryContext(ctx, baseQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("search pages: %w", err)
	}
	defer rows.Close()

	return scanSearchResults(rows)
}

func (d *DB) searchPagesTitleFallback(ctx context.Context, query, spaceKey string, limit int) ([]store.SearchResult, error) {
	words := searchQueryTokens(query)
	if len(words) == 0 {
		return nil, nil
	}

	baseQuery := `
		SELECT p.confluence_id, s.key, p.title,
		       substr(p.title, 1, 200) AS excerpt,
		       0.0 AS similarity,
		       CASE WHEN p.file_dir <> '' THEN p.file_dir || '/content.md' ELSE p.html_path END, p.id -- file_path: page Markdown (ADR-018)
		FROM pages p
		JOIN spaces s ON s.id = p.space_id
		WHERE 1=1`
	args := make([]any, 0, len(words)+2)
	for _, w := range words {
		baseQuery += " AND lower(p.title) LIKE ?"
		args = append(args, "%"+strings.ToLower(w)+"%")
	}
	if spaceKey != "" {
		baseQuery += " AND s.key = ?"
		args = append(args, spaceKey)
	}
	baseQuery += " ORDER BY p.title LIMIT ?"
	args = append(args, limit)

	rows, err := d.sql.QueryContext(ctx, baseQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("search pages title fallback: %w", err)
	}
	defer rows.Close()

	return scanSearchResults(rows)
}

func scanSearchResults(rows *sql.Rows) ([]store.SearchResult, error) {
	var results []store.SearchResult
	for rows.Next() {
		var r store.SearchResult
		var idStr string
		if err := rows.Scan(&r.ConfluenceID, &r.SpaceKey, &r.Title, &r.Excerpt, &r.Similarity, &r.FilePath, &idStr); err != nil {
			return nil, err
		}
		var err error
		r.InternalID, err = uuid.Parse(idStr)
		if err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (d *DB) IndexPageContent(ctx context.Context, spaceKey string, pageID int) error {
	// FTS rows are maintained by triggers on pages.
	return nil
}

func (d *DB) IndexAllPageContents(ctx context.Context) error {
	_, err := d.sql.ExecContext(ctx, `INSERT INTO pages_fts(pages_fts) VALUES('rebuild')`)
	if err != nil {
		return fmt.Errorf("rebuild fts index: %w", err)
	}
	return nil
}

func (d *DB) GetPageStats(ctx context.Context) (*store.PageStats, error) {
	stats := &store.PageStats{}
	var lastCrawled sql.NullString
	var contentLang string
	err := d.sql.QueryRowContext(ctx, `
		SELECT (SELECT COUNT(*) FROM pages),
		       (SELECT COUNT(*) FROM spaces),
		       (SELECT MAX(last_crawled) FROM spaces),
		       CASE WHEN EXISTS (SELECT 1 FROM pages_fts LIMIT 1)
		            THEN 'fts5' ELSE 'none' END
	`).Scan(&stats.TotalPages, &stats.TotalSpaces, &lastCrawled, &contentLang)
	if err != nil {
		return nil, err
	}
	stats.ContentLang = contentLang
	if lastCrawled.Valid && lastCrawled.String != "" {
		t, err := parseTime(lastCrawled.String)
		if err != nil {
			return nil, err
		}
		stats.LastCrawled = t
		stats.LastCrawledStr = t.Format(time.RFC3339)
	}
	return stats, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPage(rows rowScanner) (store.Page, error) {
	var p store.Page
	var idStr, spaceIDStr string
	var createdAt, updatedAt string
	err := rows.Scan(&idStr, &spaceIDStr, &p.ConfluenceID, &p.Version, &p.Title, &p.ParentConfluenceID,
		&p.Content, &p.HTMLPath, &p.RawHTMLPath, &p.MetadataPath, &p.FileDir, &p.BodyFormat,
		&createdAt, &updatedAt)
	if err != nil {
		return p, err
	}
	p.ID, err = uuid.Parse(idStr)
	if err != nil {
		return p, err
	}
	p.SpaceID, err = uuid.Parse(spaceIDStr)
	if err != nil {
		return p, err
	}
	p.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return p, err
	}
	p.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return p, err
	}
	return p, nil
}

func scanPageWithSpaceKey(rows rowScanner) (store.Page, string, error) {
	var p store.Page
	var idStr, spaceIDStr, spaceKey string
	var createdAt, updatedAt string
	err := rows.Scan(&idStr, &spaceIDStr, &p.ConfluenceID, &p.Version, &p.Title, &p.ParentConfluenceID,
		&p.Content, &p.HTMLPath, &p.RawHTMLPath, &p.MetadataPath, &p.FileDir, &p.BodyFormat,
		&createdAt, &updatedAt, &spaceKey)
	if err != nil {
		return p, "", err
	}
	p.ID, err = uuid.Parse(idStr)
	if err != nil {
		return p, "", err
	}
	p.SpaceID, err = uuid.Parse(spaceIDStr)
	if err != nil {
		return p, "", err
	}
	p.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return p, "", err
	}
	p.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return p, "", err
	}
	return p, spaceKey, nil
}

func parseTime(s string) (time.Time, error) {
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("parse time %q", s)
}

func searchQueryTokens(q string) []string {
	return strings.Fields(strings.TrimSpace(q))
}

func buildFTSQuery(q string) string {
	words := searchQueryTokens(q)
	if len(words) == 0 {
		return q
	}
	for i, w := range words {
		w = strings.ReplaceAll(w, `"`, `""`)
		words[i] = `"` + w + `"`
	}
	return strings.Join(words, " AND ")
}
