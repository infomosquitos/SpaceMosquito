package csf

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// imageRule renders ac:image. An ri:attachment child is a page attachment: the
// rule schedules a download and links the local copy under assets/images/. An
// ri:url child is an external image linked directly (no download).
type imageRule struct{}

func (imageRule) Name() string { return "image" }

func (imageRule) Match(sel *goquery.Selection) bool {
	return tagName(sel) == "ac:image"
}

func (imageRule) Render(sel *goquery.Selection, ctx *RenderContext) (string, error) {
	alt := strings.TrimSpace(sel.AttrOr("ac:alt", ""))

	// External image: <ri:url ri:value="https://…"/>
	if u := findDescendant(sel, "ri:url"); u != nil {
		if v := strings.TrimSpace(u.AttrOr("ri:value", "")); v != "" {
			return placeImage(sel, ctx, fmt.Sprintf("![%s](%s)", alt, v)), nil
		}
	}

	// Attachment: <ri:attachment ri:filename="…"/>
	att := findDescendant(sel, "ri:attachment")
	if att == nil {
		return "", nil
	}
	filename := strings.TrimSpace(att.AttrOr("ri:filename", ""))
	if filename == "" {
		return "", nil
	}
	local := sanitizeAssetName(filename)

	downloadURL := attachmentURL(ctx.BaseURL, ctx.PageID, filename, ctx.Cloud)
	if downloadURL != "" {
		ctx.RequestAsset(AssetRequest{Kind: "image", Filename: local, URL: downloadURL})
	}
	return placeImage(sel, ctx, fmt.Sprintf("![%s](assets/images/%s)", alt, url.PathEscape(local))), nil
}

// blockParents hold images placed as their own block (not inside a
// paragraph); in table cells the line breaks collapse to spaces.
var blockParents = map[string]bool{
	"body": true, "ac:layout-cell": true, "ac:rich-text-body": true, "ac:adf-content": true,
	"div": true, "blockquote": true, "td": true, "th": true,
}

// placeImage adds the image's caption (in italics) and, for an image placed
// as its own block, paragraph breaks so neighbouring images do not run
// together on one line.
func placeImage(sel *goquery.Selection, ctx *RenderContext, img string) string {
	caption := ""
	if c := findDescendant(sel, "ac:caption"); c != nil {
		caption = strings.Join(strings.Fields(ctx.RenderChildren(c)), " ")
	}
	if !blockParents[tagName(sel.Parent())] {
		if caption != "" {
			return img + " *" + caption + "*"
		}
		return img
	}
	if caption != "" {
		img += "\n\n*" + caption + "*"
	}
	return "\n\n" + img + "\n\n"
}

// attachmentURL builds the download URL for a page attachment, flavor-aware.
// Returns "" if base/pageID/filename are missing.
//
// The Server/DC shape (/download/attachments/...) is verified against a live
// instance. The Cloud shape (/wiki/download/attachments/...) is ASSUMED and not
// yet verified on a live Cloud instance — revisit if Cloud downloads 404.
func attachmentURL(base string, pageID int, filename string, cloud bool) string {
	if base == "" || pageID == 0 || filename == "" {
		return ""
	}
	prefix := "/download/attachments/"
	if cloud {
		prefix = "/wiki/download/attachments/"
	}
	return fmt.Sprintf("%s%s%d/%s", base, prefix, pageID, url.PathEscape(filename))
}

// sanitizeAssetName makes an attachment filename safe as a single path segment,
// preserving spaces and the extension (path separators become underscores).
func sanitizeAssetName(name string) string {
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	return strings.TrimSpace(name)
}
