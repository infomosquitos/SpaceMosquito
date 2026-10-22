package csf

import (
	"strings"
	"testing"
)

// convert runs CSFToMarkdown with a given context and returns md + assets.
func convert(t *testing.T, in string, ctx *RenderContext) (string, []AssetRequest) {
	t.Helper()
	out, assets, err := CSFToMarkdown(in, ctx)
	if err != nil {
		t.Fatalf("CSFToMarkdown: %v", err)
	}
	return out, assets
}

func TestImage_attachmentServer(t *testing.T) {
	ctx := &RenderContext{PageID: 123, BaseURL: "https://wiki.example.net"}
	got, assets := convert(t, `<p><ac:image><ri:attachment ri:filename="pic 1.png"/></ac:image></p>`, ctx)

	if !strings.Contains(got, "![](assets/images/pic%201.png)") {
		t.Fatalf("image link wrong: %q", got)
	}
	if len(assets) != 1 {
		t.Fatalf("want 1 asset request, got %d", len(assets))
	}
	a := assets[0]
	if a.Kind != "image" || a.Filename != "pic 1.png" {
		t.Fatalf("bad asset request: %+v", a)
	}
	if a.URL != "https://wiki.example.net/download/attachments/123/pic%201.png" {
		t.Fatalf("bad download URL: %q", a.URL)
	}
}

func TestImage_attachmentCloudURLShape(t *testing.T) {
	ctx := &RenderContext{PageID: 42, BaseURL: "https://x.atlassian.net", Cloud: true}
	_, assets := convert(t, `<ac:image><ri:attachment ri:filename="d.png"/></ac:image>`, ctx)
	if len(assets) != 1 || assets[0].URL != "https://x.atlassian.net/wiki/download/attachments/42/d.png" {
		t.Fatalf("cloud URL shape wrong: %+v", assets)
	}
}

func TestImage_externalURLNoDownload(t *testing.T) {
	ctx := &RenderContext{PageID: 1, BaseURL: "https://wiki.example.net"}
	got, assets := convert(t, `<ac:image><ri:url ri:value="https://cdn.example.com/x.png"/></ac:image>`, ctx)
	if !strings.Contains(got, "![](https://cdn.example.com/x.png)") {
		t.Fatalf("external image link wrong: %q", got)
	}
	if len(assets) != 0 {
		t.Fatalf("external image should not schedule a download, got %d", len(assets))
	}
}

func TestImage_noBaseURLStillLinksNoDownload(t *testing.T) {
	// Missing base/pageID (e.g. incomplete context): emit a link, skip download.
	got, assets := convert(t, `<ac:image><ri:attachment ri:filename="a.png"/></ac:image>`, &RenderContext{})
	if !strings.Contains(got, "![](assets/images/a.png)") {
		t.Fatalf("link missing: %q", got)
	}
	if len(assets) != 0 {
		t.Fatalf("no download expected without base/pageID, got %d", len(assets))
	}
}

// Images pasted from another page are that page's attachments.
func TestImage_attachmentOfOtherPage(t *testing.T) {
	in := `<ac:image ac:alt="a.png"><ri:attachment ri:filename="a.png"><ri:page ri:content-title="Import flow for deliveries" /></ri:attachment></ac:image>`
	var asked []string
	ctx := &RenderContext{PageID: 1, BaseURL: "https://x.atlassian.net", Cloud: true, ResolvePageID: func(space, title string) int {
		asked = append(asked, space+"|"+title)
		return 777
	}}
	got, assets := convert(t, in, ctx)
	if !strings.Contains(got, "![a.png](assets/images/a.png)") {
		t.Fatalf("image link: %q", got)
	}
	if len(assets) != 1 || assets[0].URL != "https://x.atlassian.net/wiki/download/attachments/777/a.png" {
		t.Fatalf("asset requests = %+v", assets)
	}
	if len(asked) != 1 || asked[0] != "|Import flow for deliveries" {
		t.Fatalf("resolver asked %v", asked)
	}
	// Unknown page (offline reindex, lookup failed): no request that would 404.
	if _, assets := convert(t, in, &RenderContext{PageID: 1, BaseURL: "https://x.atlassian.net", Cloud: true}); len(assets) != 0 {
		t.Fatalf("requested an attachment of an unknown page: %+v", assets)
	}
}
