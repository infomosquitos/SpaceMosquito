package csf

import (
	"strings"
	"testing"
)

func TestJira_keyRef(t *testing.T) {
	in := `<p><ac:structured-macro ac:name="jira"><ac:parameter ac:name="server">Some-Jira</ac:parameter><ac:parameter ac:name="key">PROJ-1400</ac:parameter></ac:structured-macro></p>`
	got := md(t, in)
	if !strings.Contains(got, "**JIRA:** PROJ-1400") {
		t.Fatalf("jira ref wrong: %q", got)
	}
	if strings.Contains(got, "Some-Jira") {
		t.Fatalf("server param leaked: %q", got)
	}
}

func TestJira_noKeyDropped(t *testing.T) {
	in := `<ac:structured-macro ac:name="jira"><ac:parameter ac:name="jqlQuery">project = PROJ</ac:parameter></ac:structured-macro>`
	got := md(t, in)
	if strings.Contains(got, "JIRA") || strings.Contains(got, "project = PROJ") {
		t.Fatalf("keyless jira should be dropped: %q", got)
	}
}

func TestStatus_titleInlineCode(t *testing.T) {
	in := `<p>Result: <ac:structured-macro ac:name="status"><ac:parameter ac:name="colour">Green</ac:parameter><ac:parameter ac:name="title">Tested successfully</ac:parameter></ac:structured-macro></p>`
	got := md(t, in)
	if !strings.Contains(got, "`TESTED SUCCESSFULLY`") { // upper-cased as Confluence shows it
		t.Fatalf("status title not inline code: %q", got)
	}
	if strings.Contains(got, "Green") {
		t.Fatalf("colour leaked: %q", got)
	}
}

func TestPanel_infoCallout(t *testing.T) {
	in := `<ac:structured-macro ac:name="info"><ac:rich-text-body><p>Heads up here</p></ac:rich-text-body></ac:structured-macro>`
	got := md(t, in)
	if !strings.Contains(got, "> **Info:**") {
		t.Fatalf("info label missing: %q", got)
	}
	if !strings.Contains(got, "> Heads up here") {
		t.Fatalf("info body not quoted: %q", got)
	}
}

func TestPanel_warningNoParamNoise(t *testing.T) {
	in := `<ac:structured-macro ac:name="warning"><ac:parameter ac:name="borderStyle">solid</ac:parameter><ac:parameter ac:name="borderWidth">1</ac:parameter><ac:rich-text-body><p>Careful</p></ac:rich-text-body></ac:structured-macro>`
	got := md(t, in)
	if !strings.Contains(got, "> **Warning:**") || !strings.Contains(got, "Careful") {
		t.Fatalf("warning callout wrong: %q", got)
	}
	if strings.Contains(got, "1solid") || strings.Contains(got, "solid") {
		t.Fatalf("border params leaked: %q", got)
	}
}

func TestPanel_plainIsTransparentNotQuoted(t *testing.T) {
	// A generic panel is a container, not an admonition: title as a bold line,
	// body as normal content, NO blockquote (so page-spanning panels don't quote
	// the whole document).
	in := `<ac:structured-macro ac:name="panel"><ac:parameter ac:name="title">My Box</ac:parameter><ac:rich-text-body><p>content</p></ac:rich-text-body></ac:structured-macro>`
	got := md(t, in)
	if !strings.Contains(got, "**My Box**") || !strings.Contains(got, "content") {
		t.Fatalf("panel title/body wrong: %q", got)
	}
	if strings.Contains(got, ">") {
		t.Fatalf("plain panel should not be a blockquote: %q", got)
	}
}

func TestPanel_pageSpanningNotQuoted(t *testing.T) {
	// The real-world case: a whole report wrapped in one panel must not become
	// an all-blockquote document.
	in := `<ac:structured-macro ac:name="panel"><ac:rich-text-body>` +
		`<h2>Section</h2><p>Body text</p>` +
		`<ac:structured-macro ac:name="note"><ac:rich-text-body><p>a note</p></ac:rich-text-body></ac:structured-macro>` +
		`</ac:rich-text-body></ac:structured-macro>`
	got := md(t, in)
	if !strings.Contains(got, "## Section") || !strings.Contains(got, "Body text") {
		t.Fatalf("panel body not passed through: %q", got)
	}
	// The section heading/body must be unquoted; only the inner note is quoted.
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "## Section") && strings.HasPrefix(strings.TrimSpace(line), ">") {
			t.Fatalf("panel content wrongly quoted: %q", got)
		}
	}
	if !strings.Contains(got, "> **Note:**") {
		t.Fatalf("inner note should still be a callout: %q", got)
	}
}

func TestExpand_titleAndBody(t *testing.T) {
	in := `<ac:structured-macro ac:name="expand"><ac:parameter ac:name="title">More info</ac:parameter><ac:rich-text-body><p>hidden content</p></ac:rich-text-body></ac:structured-macro>`
	got := md(t, in)
	if !strings.Contains(got, "**More info**") || !strings.Contains(got, "hidden content") {
		t.Fatalf("expand render wrong: %q", got)
	}
}

func TestToc_dropped(t *testing.T) {
	in := `<p>before</p><ac:structured-macro ac:name="toc"><ac:parameter ac:name="maxLevel">3</ac:parameter></ac:structured-macro><p>after</p>`
	got := md(t, in)
	if strings.Contains(got, "maxLevel") || strings.Contains(got, "3") {
		t.Fatalf("toc params leaked: %q", got)
	}
	if !strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Fatalf("surrounding content lost: %q", got)
	}
}

func TestTaskList_checkboxes(t *testing.T) {
	in := `<ac:task-list>
		<ac:task><ac:task-id>1</ac:task-id><ac:task-status>complete</ac:task-status><ac:task-body>done item</ac:task-body></ac:task>
		<ac:task><ac:task-id>2</ac:task-id><ac:task-status>incomplete</ac:task-status><ac:task-body>todo item</ac:task-body></ac:task>
	</ac:task-list>`
	got := md(t, in)
	if !strings.Contains(got, "- [x] done item") {
		t.Fatalf("completed task wrong: %q", got)
	}
	if !strings.Contains(got, "- [ ] todo item") {
		t.Fatalf("incomplete task wrong: %q", got)
	}
	// task metadata must not leak.
	for _, noise := range []string{"complete\n", "incomplete\n"} {
		if strings.Contains(got, noise) {
			t.Fatalf("task-status text leaked: %q", got)
		}
	}
}

// On Confluence Cloud Jira is on the same site, so keys and queries link.
func TestJira_cloudLinks(t *testing.T) {
	cloud := &RenderContext{BaseURL: "https://acme.atlassian.net", Cloud: true}
	got, _ := convert(t, `<p><ac:structured-macro ac:name="jira"><ac:parameter ac:name="key">CCP-658</ac:parameter><ac:parameter ac:name="serverId">8b67</ac:parameter></ac:structured-macro></p>`, cloud)
	if want := "**JIRA:** [CCP-658](https://acme.atlassian.net/browse/CCP-658)"; strings.TrimSpace(got) != want {
		t.Errorf("key: got %q, want %q", strings.TrimSpace(got), want)
	}
	got, _ = convert(t, `<ac:structured-macro ac:name="jira"><ac:parameter ac:name="jqlQuery">labels = "W07"</ac:parameter></ac:structured-macro>`, cloud)
	if want := `**JIRA:** [labels = "W07"](https://acme.atlassian.net/issues/?jql=labels+%3D+%22W07%22)`; strings.TrimSpace(got) != want {
		t.Errorf("jql: got %q, want %q", strings.TrimSpace(got), want)
	}
	// Server/DC: the Jira URL is unknown, the key stays text.
	server := &RenderContext{BaseURL: "https://wiki.example.net"}
	if got, _ := convert(t, `<p><ac:structured-macro ac:name="jira"><ac:parameter ac:name="key">PROJ-1</ac:parameter></ac:structured-macro></p>`, server); strings.TrimSpace(got) != "**JIRA:** PROJ-1" {
		t.Errorf("server: %q", got)
	}
}
