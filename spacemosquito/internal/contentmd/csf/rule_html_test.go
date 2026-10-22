package csf

import (
	"strings"
	"testing"
)

func TestHTML_headings(t *testing.T) {
	got := md(t, `<h2>Section Title</h2><h3>Sub</h3>`)
	if !strings.Contains(got, "## Section Title") {
		t.Fatalf("h2 not rendered: %q", got)
	}
	if !strings.Contains(got, "### Sub") {
		t.Fatalf("h3 not rendered: %q", got)
	}
}

func TestHTML_emphasisAndCode(t *testing.T) {
	got := md(t, `<p>a <strong>bold</strong> and <em>italic</em> and <code>x=1</code></p>`)
	for _, want := range []string{"**bold**", "*italic*", "`x=1`"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
}

func TestHTML_anchor(t *testing.T) {
	got := md(t, `<p>See <a href="https://example.com/x">the docs</a>.</p>`)
	if !strings.Contains(got, "[the docs](https://example.com/x)") {
		t.Fatalf("anchor not rendered: %q", got)
	}
}

func TestHTML_anchorNoHrefFallsBackToText(t *testing.T) {
	got := md(t, `<p><a href="#">anchor text</a></p>`)
	if strings.Contains(got, "](") {
		t.Fatalf("expected plain text for empty href, got %q", got)
	}
	if !strings.Contains(got, "anchor text") {
		t.Fatalf("anchor text lost: %q", got)
	}
}

func TestHTML_unorderedList(t *testing.T) {
	got := md(t, `<ul><li>one</li><li>two</li></ul>`)
	if !strings.Contains(got, "- one") || !strings.Contains(got, "- two") {
		t.Fatalf("list items not rendered: %q", got)
	}
}

func TestHTML_orderedList(t *testing.T) {
	got := md(t, `<ol><li>first</li><li>second</li></ol>`)
	if !strings.Contains(got, "1. first") || !strings.Contains(got, "2. second") {
		t.Fatalf("ordered list not rendered: %q", got)
	}
}

func TestHTML_orderedListStart(t *testing.T) {
	got := md(t, `<ol start="2"><li>second</li><li>third</li></ol>`)
	if !strings.Contains(got, "2. second") || !strings.Contains(got, "3. third") {
		t.Fatalf("ol start not honored: %q", got)
	}
}

func TestHTML_nestedList(t *testing.T) {
	got := md(t, `<ul><li>parent<ul><li>child</li></ul></li></ul>`)
	if !strings.Contains(got, "- parent") {
		t.Fatalf("parent item missing: %q", got)
	}
	if !strings.Contains(got, "  - child") {
		t.Fatalf("nested child not indented: %q", got)
	}
}

func TestHTML_horizontalRule(t *testing.T) {
	got := md(t, `<p>above</p><hr/><p>below</p>`)
	if !strings.Contains(got, "---") {
		t.Fatalf("hr not rendered: %q", got)
	}
}
