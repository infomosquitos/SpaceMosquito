package csf

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// adfRule renders ac:adf-extension, the storage form of newer editor blocks
// (Atlassian Document Format): decision lists and panels. Their
// ac:adf-attribute values (local-id GUIDs, state, panel-type) are metadata,
// never text. Unknown node types use the extension's ac:adf-fallback, the
// plain-HTML rendering Confluence stores alongside.
type adfRule struct{}

func (adfRule) Name() string { return "adf" }

func (adfRule) Match(sel *goquery.Selection) bool {
	switch tagName(sel) {
	case "ac:adf-extension", "ac:adf-attribute", "ac:adf-fallback":
		return true
	}
	return false
}

func (adfRule) Render(sel *goquery.Selection, ctx *RenderContext) (string, error) {
	if tagName(sel) != "ac:adf-extension" {
		// Attributes are metadata; a fallback is only used via its extension.
		return "", nil
	}
	if node := adfChild(sel, "ac:adf-node"); node != nil {
		if out := renderADFNode(node, ctx); strings.TrimSpace(out) != "" {
			return out, nil
		}
	}
	if fb := adfChild(sel, "ac:adf-fallback"); fb != nil {
		if out := strings.TrimSpace(ctx.RenderChildren(fb)); out != "" {
			return "\n\n" + out + "\n\n", nil
		}
	}
	return "", nil
}

// adfPanelLabels maps ADF panel types to callout labels ("" = no label).
var adfPanelLabels = map[string]string{
	"info": "Info", "note": "Note", "warning": "Warning", "tip": "Tip",
	"success": "Success", "error": "Error",
}

func renderADFNode(node *goquery.Selection, ctx *RenderContext) string {
	switch node.AttrOr("type", "") {
	case "decision-list":
		// One callout for the list, each decision on its own paragraph, as
		// Confluence shows a decision panel.
		var items []string
		node.Children().Each(func(_ int, item *goquery.Selection) {
			if tagName(item) != "ac:adf-node" || item.AttrOr("type", "") != "decision-item" {
				return
			}
			text := ""
			if c := adfChild(item, "ac:adf-content"); c != nil {
				text = strings.Join(strings.Fields(ctx.RenderChildren(c)), " ")
			}
			if text == "" {
				return
			}
			label := "Decision"
			if st := adfAttr(item, "state"); st != "" && !strings.EqualFold(st, "DECIDED") {
				label = "Decision (open)"
			}
			items = append(items, "**"+label+":** "+text)
		})
		return calloutBlock("", strings.Join(items, "\n\n"))
	case "panel":
		body := ""
		if c := adfChild(node, "ac:adf-content"); c != nil {
			body = ctx.RenderChildren(c)
		}
		return calloutBlock(adfPanelLabels[strings.ToLower(adfAttr(node, "panel-type"))], body)
	}
	return ""
}

// adfChild returns the first direct child element with the given tag.
func adfChild(sel *goquery.Selection, tag string) *goquery.Selection {
	var found *goquery.Selection
	sel.Children().EachWithBreak(func(_ int, ch *goquery.Selection) bool {
		if tagName(ch) == tag {
			found = ch
			return false
		}
		return true
	})
	return found
}

// adfAttr returns the value of a direct ac:adf-attribute child with key.
func adfAttr(node *goquery.Selection, key string) string {
	val := ""
	node.Children().EachWithBreak(func(_ int, ch *goquery.Selection) bool {
		if tagName(ch) == "ac:adf-attribute" && ch.AttrOr("key", "") == key {
			val = strings.TrimSpace(ch.Text())
			return false
		}
		return true
	})
	return val
}
