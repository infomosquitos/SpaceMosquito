package csf

import "github.com/PuerkitoBio/goquery"

// Registry is an ordered, first-match-wins set of rules.
type Registry struct {
	rules []Rule
}

// Register appends a rule. Order matters: register specific rules before
// generic ones, and the fallback rule last.
func (r *Registry) Register(rule Rule) {
	r.rules = append(r.rules, rule)
}

// Match returns the first rule whose Match reports true.
func (r *Registry) Match(sel *goquery.Selection) (Rule, bool) {
	for _, rule := range r.rules {
		if rule.Match(sel) {
			return rule, true
		}
	}
	return nil, false
}

// DefaultRegistry returns the current rule set, ordered specific → generic with
// the structured-macro fallback last:
//
//	strip, layout                     — noise removal + structural passthrough
//	code, drawio                      — code macro + <pre>; drawio diagrams
//	image, emoticon                   — ac:image assets + emoticons
//	file, attachments                 — file previews + attachment list
//	jira, status, panel, expand, toc  — high-volume macros
//	adf                               — ADF blocks (decisions, panels)
//	tasklist                          — ac:task-list checkboxes
//	link, table, html                 — Confluence links, GFM tables, native HTML
//	fallback                          — any remaining ac:structured-macro
func DefaultRegistry() *Registry {
	r := &Registry{}
	r.Register(stripRule{})
	r.Register(layoutRule{})
	r.Register(codeRule{})
	r.Register(drawioRule{})
	r.Register(imageRule{})
	r.Register(emoticonRule{})
	r.Register(fileRule{})
	r.Register(attachmentsRule{})
	r.Register(jiraRule{})
	r.Register(statusRule{})
	r.Register(panelRule{})
	r.Register(expandRule{})
	r.Register(tocRule{})
	r.Register(adfRule{})
	r.Register(tasklistRule{})
	r.Register(linkRule{})
	r.Register(tableRule{})
	r.Register(htmlRule{})
	r.Register(fallbackRule{})
	return r
}
