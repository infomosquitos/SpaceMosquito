package storage

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vkh/spacemosquito/internal/ratelimit"
	"github.com/vkh/spacemosquito/pkg/logging"
)

const (
	defaultMaxRetries = 3
	defaultRetryDelay = 2 * time.Second
	// DefaultAssetDelay is the pause before each image/attachment download
	// (storage.asset_delay_ms); throttling by Confluence raises it.
	DefaultAssetDelay = 1500 * time.Millisecond
	maxAssetDelay     = 30 * time.Second
)

type AssetDownloader struct {
	client      *http.Client
	log         logging.Sugar
	maxRetries  int
	retryDelay  time.Duration
	rateLimit   time.Duration
	mu          sync.Mutex
	lastReq     time.Time
	authHeaders map[string]string
	force       bool
	fileClient  *http.Client // attachments; nil = fileClient
}

// SetAuthHeaders sets the headers (e.g. the session Cookie) attached to every
// asset request. Without them, attachment downloads on SSO-protected instances
// receive a login/redirect page instead of the file. The scraper sets these
// from the active session at crawl time.
func (d *AssetDownloader) SetAuthHeaders(h map[string]string) {
	m := make(map[string]string, len(h))
	for k, v := range h {
		m[k] = v
	}
	d.mu.Lock()
	d.authHeaders = m
	d.mu.Unlock()
}

// SetDelay sets the pause before each download (0 = none).
func (d *AssetDownloader) SetDelay(delay time.Duration) {
	if delay < 0 {
		delay = 0
	}
	d.mu.Lock()
	d.rateLimit = delay
	d.mu.Unlock()
}

// throttled handles HTTP 429: it waits as long as Confluence asks
// (Retry-After) and doubles the pause between downloads for the rest of the
// run, up to maxAssetDelay, so the limit is not hit again right away.
func (d *AssetDownloader) throttled(h http.Header, rawURL string) {
	wait := ratelimit.RetryAfter(h, time.Now())
	d.mu.Lock()
	next := d.rateLimit * 2
	if next < DefaultAssetDelay {
		next = DefaultAssetDelay
	}
	if next > maxAssetDelay {
		next = maxAssetDelay
	}
	d.rateLimit = next
	d.mu.Unlock()
	if d.log.Enabled() {
		d.log.Warnw("confluence is throttling downloads, waiting", "url", rawURL, "wait", wait.String(), "new_delay", next.String())
	}
	ratelimit.Sleep(wait)
}

// SetForce controls whether existing on-disk assets are re-downloaded.
// When false (default), non-empty destination files are skipped before any
// HTTP request. When true, every asset is fetched and overwritten.
func (d *AssetDownloader) SetForce(force bool) {
	d.mu.Lock()
	d.force = force
	d.mu.Unlock()
}

func (d *AssetDownloader) shouldSkip(path string) bool {
	d.mu.Lock()
	force := d.force
	d.mu.Unlock()
	if force {
		return false
	}
	fi, err := os.Stat(path)
	return err == nil && fi.Size() > 0
}

// get issues an authenticated GET, attaching any configured auth headers.
func (d *AssetDownloader) get(rawURL string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	for k, v := range d.authHeaders {
		req.Header.Set(k, v)
	}
	d.mu.Unlock()
	return d.client.Do(req)
}

// retryableStatus reports whether a failed download may succeed when tried
// again: rate limiting and server errors. Others (404 missing, 403 denied, …)
// fail at once instead of retrying with growing delays.
func retryableStatus(code int) bool {
	return code == http.StatusTooManyRequests || code == http.StatusRequestTimeout || code >= 500
}

// looksLikeLoginPage reports whether a response is an HTML page rather than the
// requested binary asset — the tell-tale sign of an unauthenticated request
// being answered with an SSO login/redirect page.
func looksLikeLoginPage(resp *http.Response) bool {
	return strings.Contains(resp.Header.Get("Content-Type"), "text/html")
}

func NewAssetDownloader(log logging.Sugar) *AssetDownloader {
	return &AssetDownloader{
		client:     &http.Client{Timeout: 30 * time.Second},
		log:        log,
		maxRetries: defaultMaxRetries,
		retryDelay: defaultRetryDelay,
		rateLimit:  DefaultAssetDelay,
	}
}

// SetClientForTest swaps the HTTP client and disables rate limiting / retries
// so scraper integration tests can drive httptest without waiting.
func (d *AssetDownloader) SetClientForTest(c *http.Client) {
	d.client = c
	d.fileClient = c
	d.rateLimit = 0
	d.retryDelay = time.Millisecond
	d.maxRetries = 1
}

// urlPathExt returns the file extension of the URL path (ignoring query/fragment).
func urlPathExt(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return filepath.Ext(rawURL)
	}
	return filepath.Ext(parsed.Path)
}

// existingByHashPrefix finds a non-empty file in destDir whose name starts with
// the 8-byte hex hash prefix (e.g. "a1b2c3d4.*"). Used to skip extensionless
// Downloads before the GET when the Content-Type-derived ext is unknown.
func (d *AssetDownloader) existingByHashPrefix(destDir, hashPrefix string) (string, bool) {
	matches, err := filepath.Glob(filepath.Join(destDir, hashPrefix+".*"))
	if err != nil || len(matches) == 0 {
		return "", false
	}
	var found []string
	for _, m := range matches {
		fi, err := os.Stat(m)
		if err == nil && fi.Size() > 0 {
			found = append(found, m)
		}
	}
	if len(found) == 0 {
		return "", false
	}
	if len(found) > 1 && d.log.Enabled() {
		d.log.Warnw("multiple existing assets for hash prefix; using first",
			"prefix", hashPrefix,
			"matches", found)
	}
	return found[0], true
}

func (d *AssetDownloader) Download(destDir, rawURL string) (string, error) {
	hash := sha256.Sum256([]byte(rawURL))
	hashPrefix := fmt.Sprintf("%x", hash[:8])
	ext := urlPathExt(rawURL)

	if ext != "" {
		destPath := filepath.Join(destDir, hashPrefix+ext)
		if d.shouldSkip(destPath) {
			if d.log.Enabled() {
				d.log.Debugw("asset already exists, skipping download",
					"url", rawURL,
					"path", destPath)
			}
			return destPath, nil
		}
	} else if path, ok := d.existingByHashPrefix(destDir, hashPrefix); ok {
		// existingByHashPrefix only returns non-empty files; still honor --force.
		d.mu.Lock()
		force := d.force
		d.mu.Unlock()
		if !force {
			if d.log.Enabled() {
				d.log.Debugw("asset already exists, skipping download",
					"url", rawURL,
					"path", path)
			}
			return path, nil
		}
	}

	var lastErr error

	for attempt := 0; attempt <= d.maxRetries; attempt++ {
		if attempt > 0 {
			retryDelay := time.Duration(attempt) * d.retryDelay
			if d.log.Enabled() {
				d.log.Warnw("retrying asset download",
					"url", rawURL,
					"attempt", attempt+1,
					"max_retries", d.maxRetries,
					"retry_delay_ms", retryDelay.Milliseconds())
			}
			time.Sleep(retryDelay)
		}

		d.rateLimitWait()

		resp, err := d.get(rawURL)
		if err != nil {
			lastErr = fmt.Errorf("request error: %w", err)
			if d.log.Enabled() {
				d.log.Warnw("asset download attempt failed: request error",
					"url", rawURL,
					"attempt", attempt+1,
					"error", err)
			}
			continue
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			resp.Body.Close()
			if d.log.Enabled() {
				d.log.Warnw("asset download attempt failed: non-200 status",
					"url", rawURL,
					"status", resp.StatusCode,
					"attempt", attempt+1)
			}
			if resp.StatusCode == http.StatusTooManyRequests {
				d.throttled(resp.Header, rawURL)
			} else if !retryableStatus(resp.StatusCode) {
				return "", fmt.Errorf("download %s: %w", rawURL, lastErr)
			}
			continue
		}

		if looksLikeLoginPage(resp) {
			resp.Body.Close()
			return "", fmt.Errorf("download %s: got an HTML page (not the asset) — session likely missing or expired", rawURL)
		}

		fileExt := ext
		if fileExt == "" {
			contentType := resp.Header.Get("Content-Type")
			switch {
			case strings.Contains(contentType, "image/png"):
				fileExt = ".png"
			case strings.Contains(contentType, "image/jpeg"):
				fileExt = ".jpg"
			case strings.Contains(contentType, "image/gif"):
				fileExt = ".gif"
			case strings.Contains(contentType, "image/webp"):
				fileExt = ".webp"
			default:
				fileExt = ".bin"
			}
		}

		destPath := filepath.Join(destDir, hashPrefix+fileExt)

		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			lastErr = fmt.Errorf("create asset dir: %w", err)
			if d.log.Enabled() {
				d.log.Errorw("asset download failed: directory creation",
					"path", filepath.Dir(destPath), "error", err)
			}
			continue
		}

		f, err := os.Create(destPath)
		if err != nil {
			resp.Body.Close()
			lastErr = fmt.Errorf("create asset file: %w", err)
			if d.log.Enabled() {
				d.log.Errorw("asset download failed: file creation",
					"path", destPath, "error", err)
			}
			continue
		}

		size, err := io.Copy(f, resp.Body)
		resp.Body.Close()
		f.Close()

		if err != nil {
			lastErr = fmt.Errorf("write asset: %w", err)
			if d.log.Enabled() {
				d.log.Errorw("asset download failed: write error",
					"url", rawURL, "error", err)
			}
			_ = os.Remove(destPath)
			continue
		}

		if d.log.Enabled() {
			d.log.Infow("asset downloaded successfully",
				"url", rawURL,
				"path", destPath,
				"bytes", size)
		}

		return destPath, nil
	}

	return "", fmt.Errorf("download %s: failed after %d attempts: %w", rawURL, d.maxRetries+1, lastErr)
}

// rateLimitWait blocks until at least rateLimit has elapsed since the last
// request, then records the current time as the last request.
func (d *AssetDownloader) rateLimitWait() {
	d.mu.Lock()
	sinceLast := time.Since(d.lastReq)
	if sinceLast < d.rateLimit {
		wait := d.rateLimit - sinceLast
		d.mu.Unlock()
		time.Sleep(wait)
	} else {
		d.mu.Unlock()
	}
	d.mu.Lock()
	d.lastReq = time.Now()
	d.mu.Unlock()
}

// DownloadAs fetches rawURL and writes it to the exact destPath (creating parent
// dirs), reusing the same retry and rate-limit policy as Download. Unlike
// Download, the caller controls the filename — used by the CSF converter, whose
// rules emit Markdown links to a known local path before download happens.
func (d *AssetDownloader) DownloadAs(destPath, rawURL string) error {
	if d.shouldSkip(destPath) {
		if d.log.Enabled() {
			d.log.Debugw("asset already exists, skipping download",
				"url", rawURL,
				"path", destPath)
		}
		return nil
	}

	var lastErr error
	for attempt := 0; attempt <= d.maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * d.retryDelay)
		}
		d.rateLimitWait()

		resp, err := d.get(rawURL)
		if err != nil {
			lastErr = fmt.Errorf("request error: %w", err)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			if resp.StatusCode == http.StatusTooManyRequests {
				d.throttled(resp.Header, rawURL)
			} else if !retryableStatus(resp.StatusCode) {
				return fmt.Errorf("download %s: %w", rawURL, lastErr)
			}
			continue
		}
		if looksLikeLoginPage(resp) {
			resp.Body.Close()
			return fmt.Errorf("download %s: got an HTML page (not the asset) — session likely missing or expired", rawURL)
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			resp.Body.Close()
			lastErr = fmt.Errorf("create asset dir: %w", err)
			continue
		}
		f, err := os.Create(destPath)
		if err != nil {
			resp.Body.Close()
			lastErr = fmt.Errorf("create asset file: %w", err)
			continue
		}
		_, err = io.Copy(f, resp.Body)
		resp.Body.Close()
		f.Close()
		if err != nil {
			_ = os.Remove(destPath)
			lastErr = fmt.Errorf("write asset: %w", err)
			continue
		}
		if d.log.Enabled() {
			d.log.Infow("asset downloaded", "url", rawURL, "path", destPath)
		}
		return nil
	}
	return fmt.Errorf("download %s: failed after %d attempts: %w", rawURL, d.maxRetries+1, lastErr)
}

func (d *AssetDownloader) RewriteURL(rawURL, assetBase string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		if d.log.Enabled() {
			d.log.Debugw("asset URL rewrite failed: parse error", "url", rawURL, "error", err)
		}
		return rawURL
	}

	if strings.HasPrefix(parsed.Path, "/download/attachments/") {
		ext := urlPathExt(rawURL)
		hash := sha256.Sum256([]byte(rawURL))
		return filepath.Join(assetBase, "attachments", fmt.Sprintf("%x%s", hash[:8], ext))
	}

	if strings.HasPrefix(parsed.Host, "confluence-attachments") ||
		strings.Contains(parsed.Path, "/plugins/attachments") {
		ext := urlPathExt(rawURL)
		if ext == "" {
			ext = ".bin"
		}
		hash := sha256.Sum256([]byte(rawURL))
		return filepath.Join(assetBase, "images", fmt.Sprintf("%x%s", hash[:8], ext))
	}

	return rawURL
}

// fileClient downloads page attachments: no overall timeout (files can be
// large), but a bounded wait for the response headers.
var fileClient = &http.Client{Transport: &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	ResponseHeaderTimeout: 60 * time.Second,
}}

// DownloadFile downloads a page attachment (pptx, pdf, xlsx, …) to destPath.
// An existing file of the expected size is kept (unless force); a changed
// size means a new version and is re-downloaded. The file is written next to
// destPath first and renamed, so an interrupted download leaves no partial
// file behind.
func (d *AssetDownloader) DownloadFile(destPath, rawURL string, size int64) error {
	d.mu.Lock()
	force := d.force
	client := d.fileClient
	d.mu.Unlock()
	if client == nil {
		client = fileClient
	}
	if fi, err := os.Stat(destPath); err == nil && !force && fi.Size() > 0 && (size <= 0 || fi.Size() == size) {
		return nil
	}
	var resp *http.Response
	for attempt := 1; ; attempt++ {
		d.rateLimitWait()
		req, err := http.NewRequest(http.MethodGet, rawURL, nil)
		if err != nil {
			return err
		}
		d.mu.Lock()
		for k, v := range d.authHeaders {
			req.Header.Set(k, v)
		}
		d.mu.Unlock()
		resp, err = client.Do(req)
		if err != nil {
			return fmt.Errorf("download %s: %w", rawURL, err)
		}
		if resp.StatusCode != http.StatusTooManyRequests || attempt >= ratelimit.MaxAttempts {
			break
		}
		resp.Body.Close()
		d.throttled(resp.Header, rawURL)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", rawURL, resp.StatusCode)
	}
	if ext := strings.ToLower(filepath.Ext(destPath)); ext != ".html" && ext != ".htm" && looksLikeLoginPage(resp) {
		return fmt.Errorf("download %s: got an HTML page (not the file) — session likely missing or expired", rawURL)
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	tmp := destPath + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("download %s: %w", rawURL, err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, destPath); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if d.log.Enabled() {
		d.log.Infow("attachment downloaded", "url", rawURL, "path", destPath)
	}
	return nil
}
