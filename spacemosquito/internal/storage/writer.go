package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vkh/spacemosquito/pkg/logging"
)

type Metadata struct {
	Title         string     `json:"title"`
	ConfluenceURL string     `json:"confluence_url"`
	SpaceKey      string     `json:"space_key"`
	ParentTitle   string     `json:"parent_title,omitempty"`
	Author        string     `json:"author,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	Images        []AssetRef `json:"images,omitempty"`
	Diagrams      []AssetRef `json:"diagrams,omitempty"`
	Attachments   []AssetRef `json:"attachments,omitempty"`
	// BodyFormat records the source format of raw.html: "storage" (Confluence
	// Storage Format, API crawl) or "rendered" (browser fallback). Used to route
	// reindex/import to the right converter.
	BodyFormat string    `json:"body_format,omitempty"`
	SavedAt    time.Time `json:"saved_at"`
}

type AssetRef struct {
	OriginalURL string `json:"original_url"`
	LocalPath   string `json:"local_path"`
}

type Writer struct {
	basePath string
	log      logging.Sugar
}

func NewWriter(basePath string, log logging.Sugar) *Writer {
	return &Writer{basePath: basePath, log: log}
}

// BasePath is the saved catalog root (pages live under {base}/{spaceKey}/).
func (w *Writer) BasePath() string {
	return w.basePath
}

func (w *Writer) MakePageDir(spaceKey, pageTitle string, confluenceID int) (string, error) {
	// {id}-{title}: ID first so it is never truncated; title is sanitized/capped at 100.
	safeTitle := sanitizeFilename(pageTitle)
	dirName := fmt.Sprintf("%d-%s", confluenceID, safeTitle)
	dir := filepath.Join(w.basePath, spaceKey, dirName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		if w.log.Enabled() {
			w.log.Errorw("make page dir failed: directory creation",
				"space", spaceKey,
				"title", pageTitle,
				"confluence_id", confluenceID,
				"path", dir,
				"error", err)
		}
		return "", fmt.Errorf("create page dir: %w", err)
	}
	if w.log.Enabled() {
		w.log.Infow("page directory created",
			"space", spaceKey,
			"title", pageTitle,
			"confluence_id", confluenceID,
			"path", dir)
	}
	return dir, nil
}

// ClearPageDir removes all contents of dir but keeps the directory itself.
// A missing directory is a no-op.
func (w *Writer) ClearPageDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read page dir: %w", err)
	}
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		if err := os.RemoveAll(path); err != nil {
			if w.log.Enabled() {
				w.log.Errorw("clear page dir failed", "path", path, "error", err)
			}
			return fmt.Errorf("remove %s: %w", path, err)
		}
	}
	if w.log.Enabled() {
		w.log.Infow("page directory cleared", "path", dir)
	}
	return nil
}

func (w *Writer) SaveHTML(dir, html string) error {
	path := filepath.Join(dir, "index.html")
	if err := os.WriteFile(path, []byte(html), 0644); err != nil {
		if w.log.Enabled() {
			w.log.Errorw("save HTML failed", "path", path, "error", err)
		}
		return fmt.Errorf("save html: %w", err)
	}
	if w.log.Enabled() {
		w.log.Infow("HTML saved", "path", path, "bytes", len(html))
	}
	return nil
}

func (w *Writer) SaveMarkdown(dir, markdown string) error {
	path := filepath.Join(dir, "content.md")
	if err := os.WriteFile(path, []byte(markdown), 0644); err != nil {
		if w.log.Enabled() {
			w.log.Errorw("save markdown failed", "path", path, "error", err)
		}
		return fmt.Errorf("save markdown: %w", err)
	}
	if w.log.Enabled() {
		w.log.Infow("markdown saved", "path", path, "bytes", len(markdown))
	}
	return nil
}

func (w *Writer) SaveRawHTML(dir, html string) error {
	path := filepath.Join(dir, "raw.html")
	if err := os.WriteFile(path, []byte(html), 0644); err != nil {
		if w.log.Enabled() {
			w.log.Errorw("save raw HTML failed", "path", path, "error", err)
		}
		return fmt.Errorf("save raw html: %w", err)
	}
	if w.log.Enabled() {
		w.log.Infow("raw HTML saved", "path", path, "bytes", len(html))
	}
	return nil
}

func (w *Writer) SaveMetadata(dir string, meta *Metadata) error {
	path := filepath.Join(dir, "metadata.json")
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		if w.log.Enabled() {
			w.log.Errorw("save metadata failed: marshal error", "error", err)
		}
		return fmt.Errorf("marshal metadata: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		if w.log.Enabled() {
			w.log.Errorw("save metadata failed: file write", "path", path, "error", err)
		}
		return fmt.Errorf("write metadata file: %w", err)
	}
	if w.log.Enabled() {
		w.log.Infow("metadata saved", "path", path, "title", meta.Title, "space", meta.SpaceKey)
	}
	return nil
}

func (w *Writer) SaveAsset(dir, url, localPath string) (string, error) {
	destDir := filepath.Join(dir, localPath)
	if err := os.MkdirAll(filepath.Dir(destDir), 0755); err != nil {
		if w.log.Enabled() {
			w.log.Errorw("save asset: directory creation failed",
				"space", dir,
				"url", url,
				"error", err)
		}
		return "", fmt.Errorf("create asset dir: %w", err)
	}
	if w.log.Enabled() {
		w.log.Infow("asset directory created", "path", filepath.Dir(destDir), "url", url)
	}
	return destDir, nil
}

func sanitizeFilename(name string) string {
	s := strings.ReplaceAll(name, "/", "-")
	s = strings.ReplaceAll(s, "\\", "-")
	s = strings.ReplaceAll(s, ":", "-")
	s = strings.TrimSpace(s)
	if len(s) > 100 {
		s = s[:100]
	}
	return s
}
