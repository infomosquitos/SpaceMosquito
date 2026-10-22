package csf

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// statusRule renders the status macro (a colored lozenge) as its title in
// inline code, upper-cased as Confluence displays it (e.g. `TESTED
// SUCCESSFULLY`; a typed "moderATE" shows as MODERATE there too). The colour
// param is dropped.
type statusRule struct{}

func (statusRule) Name() string { return "status" }

func (statusRule) Match(sel *goquery.Selection) bool {
	return tagName(sel) == "ac:structured-macro" && macroName(sel) == "status"
}

func (statusRule) Render(sel *goquery.Selection, _ *RenderContext) (string, error) {
	title := macroParam(sel, "title")
	if title == "" {
		return "", nil
	}
	return renderInlineCode(strings.ToUpper(title)), nil
}
