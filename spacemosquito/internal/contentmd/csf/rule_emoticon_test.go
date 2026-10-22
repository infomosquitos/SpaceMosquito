package csf

import (
	"strings"
	"testing"
)

func TestEmoticon_emojiFallbackPreferred(t *testing.T) {
	got := md(t, `<p>done <ac:emoticon ac:name="blue-star" ac:emoji-fallback="👍"/> ok</p>`)
	if !strings.Contains(got, "👍") {
		t.Fatalf("emoji-fallback not used: %q", got)
	}
}

func TestEmoticon_legacyStaticMap(t *testing.T) {
	got := md(t, `<p>status <ac:emoticon ac:name="tick"/> and <ac:emoticon ac:name="cross"/></p>`)
	if !strings.Contains(got, "✓") || !strings.Contains(got, "✗") {
		t.Fatalf("legacy emoticon map failed: %q", got)
	}
}

func TestEmoticon_unknownDropped(t *testing.T) {
	got := md(t, `<p>before <ac:emoticon ac:name="totally-unknown-thing"/> after</p>`)
	if strings.Contains(got, "totally-unknown-thing") {
		t.Fatalf("unknown emoticon name leaked: %q", got)
	}
	if !strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Fatalf("surrounding text lost: %q", got)
	}
}

func TestEmoticon_fallbackForms(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		// Escaped UTF-16 surrogate pairs (📘, 📋).
		{`<h2><ac:emoticon ac:name="blue-star" ac:emoji-id="1f4d8" ac:emoji-fallback="\uD83D\uDCD8" />&nbsp;Instructions</h2>`, "## 📘\u00a0Instructions"}, // nbsp kept from the source,
		{`<p><ac:emoticon ac:name="blue-star" ac:emoji-fallback="\uD83D\uDCCB" /> Related</p>`, "📋 Related"},
		// Atlassian shortnames without a Unicode fallback.
		{`<p><ac:emoticon ac:name="tick" ac:emoji-fallback=":check_mark:" /> done</p>`, "✔️ done"},
		{`<p><ac:emoticon ac:name="blue-star" ac:emoji-fallback=":question_mark:" /> open</p>`, "❓ open"},
		// Unknown shortname: dropped, not shown as text.
		{`<p><ac:emoticon ac:name="blue-star" ac:emoji-fallback=":some_custom:" />x</p>`, "x"},
		// Plain glyph unchanged.
		{`<p><ac:emoticon ac:name="tick" ac:emoji-fallback="✅" /> ok</p>`, "✅ ok"},
	} {
		got, _ := convert(t, tc.in, nil)
		if strings.TrimSpace(got) != tc.want {
			t.Errorf("%s:\n got %q\nwant %q", tc.in, strings.TrimSpace(got), tc.want)
		}
	}
}
