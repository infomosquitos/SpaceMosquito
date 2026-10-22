package csf

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

// linkRule renders ac:link into Markdown. The label comes from ac:link-body
// (rich) or ac:plain-text-link-body (CDATA — often lost to HTML parsing, so a
// target-derived fallback is used). The target is one of the ri:* refs.
//
// Phase 1 emits a real Markdown link only for ri:url (a known external target).
// Page/attachment/user/space refs cannot be resolved to a local/remote URL yet
// (that arrives with asset handling and page resolution in later phases), so
// they render as their label text — never a broken link.
type linkRule struct{}

func (linkRule) Name() string { return "link" }

func (linkRule) Match(sel *goquery.Selection) bool {
	return tagName(sel) == "ac:link"
}

func (linkRule) Render(sel *goquery.Selection, ctx *RenderContext) (string, error) {
	label := linkLabel(sel, ctx)

	out := label
	sel.Children().EachWithBreak(func(_ int, ch *goquery.Selection) bool {
		switch tagName(ch) {
		case "ri:url":
			v := strings.TrimSpace(ch.AttrOr("ri:value", ""))
			if label == "" {
				label = v
			}
			if v != "" {
				out = "[" + label + "](" + v + ")"
			} else {
				out = label
			}
			return false
		case "ri:page":
			out = firstNonEmpty(label, ch.AttrOr("ri:content-title", ""))
			return false
		case "ri:user":
			u := firstNonEmpty(ch.AttrOr("ri:username", ""), ch.AttrOr("ri:userkey", ""))
			if label != "" {
				out = label
			} else if u != "" {
				out = "@" + u
			} else {
				out = ""
			}
			return false
		case "ri:attachment":
			name := ch.AttrOr("ri:filename", "")
			out = firstNonEmpty(label, name)
			// A file attachment links its saved copy (images are inline).
			if f, ok := ctx.Files[name]; ok && f.Href != "" && out != "" {
				out = fmt.Sprintf("[%s](<%s>)", out, f.Href)
			}
			return false
		case "ri:space":
			out = firstNonEmpty(label, ch.AttrOr("ri:space-key", ""))
			return false
		}
		return true
	})
	if out != "" && sel.AttrOr("ac:card-appearance", "") != "" {
		out = separateCard(sel, out)
	}
	return out, nil
}

// separateCard adds a space between a smart link and adjacent text. Confluence
// shows such links as cards (inline chips), so the source often has no space
// around them: <ac:link>…SAPCC</ac:link>for SAP.
func separateCard(sel *goquery.Selection, out string) string {
	n := sel.Get(0)
	if next := n.NextSibling; next != nil && next.Type == html.TextNode && startsWord(next.Data) {
		out += " "
	}
	if prev := n.PrevSibling; prev != nil && prev.Type == html.TextNode && endsWord(prev.Data) {
		out = " " + out
	}
	return out
}

func startsWord(s string) bool {
	r, _ := utf8.DecodeRuneInString(s)
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

func endsWord(s string) bool {
	r, _ := utf8.DecodeLastRuneInString(s)
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// linkLabel extracts the link label from a body element. It searches
// descendants (not just direct children) because a self-closing ri:* ref makes
// the HTML parser mis-nest the body inside the ref element.
func linkLabel(sel *goquery.Selection, ctx *RenderContext) string {
	if body := findDescendant(sel, "ac:link-body"); body != nil {
		if l := strings.TrimSpace(ctx.RenderChildren(body)); l != "" {
			return l
		}
	}
	if body := findDescendant(sel, "ac:plain-text-link-body"); body != nil {
		if l := strings.TrimSpace(body.Text()); l != "" {
			return l
		}
	}
	return ""
}

// findDescendant returns the first descendant of sel with the given tag name,
// or nil. Avoids CSS selectors so namespaced (colon) tag names work.
func findDescendant(sel *goquery.Selection, tag string) *goquery.Selection {
	var found *goquery.Selection
	sel.Find("*").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		if tagName(s) == tag {
			found = s
			return false
		}
		return true
	})
	return found
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}
