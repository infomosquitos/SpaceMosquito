package contentmd

import (
	"bytes"
	"fmt"
	"html/template"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

// mdRenderer renders content.md for the browse UI (ADR-018). Raw HTML in the
// Markdown is escaped (goldmark default); hard wraps keep CSF <br> line
// breaks, which the converter emits as single newlines.
var mdRenderer = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithRendererOptions(html.WithHardWraps()),
)

// RenderMarkdown converts page Markdown to an HTML fragment. content.md links
// downloaded assets by path relative to the page directory, so images show
// when the fragment is served from that directory's URL.
func RenderMarkdown(markdown string) (template.HTML, error) {
	var body bytes.Buffer
	if err := mdRenderer.Convert([]byte(markdown), &body); err != nil {
		return "", fmt.Errorf("render markdown: %w", err)
	}
	return template.HTML(body.String()), nil
}
