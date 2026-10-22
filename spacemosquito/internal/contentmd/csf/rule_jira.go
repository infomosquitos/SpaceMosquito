package csf

import (
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// jiraRule renders the jira macro: an issue key as a bold ref, or a JQL query
// (issue table). The macro carries a server name / UUID, not a URL; on
// Confluence Cloud Jira lives on the same site, so keys link to
// <site>/browse/KEY and queries to <site>/issues/?jql=…. On Server/DC the
// Jira URL is unknown: keys stay text and query tables are dropped.
// server/columns params are dropped.
type jiraRule struct{}

func (jiraRule) Name() string { return "jira" }

func (jiraRule) Match(sel *goquery.Selection) bool {
	return tagName(sel) == "ac:structured-macro" && macroName(sel) == "jira"
}

func (jiraRule) Render(sel *goquery.Selection, ctx *RenderContext) (string, error) {
	site := ""
	if ctx != nil && ctx.Cloud && ctx.BaseURL != "" {
		site = strings.TrimSuffix(ctx.BaseURL, "/")
	}
	if key := macroParam(sel, "key"); key != "" {
		if site == "" {
			return "**JIRA:** " + key, nil
		}
		return "**JIRA:** [" + key + "](" + site + "/browse/" + url.PathEscape(key) + ")", nil
	}
	if jql := macroParam(sel, "jqlQuery"); jql != "" && site != "" {
		return "**JIRA:** [" + escapeLinkText(jql) + "](" + site + "/issues/?jql=" + url.QueryEscape(jql) + ")", nil
	}
	return "", nil
}
