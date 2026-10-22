package search

import (
	"strings"
	"unicode/utf8"
)

// DefaultExcerptMaxRunes is the post-normalization cap for search snippets.
const DefaultExcerptMaxRunes = 400

// Match markers wrap matched terms in raw FTS excerpts (Unicode private-use
// characters, so they never collide with page text). NormalizeExcerpt strips
// them; HighlightedExcerpt keeps them for the browse UI.
const (
	MatchStart = "\uE000"
	MatchEnd   = "\uE001"
)

var matchMarkers = strings.NewReplacer(MatchStart, "", MatchEnd, "")

// NormalizeExcerpt cleans ts_headline output for plain-text API consumers.
func NormalizeExcerpt(raw string, maxRunes int) string {
	return normalizeExcerpt(matchMarkers.Replace(raw), maxRunes)
}

// HighlightedExcerpt normalizes like NormalizeExcerpt but keeps the
// MatchStart/MatchEnd markers around matched terms.
func HighlightedExcerpt(raw string, maxRunes int) string {
	return normalizeExcerpt(raw, maxRunes)
}

func normalizeExcerpt(raw string, maxRunes int) string {
	if maxRunes <= 0 {
		maxRunes = DefaultExcerptMaxRunes
	}

	s := raw
	s = strings.ReplaceAll(s, "<b>", "")
	s = strings.ReplaceAll(s, "</b>", "")
	s = strings.ReplaceAll(s, "<B>", "")
	s = strings.ReplaceAll(s, "</B>", "")
	s = strings.Join(strings.Fields(s), " ")
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}

	runes := []rune(s)
	if len(runes) > maxRunes {
		runes = runes[:maxRunes]
	}
	return strings.TrimSpace(string(runes)) + "..."
}
