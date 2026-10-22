package csf

import (
	"fmt"
	"sort"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// fileRule renders file preview macros (view-file on Cloud; viewpdf, viewppt,
// viewxls, viewdoc on older Server/DC) as a link to the attachment. The link
// title "attachment" lets the browse UI show PDFs inline.
type fileRule struct{}

func (fileRule) Name() string { return "file" }

var fileMacros = map[string]bool{"view-file": true, "viewpdf": true, "viewppt": true, "viewxls": true, "viewdoc": true}

func (fileRule) Match(sel *goquery.Selection) bool {
	return tagName(sel) == "ac:structured-macro" && fileMacros[macroName(sel)]
}

func (fileRule) Render(sel *goquery.Selection, ctx *RenderContext) (string, error) {
	att := findDescendant(sel, "ri:attachment")
	if att == nil {
		return "", nil
	}
	name := strings.TrimSpace(att.AttrOr("ri:filename", ""))
	if name == "" {
		return "", nil
	}
	return "\n\n" + fileMarkdown(ctx, name, name) + "\n\n", nil
}

// attachmentsRule renders the attachments macro (Confluence lists every file
// of the page) as a list of the page's file attachments.
type attachmentsRule struct{}

func (attachmentsRule) Name() string { return "attachments" }

func (attachmentsRule) Match(sel *goquery.Selection) bool {
	return tagName(sel) == "ac:structured-macro" && macroName(sel) == "attachments"
}

func (attachmentsRule) Render(_ *goquery.Selection, ctx *RenderContext) (string, error) {
	if len(ctx.Files) == 0 {
		return "", nil
	}
	names := make([]string, 0, len(ctx.Files))
	for n := range ctx.Files {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })
	var b strings.Builder
	b.WriteString("\n\n")
	for _, n := range names {
		fmt.Fprintf(&b, "- %s", fileMarkdown(ctx, n, n))
		if sz := ctx.Files[n].Size; sz > 0 {
			fmt.Fprintf(&b, " (%s)", FormatSize(sz))
		}
		b.WriteString("\n")
	}
	return b.String() + "\n", nil
}

// fileMarkdown links a page attachment by filename: 📎 [label](<href> "attachment"),
// or the plain label when the file is unknown.
func fileMarkdown(ctx *RenderContext, filename, label string) string {
	f, ok := ctx.Files[filename]
	if !ok || f.Href == "" {
		return "📎 " + escapeLinkText(label)
	}
	return fmt.Sprintf("📎 [%s](<%s> \"attachment\")", escapeLinkText(label), f.Href)
}

var linkTextEscaper = strings.NewReplacer(`\`, `\\`, `[`, `\[`, `]`, `\]`, `<`, `\<`, "*", `\*`, "_", `\_`, "`", "\\`")

func escapeLinkText(s string) string { return linkTextEscaper.Replace(s) }

// FormatSize renders a byte count like "2.4 MB".
func FormatSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
