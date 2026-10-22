package contentmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

// IndexFileName is the browsable per-page HTML artifact per ADR-015.
const IndexFileName = "index.html"

// mdRenderer renders content.md for offline viewing. Raw HTML in the Markdown
// is escaped (goldmark default); hard wraps keep CSF <br> line breaks, which
// the converter emits as single newlines.
var mdRenderer = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithRendererOptions(html.WithHardWraps()),
)

var indexTemplate = template.Must(template.New("index").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
:root { color-scheme: light dark; }
body { max-width: 960px; margin: 0 auto; padding: 24px 16px; font: 15px/1.6 -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; }
header { border-bottom: 1px solid #8884; margin-bottom: 24px; }
nav { font-size: 13px; }
img { max-width: 100%; height: auto; }
table { border-collapse: collapse; display: block; overflow-x: auto; }
th, td { border: 1px solid #8886; padding: 4px 8px; vertical-align: top; }
pre { padding: 12px; overflow-x: auto; background: #8881; border-radius: 4px; }
code { font: 13px/1.5 ui-monospace, SFMono-Regular, Menlo, monospace; }
blockquote { margin: 0; padding: 0 16px; border-left: 4px solid #8886; }
</style>
</head>
<body>
<header>
<nav><a href="../index.html">← {{.SpaceKey}} index</a>{{if .SourceURL}} · <a href="{{.SourceURL}}">View in Confluence</a>{{end}}</nav>
<h1>{{.Title}}</h1>
</header>
<main>
{{.Body}}
</main>
</body>
</html>
`))

// MarkdownToHTML renders page Markdown as a standalone HTML document. It is
// used for Storage Format pages, whose raw body browsers cannot display;
// content.md already links downloaded assets by relative path, so images and
// diagrams show when index.html is opened from the page directory. The header
// links to the space index one directory up (see internal/catalogindex).
func MarkdownToHTML(spaceKey, title, sourceURL, markdown string) (string, error) {
	var body bytes.Buffer
	if err := mdRenderer.Convert([]byte(markdown), &body); err != nil {
		return "", fmt.Errorf("render markdown: %w", err)
	}
	var out bytes.Buffer
	err := indexTemplate.Execute(&out, struct {
		SpaceKey  string
		Title     string
		SourceURL string
		Body      template.HTML
	}{spaceKey, title, sourceURL, template.HTML(body.String())})
	if err != nil {
		return "", fmt.Errorf("render index html: %w", err)
	}
	return out.String(), nil
}

// WriteIndexHTML renders markdown into index.html in a page directory.
func WriteIndexHTML(dir, title, sourceURL, markdown string) error {
	if strings.TrimSpace(dir) == "" {
		return fmt.Errorf("empty page directory")
	}
	// Page dirs live at {saved}/{spaceKey}/{page}.
	spaceKey := filepath.Base(filepath.Dir(filepath.Clean(dir)))
	doc, err := MarkdownToHTML(spaceKey, title, sourceURL, markdown)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, IndexFileName), []byte(doc), 0644)
}

// RefreshDirIndexHTML regenerates index.html from markdown for Storage Format
// page directories (offline reindex/import), taking title and source URL from
// metadata.json. Rendered-HTML pages keep their extracted index.html, which is
// also the input for their Markdown. Returns whether index.html was written.
func RefreshDirIndexHTML(dir, markdown string) (bool, error) {
	if DetectBodyFormat(dir) != "storage" {
		return false, nil
	}
	var meta struct {
		Title         string `json:"title"`
		ConfluenceURL string `json:"confluence_url"`
	}
	if b, err := os.ReadFile(filepath.Join(dir, "metadata.json")); err == nil {
		_ = json.Unmarshal(b, &meta)
	}
	if meta.Title == "" {
		meta.Title = filepath.Base(dir)
	}
	if err := WriteIndexHTML(dir, meta.Title, meta.ConfluenceURL, markdown); err != nil {
		return false, err
	}
	return true, nil
}
