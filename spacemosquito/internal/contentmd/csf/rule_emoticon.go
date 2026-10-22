package csf

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/PuerkitoBio/goquery"
)

// emoticonRule renders ac:emoticon as Unicode. Modern Confluence carries the
// glyph in ac:emoji-fallback; legacy emoticons only have ac:name, mapped via a
// static table. Unknown emoticons are dropped.
type emoticonRule struct{}

func (emoticonRule) Name() string { return "emoticon" }

func (emoticonRule) Match(sel *goquery.Selection) bool {
	return tagName(sel) == "ac:emoticon"
}

func (emoticonRule) Render(sel *goquery.Selection, ctx *RenderContext) (string, error) {
	// Modern emoji carry ac:emoji-fallback (their ac:name is a generic
	// "blue-star"); only legacy emoticons are looked up by name.
	var glyph string
	if fb, ok := sel.Attr("ac:emoji-fallback"); ok {
		glyph = emojiFallback(fb)
	} else {
		glyph = emoticonUnicode[sel.AttrOr("ac:name", "")]
	}
	// ac:emoticon is empty, but its self-closing form makes the HTML parser
	// mis-nest following siblings inside it; render them so nothing is lost.
	return glyph + ctx.RenderChildren(sel), nil
}

// emoticonUnicode maps legacy Confluence emoticon names to Unicode glyphs.
var emoticonUnicode = map[string]string{
	"tick":         "✓",
	"cross":        "✗",
	"warning":      "⚠",
	"information":  "ℹ",
	"question":     "❓",
	"plus":         "＋",
	"minus":        "－",
	"check":        "✓",
	"error":        "✗",
	"smile":        "🙂",
	"sad":          "🙁",
	"cheeky":       "😛",
	"laugh":        "😂",
	"wink":         "😉",
	"thumbs-up":    "👍",
	"thumbs-down":  "👎",
	"heart":        "❤",
	"broken-heart": "💔",
	"star":         "⭐",
	"yellow-star":  "⭐",
	"red-star":     "⭐",
	"green-star":   "⭐",
	"blue-star":    "⭐",
}

// emojiFallback turns ac:emoji-fallback into a glyph. Confluence stores it as
// the glyph itself ("✅"), as escaped UTF-16 ("\uD83D\uDCD8" for 📘), or as
// an Atlassian emoji shortname (":check_mark:"). Unknown shortnames are dropped
// rather than shown as text.
func emojiFallback(fb string) string {
	fb = strings.TrimSpace(fb)
	switch {
	case fb == "":
		return ""
	case strings.Contains(fb, `\u`):
		return decodeUTF16Escapes(fb)
	case shortname.MatchString(fb):
		return atlassianEmoji[strings.Trim(fb, ":")]
	}
	return fb
}

var (
	shortname   = regexp.MustCompile(`^:[a-z0-9_+-]+:$`)
	utf16Escape = regexp.MustCompile(`\\u([0-9a-fA-F]{4})`)
)

// decodeUTF16Escapes decodes \uXXXX sequences, joining surrogate pairs.
func decodeUTF16Escapes(s string) string {
	var out strings.Builder
	var units []uint16
	flush := func() {
		if len(units) > 0 {
			out.WriteString(string(utf16.Decode(units)))
			units = units[:0]
		}
	}
	last := 0
	for _, m := range utf16Escape.FindAllStringSubmatchIndex(s, -1) {
		if m[0] > last {
			flush()
			out.WriteString(s[last:m[0]])
		}
		n, _ := strconv.ParseUint(s[m[2]:m[3]], 16, 16)
		units = append(units, uint16(n))
		last = m[1]
	}
	flush()
	out.WriteString(s[last:])
	return out.String()
}

// atlassianEmoji maps Atlassian emoji shortnames (no Unicode fallback) to
// glyphs.
var atlassianEmoji = map[string]string{
	"check_mark":     "✔️",
	"cross_mark":     "❌",
	"info":           "ℹ️",
	"question_mark":  "❓",
	"warning":        "⚠️",
	"light_bulb_on":  "💡",
	"light_bulb_off": "💡",
	"plus":           "➕",
	"minus":          "➖",
	"thumbsup":       "👍",
	"thumbsdown":     "👎",
	"star":           "⭐",
}
