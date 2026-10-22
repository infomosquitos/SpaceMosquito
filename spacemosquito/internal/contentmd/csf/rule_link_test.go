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

func TestLink_cloudMention(t *testing.T) {
	in := `<ul><li><p><ac:link><ri:user ri:account-id="712020:aa" ri:local-id="x" /></ac:link></p></li><li><p><ac:link><ri:user ri:account-id="712020:zz" /></ac:link></p></li></ul>`
	ctx := &RenderContext{UserName: func(id string) string { return map[string]string{"712020:aa": "Ann Larsen"}[id] }}
	got, _ := convert(t, in, ctx)
	if want := "- @Ann Larsen\n- @unknown user"; strings.TrimSpace(got) != want {
		t.Errorf("got %q, want %q", strings.TrimSpace(got), want)
	}
	// Without names (no crawl context) the mention is still visible.
	if got, _ := convert(t, `<p>Owner: <ac:link><ri:user ri:account-id="712020:aa" /></ac:link></p>`, nil); strings.TrimSpace(got) != "Owner: @unknown user" {
		t.Errorf("no names: %q", got)
	}
}

// Older pages keep the account ID as ri:userkey.
func TestLink_mentionUserKeyAccountID(t *testing.T) {
	in := `<ul><li><p><ac:link><ri:user ri:userkey="712020:43929ea0-bde0-4d06-96a9-bb3ede102937" /></ac:link></p></li><li><p><ac:link><ri:user ri:userkey="ff8081815a3f4a2b015a3f4b6d1e0001" /></ac:link></p></li></ul>`
	ctx := &RenderContext{UserName: func(id string) string {
		return map[string]string{"712020:43929ea0-bde0-4d06-96a9-bb3ede102937": "Ann Larsen"}[id]
	}}
	got, _ := convert(t, in, ctx)
	// A Server-style user key (32 hex) is not an account ID: kept as is.
	if want := "- @Ann Larsen\n- @ff8081815a3f4a2b015a3f4b6d1e0001"; strings.TrimSpace(got) != want {
		t.Errorf("got %q, want %q", strings.TrimSpace(got), want)
	}
	for id, want := range map[string]bool{"712020:43929ea0-bde0-4d06-96a9-bb3ede102937": true, "557058:9b673f74-cb0a-45d7-8cc7-0f1d10398cda": true, "5b10ac8d82e05b22cc7d4ef5": true, "jdoe": false, "ff8081815a3f4a2b015a3f4b6d1e0001": false} {
		if IsAccountID(id) != want {
			t.Errorf("IsAccountID(%q) = %v", id, !want)
		}
	}
}
