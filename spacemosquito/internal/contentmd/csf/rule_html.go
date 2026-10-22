package csf

import (
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

// htmlRule renders native HTML block/inline tags into Markdown: headings,
// paragraphs, emphasis, inline code, links, rules, line breaks, and lists.
// Tables (native <table>) and Confluence links (ac:link) have their own rules.
type htmlRule struct{}

func (htmlRule) Name() string { return "html_passthrough" }

var htmlInline = map[string]bool{
	"strong": true, "b": true, "em": true, "i": true, "code": true, "a": true,
}

func (htmlRule) Match(sel *goquery.Selection) bool {
	name := tagName(sel)
	switch name {
	case "h1", "h2", "h3", "h4", "h5", "h6", "p", "hr", "br", "ul", "ol":
		return true
	}
	return htmlInline[name]
}

func (htmlRule) Render(sel *goquery.Selection, ctx *RenderContext) (string, error) {
	name := tagName(sel)
	switch name {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		level := int(name[1] - '0')
		text := strings.TrimSpace(ctx.RenderChildren(sel))
		if text == "" {
			return "", nil
		}
		return "\n\n" + strings.Repeat("#", level) + " " + text + "\n\n", nil
	case "p":
		text := strings.TrimSpace(ctx.RenderChildren(sel))
		if text == "" {
			return "", nil
		}
		return "\n\n" + text + "\n\n", nil
	case "hr":
		return "\n\n---\n\n", nil
	case "br":
		return "\n", nil
	case "ul", "ol":
		return "\n\n" + renderList(sel, ctx) + "\n\n", nil
	case "strong", "b":
		return wrapInline(ctx.RenderChildren(sel), "**"), nil
	case "em", "i":
		return wrapInline(ctx.RenderChildren(sel), "*"), nil
	case "code":
		return renderInlineCode(ctx.RenderChildren(sel)), nil
	case "a":
		return renderAnchor(sel, ctx), nil
	}
	return ctx.RenderChildren(sel), nil
}

func wrapInline(inner, marker string) string {
	inner = strings.TrimSpace(inner)
	if inner == "" {
		return ""
	}
	return marker + inner + marker
}

func renderInlineCode(inner string) string {
	inner = strings.TrimSpace(inner)
	if inner == "" {
		return ""
	}
	// Use double backticks when the content itself contains a backtick.
	if strings.Contains(inner, "`") {
		return "`` " + inner + " ``"
	}
	return "`" + inner + "`"
}

func renderAnchor(sel *goquery.Selection, ctx *RenderContext) string {
	text := strings.TrimSpace(ctx.RenderChildren(sel))
	href := strings.TrimSpace(sel.AttrOr("href", ""))
	if href == "" || href == "#" {
		return text
	}
	if text == "" {
		text = href
	}
	return "[" + text + "](" + href + ")"
}

// renderList renders a ul/ol as Markdown list lines, indented per nesting depth
// (tracked on ctx.listDepth). Nested lists are handled inline (bypassing the
// registry) so indentation composes correctly.
func renderList(sel *goquery.Selection, ctx *RenderContext) string {
	ordered := tagName(sel) == "ol"
	depth := ctx.listDepth
	indent := strings.Repeat("  ", depth)

	var lines []string
	idx := 0
	// Confluence splits numbered steps around other content into separate
	// lists continued via <ol start="N">.
	if start, err := strconv.Atoi(sel.AttrOr("start", "")); ordered && err == nil && start > 0 {
		idx = start - 1
	}
	sel.Children().Each(func(_ int, li *goquery.Selection) {
		if tagName(li) != "li" {
			return
		}
		idx++
		marker := "- "
		if ordered {
			marker = strconv.Itoa(idx) + ". "
		}
		inline, nested := renderListItem(li, ctx, depth)
		lines = append(lines, strings.TrimRight(indent+marker+inline, " "))
		if nested != "" {
			lines = append(lines, nested)
		}
	})
	return strings.Join(lines, "\n")
}

// renderListItem splits an <li> into its inline content (first line) and any
// nested lists (following, already-indented lines).
func renderListItem(li *goquery.Selection, ctx *RenderContext, depth int) (inline, nested string) {
	var ib strings.Builder
	var nb []string
	ctx.listDepth = depth + 1
	li.Contents().Each(func(_ int, s *goquery.Selection) {
		n := s.Get(0)
		if n == nil {
			return
		}
		switch {
		case n.Type == html.TextNode:
			ib.WriteString(whitespace.ReplaceAllString(n.Data, " "))
		case n.Type == html.ElementNode:
			switch tagName(s) {
			case "ul", "ol":
				nb = append(nb, renderList(s, ctx))
			case "p", "div":
				// Unwrap block wrappers inside list items to keep the item on
				// one logical line.
				ib.WriteString(strings.TrimSpace(ctx.RenderChildren(s)) + " ")
			default:
				if rule, ok := ctx.registry.Match(s); ok {
					if md, err := rule.Render(s, ctx); err == nil {
						ib.WriteString(md)
					}
				} else {
					ib.WriteString(defaultRender(s, ctx))
				}
			}
		}
	})
	ctx.listDepth = depth
	return strings.TrimSpace(ib.String()), strings.Join(nb, "\n")
}
