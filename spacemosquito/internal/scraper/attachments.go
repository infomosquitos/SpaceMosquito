package scraper

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/vkh/spacemosquito/internal/config"
	"github.com/vkh/spacemosquito/internal/confluence"
	"github.com/vkh/spacemosquito/internal/contentmd/csf"
	"github.com/vkh/spacemosquito/internal/ratelimit"
	"github.com/vkh/spacemosquito/internal/session"
	"github.com/vkh/spacemosquito/internal/storage"
)

// attachmentFile is a page's file attachment (pptx, pdf, xlsx, …) as listed
// by the Confluence REST API.
type attachmentFile struct {
	Name      string
	MediaType string
	Size      int64
	URL       string // absolute download URL
}

// attachmentsDir holds downloaded file attachments in a page directory.
const attachmentsDir = "assets/attachments"

// fetchAttachments lists a page's attachments (GET content/{id}/child/attachment,
// paginated) and keeps the files: images are saved where the page shows them
// (assets/images) and draw.io sources are rendered as diagrams.
func (s *Scraper) fetchAttachments(baseURL string, pageID int, sess *session.Session) ([]attachmentFile, error) {
	cloud := sess.Flavor == session.FlavorCloud
	api := baseURL + "/rest/api/content/"
	if cloud {
		api = baseURL + "/wiki/rest/api/content/"
	}
	client := &http.Client{Timeout: 30 * time.Second}
	var files []attachmentFile
	const limit = 100
	for start := 0; ; start += limit {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s%d/child/attachment?limit=%d&start=%d", api, pageID, limit, start), nil)
		if err != nil {
			return nil, err
		}
		for k, v := range sess.AsHeaders() {
			req.Header.Set(k, v)
		}
		resp, err := ratelimit.Do(client, req, s.logThrottle)
		if err != nil {
			return nil, err
		}
		var result struct {
			Results []struct {
				Title      string `json:"title"`
				Extensions struct {
					MediaType string `json:"mediaType"`
					FileSize  int64  `json:"fileSize"`
				} `json:"extensions"`
				Links struct {
					Download string `json:"download"`
				} `json:"_links"`
			} `json:"results"`
			Size int `json:"size"`
		}
		err = func() error {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("list attachments of page %d: HTTP %d", pageID, resp.StatusCode)
			}
			return json.NewDecoder(resp.Body).Decode(&result)
		}()
		if err != nil {
			return nil, err
		}
		for _, r := range result.Results {
			f := attachmentFile{Name: r.Title, MediaType: r.Extensions.MediaType, Size: r.Extensions.FileSize}
			if r.Links.Download != "" {
				f.URL = confluence.WebUIURL(baseURL, r.Links.Download, cloud)
			}
			if isFileAttachment(f) {
				files = append(files, f)
			}
		}
		if len(result.Results) < limit {
			return files, nil
		}
	}
}

// isFileAttachment reports whether an attachment is a document to offer as a
// file: not an image (shown inline) and not a draw.io source.
func isFileAttachment(f attachmentFile) bool {
	mt := strings.ToLower(f.MediaType)
	switch {
	case f.Name == "" || f.URL == "":
		return false
	case strings.HasPrefix(mt, "image/"):
		return false
	case strings.Contains(mt, "jgraph") || strings.Contains(mt, "drawio"):
		return false
	case filepath.Ext(f.Name) == "" || strings.HasSuffix(strings.ToLower(f.Name), ".tmp"):
		// draw.io stores diagrams as extensionless attachments
		return false
	}
	return true
}

// maxAttachmentBytes is the configured attachment size limit (-1 = none).
func (s *Scraper) maxAttachmentBytes() int64 {
	if s.cfg == nil {
		return config.StorageConfig{}.MaxAttachmentBytes()
	}
	return s.cfg.Storage.MaxAttachmentBytes()
}

func (s *Scraper) downloadable(f attachmentFile) bool {
	limit := s.maxAttachmentBytes()
	return limit < 0 || f.Size <= limit
}

// attachmentLocalName is the file name under assets/attachments.
func attachmentLocalName(name string) string {
	name = strings.NewReplacer("/", "_", "\\", "_").Replace(strings.TrimSpace(name))
	if name == "." || name == ".." {
		return "_" + name
	}
	return name
}

// fileLinks maps attachment names to where content.md links them: the local
// copy, or Confluence for files over the size limit.
func (s *Scraper) fileLinks(files []attachmentFile) map[string]csf.FileLink {
	links := make(map[string]csf.FileLink, len(files))
	for _, f := range files {
		href := f.URL
		if s.downloadable(f) {
			href = attachmentsDir + "/" + url.PathEscape(attachmentLocalName(f.Name))
		}
		links[f.Name] = csf.FileLink{Href: href, Size: f.Size}
	}
	return links
}

// downloadAttachments saves a page's file attachments into dir and returns
// their metadata. Files over the size limit or failing to download are
// recorded without a local path (linked to Confluence).
func (s *Scraper) downloadAttachments(dir string, files []attachmentFile) []storage.AssetRef {
	refs := make([]storage.AssetRef, 0, len(files))
	for _, f := range files {
		ref := storage.AssetRef{OriginalURL: f.URL, Name: f.Name, MediaType: f.MediaType, Size: f.Size}
		if s.downloadable(f) {
			rel := filepath.Join(attachmentsDir, attachmentLocalName(f.Name))
			if err := s.assets.DownloadFile(filepath.Join(dir, rel), f.URL, f.Size); err != nil {
				s.log.Warnw("attachment download failed", "name", f.Name, "url", f.URL, "error", err)
			} else {
				ref.LocalPath = filepath.ToSlash(rel)
			}
		} else if s.log.Enabled() {
			s.log.Infow("attachment over size limit, linked to Confluence", "name", f.Name, "size", f.Size)
		}
		refs = append(refs, ref)
	}
	return refs
}
