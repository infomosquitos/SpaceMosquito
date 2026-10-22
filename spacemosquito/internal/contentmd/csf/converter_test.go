package csf

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// md is a test helper: convert a CSF fragment with the default registry.
func md(t *testing.T, in string) string {
	t.Helper()
	out, _, err := CSFToMarkdown(in, nil)
	if err != nil {
		t.Fatalf("CSFToMarkdown(%q): %v", in, err)
	}
	return out
}

func TestFallback_dropsMacroParameters(t *testing.T) {
	in := `<ac:structured-macro ac:name="panel">
		<ac:parameter ac:name="borderStyle">solid</ac:parameter>
		<ac:parameter ac:name="borderWidth">1</ac:parameter>
		<ac:rich-text-body><p>Panel body text</p></ac:rich-text-body>
	</ac:structured-macro>`
	got := md(t, in)

	if !strings.Contains(got, "Panel body text") {
		t.Fatalf("missing body text in %q", got)
	}
	for _, noise := range []string{"solid", "borderStyle", "borderWidth"} {
		if strings.Contains(got, noise) {
			t.Fatalf("parameter noise %q leaked in %q", noise, got)
		}
	}
	// The canonical "1solid" concatenation must not appear.
	if strings.Contains(got, "1solid") {
		t.Fatalf("classic noise token '1solid' present in %q", got)
	}
}

func TestFallback_preservesBodyAfterSelfClosingParameter(t *testing.T) {
	// A self-closing <ac:parameter/> makes the HTML parser mis-nest the body
	// inside it; the body must still survive. Uses an unmodelled macro so it
	// exercises the fallback rule.
	in := `<ac:structured-macro ac:name="someunknownmacro">
		<ac:parameter ac:name="width" />
		<ac:parameter ac:name="opt">x</ac:parameter>
		<ac:rich-text-body><p>Nested body survives</p></ac:rich-text-body>
	</ac:structured-macro>`
	got := md(t, in)
	if !strings.Contains(got, "Nested body survives") {
		t.Fatalf("body lost after self-closing parameter: %q", got)
	}
}

func TestLayout_transparentPassthrough(t *testing.T) {
	in := `<ac:layout><ac:layout-section ac:type="two_equal">
		<ac:layout-cell><p>Left cell</p></ac:layout-cell>
		<ac:layout-cell><p>Right cell</p></ac:layout-cell>
	</ac:layout-section></ac:layout>`
	got := md(t, in)
	if !strings.Contains(got, "Left cell") || !strings.Contains(got, "Right cell") {
		t.Fatalf("layout content lost: %q", got)
	}
}

func TestStrip_placeholderAndCommentMarker(t *testing.T) {
	in := `<p>Before <ac:placeholder>ghost text</ac:placeholder>after ` +
		`<ac:inline-comment-marker ac:ref="abc">kept comment</ac:inline-comment-marker> end</p>`
	got := md(t, in)
	if strings.Contains(got, "ghost text") {
		t.Fatalf("placeholder not stripped: %q", got)
	}
	if !strings.Contains(got, "kept comment") {
		t.Fatalf("inline-comment-marker text lost: %q", got)
	}
	if !strings.Contains(got, "Before") || !strings.Contains(got, "end") {
		t.Fatalf("surrounding text lost: %q", got)
	}
}

func TestBlockSeparation_noMergedWords(t *testing.T) {
	in := `<p>orders are defined in acquisition definition</p><p>No Articles are given</p>`
	got := md(t, in)
	if strings.Contains(got, "definitionNo") {
		t.Fatalf("adjacent paragraphs merged: %q", got)
	}
	if !strings.Contains(got, "definition") || !strings.Contains(got, "No Articles") {
		t.Fatalf("expected words missing: %q", got)
	}
}

func TestEmptyInput(t *testing.T) {
	if got := md(t, ""); got != "" {
		t.Fatalf("expected empty output, got %q", got)
	}
	if got := md(t, "   \n  "); got != "" {
		t.Fatalf("expected empty output for whitespace, got %q", got)
	}
}

func TestMalformedCSF_doesNotPanic(t *testing.T) {
	// Truncated/unbalanced input must not panic; output is best-effort.
	inputs := []string{
		`<ac:structured-macro ac:name="panel"><ac:rich-text-body><p>hi`,
		`<ac:layout><ac:layout-cell><table><tr><td>x`,
		`<<>><ac:parameter`,
	}
	for _, in := range inputs {
		_ = md(t, in) // absence of panic is the assertion
	}
}

func TestRegistry_firstMatchWins(t *testing.T) {
	reg := DefaultRegistry()
	parse := func(fragment string) *goquery.Selection {
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(fragment))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		return doc.Find("body").Children().First()
	}

	cases := map[string]string{
		`<ac:parameter ac:name="x">v</ac:parameter>`:  "strip",
		`<ac:layout><p>x</p></ac:layout>`:             "layout",
		`<ac:structured-macro ac:name="jira"/>`:       "jira",
		`<ac:structured-macro ac:name="children"/>`:   "fallback",
	}
	for fragment, wantRule := range cases {
		sel := parse(fragment)
		rule, ok := reg.Match(sel)
		if !ok {
			t.Fatalf("no rule matched %q", fragment)
		}
		if rule.Name() != wantRule {
			t.Fatalf("%q matched %q, want %q", fragment, rule.Name(), wantRule)
		}
	}
}

// Storage Format writes empty paragraphs as <p … />; parsed as HTML they
// stayed open and pushed a following panel's text out of the panel.
func TestSelfClosingParagraphKeepsPanelContent(t *testing.T) {
	in := `<hr /><p local-id="a" /><ac:structured-macro ac:name="info"><ac:rich-text-body><p local-id="b">Setup for <strong>SAP</strong>.</p></ac:rich-text-body></ac:structured-macro><p local-id="c" /><p>After.</p>`
	got, _, err := CSFToMarkdown(in, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "> **Info:**\n>\n> Setup for **SAP**.") {
		t.Fatalf("panel text outside the panel:\n%s", got)
	}
	if !strings.Contains(got, "\n\nAfter.") || strings.Contains(got, "> After.") {
		t.Fatalf("text after the panel:\n%s", got)
	}
}

func TestExpandSelfClosing(t *testing.T) {
	in := `<p local-id="x" /><th/><br/><hr /><ri:attachment ri:filename="a.png" /><ac:emoticon ac:name="tick" />`
	want := `<p local-id="x"></p><th></th><br/><hr /><ri:attachment ri:filename="a.png" /><ac:emoticon ac:name="tick" />`
	if got := expandSelfClosing(in); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

// Block images and link cards separated only by empty paragraphs stay on
// their own lines; image captions are kept.
func TestBlockImagesCardsAndCaptions(t *testing.T) {
	in := `<p>Head</p><ac:image ac:layout="center" ac:alt="a.png"><ri:attachment ri:filename="a.png" /></ac:image><p /><ac:image ac:layout="center" ac:alt="b.png"><ri:attachment ri:filename="b.png" /><ac:caption><p>Mobile layout, expanded</p></ac:caption></ac:image><p />` +
		`<a href="https://x/browse/C-1" data-card-appearance="embed">https://x/browse/C-1</a><p> </p><p /><a href="https://x/issues" data-card-appearance="block">https://x/issues</a>` +
		`<p>Inline <ac:image ac:alt="c.png"><ri:attachment ri:filename="c.png" /></ac:image> image</p>` +
		`<table><tbody><tr><td><ac:image ac:alt="d.png"><ri:attachment ri:filename="d.png" /></ac:image></td><td>x</td></tr></tbody></table>`
	got, _, err := CSFToMarkdown(in, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"![a.png](assets/images/a.png)\n\n![b.png](assets/images/b.png)\n\n*Mobile layout, expanded*",
		"[https://x/browse/C-1](https://x/browse/C-1)\n\n[https://x/issues](https://x/issues)",
		"Inline ![c.png](assets/images/c.png) image",
		"| ![d.png](assets/images/d.png) | x |",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}
