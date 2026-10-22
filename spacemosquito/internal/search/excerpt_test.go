package search

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestNormalizeExcerpt_stripsHighlightTags(t *testing.T) {
	got := NormalizeExcerpt("before <b>match</b> after", 400)
	if strings.Contains(got, "<b>") || strings.Contains(got, "</b>") {
		t.Errorf("tags not stripped: %q", got)
	}
	if !strings.Contains(got, "match") {
		t.Errorf("content lost: %q", got)
	}
}

func TestNormalizeExcerpt_collapsesWhitespace(t *testing.T) {
	got := NormalizeExcerpt("  foo   \n\n  bar  ", 400)
	if got != "foo bar" {
		t.Errorf("got %q", got)
	}
}

func TestNormalizeExcerpt_empty(t *testing.T) {
	if got := NormalizeExcerpt("", 400); got != "" {
		t.Errorf("got %q", got)
	}
	if got := NormalizeExcerpt("   ", 400); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestNormalizeExcerpt_truncatesWithEllipsis(t *testing.T) {
	long := strings.Repeat("a", 500)
	got := NormalizeExcerpt(long, 400)
	if utf8.RuneCountInString(strings.TrimSuffix(got, "...")) > 400 {
		t.Errorf("too long: %d runes", utf8.RuneCountInString(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("expected ellipsis suffix, got %q", got)
	}
}

func TestNormalizeExcerpt_shortUnchanged(t *testing.T) {
	raw := "short excerpt"
	if got := NormalizeExcerpt(raw, 400); got != raw {
		t.Errorf("got %q", got)
	}
}

func TestExcerptMatchMarkers(t *testing.T) {
	raw := "the " + MatchStart + "mosquito" + MatchEnd + "   flies"
	if got := NormalizeExcerpt(raw, 0); got != "the mosquito flies" {
		t.Fatalf("NormalizeExcerpt = %q, markers must be stripped", got)
	}
	if got := HighlightedExcerpt(raw, 0); got != "the "+MatchStart+"mosquito"+MatchEnd+" flies" {
		t.Fatalf("HighlightedExcerpt = %q", got)
	}
}
