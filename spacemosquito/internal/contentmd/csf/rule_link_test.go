package csf

import (
	"strings"
	"testing"
)

func TestLink_url(t *testing.T) {
	got := md(t, `<p><ac:link><ri:url ri:value="https://example.com/p"/><ac:link-body>External</ac:link-body></ac:link></p>`)
	if !strings.Contains(got, "[External](https://example.com/p)") {
		t.Fatalf("url link not rendered: %q", got)
	}
}

func TestLink_urlWithoutLabelUsesURL(t *testing.T) {
	got := md(t, `<p><ac:link><ri:url ri:value="https://example.com/p"/></ac:link></p>`)
	if !strings.Contains(got, "[https://example.com/p](https://example.com/p)") {
		t.Fatalf("url label fallback failed: %q", got)
	}
}

func TestLink_pageRendersLabelText(t *testing.T) {
	// Page links are not resolvable to a URL in Phase 1 → label text, no broken link.
	got := md(t, `<p><ac:link><ri:page ri:content-title="Target Page" ri:space-key="DEMO"/></ac:link></p>`)
	if !strings.Contains(got, "Target Page") {
		t.Fatalf("page title missing: %q", got)
	}
	if strings.Contains(got, "](") {
		t.Fatalf("page link should be plain text, got %q", got)
	}
}

func TestLink_user(t *testing.T) {
	got := md(t, `<p><ac:link><ri:user ri:username="jdoe"/></ac:link></p>`)
	if !strings.Contains(got, "@jdoe") {
		t.Fatalf("user mention not rendered: %q", got)
	}
}

func TestLink_attachmentLabelFromFilename(t *testing.T) {
	got := md(t, `<p><ac:link><ri:attachment ri:filename="spec.pdf"/></ac:link></p>`)
	if !strings.Contains(got, "spec.pdf") {
		t.Fatalf("attachment filename missing: %q", got)
	}
}

// Smart links render as cards in Confluence, so the source often has no
// space around them.
func TestLink_cardSeparatedFromWords(t *testing.T) {
	in := `<p>please use<ac:link ac:card-appearance="inline"><ri:page ri:content-title="Setup - SAPCC" /><ac:link-body>Setup - SAPCC</ac:link-body></ac:link>for SAP, (<ac:link ac:card-appearance="inline"><ri:page ri:content-title="X" /></ac:link>).</p>`
	got, _ := convert(t, in, nil)
	if want := "please use Setup - SAPCC for SAP, (X)."; strings.TrimSpace(got) != want {
		t.Errorf("got %q, want %q", strings.TrimSpace(got), want)
	}
	// Ordinary links keep the source spacing.
	got, _ = convert(t, `<p>a<ac:link><ri:page ri:content-title="P" /></ac:link>b</p>`, nil)
	if strings.TrimSpace(got) != "aPb" {
		t.Errorf("plain link: %q", got)
	}
}
