package contentmd_test

import (
	"strings"
	"testing"

	"github.com/vkh/spacemosquito/internal/contentmd"
)

func TestRenderMarkdown(t *testing.T) {
	md := "Intro\n\nUPDATE CATEGORY\nSET CODE = 'X'\n\n![shot.png](assets/images/shot.png)\n\n| a | b |\n| --- | --- |\n| 1 | 2 |\n\n<script>alert(1)</script>\n"
	got, err := contentmd.RenderMarkdown(md)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<img src="assets/images/shot.png" alt="shot.png">`,
		`UPDATE CATEGORY<br>`,
		`<table>`,
	} {
		if !strings.Contains(string(got), want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(string(got), "<script>") {
		t.Errorf("raw HTML not escaped:\n%s", got)
	}
}

// Storage Format images are linked to the page's downloaded assets, so they
// show when the page is served from its directory URL.
func TestRenderMarkdown_storageImages(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "metadata.json", `{"title":"Export","body_format":"storage"}`)
	writeFile(t, dir, "raw.html", `<p>Query options</p><ac:image ac:alt="shot.png"><ri:attachment ri:filename="shot.png" /></ac:image>`)

	md, skip, err := contentmd.RenderDirMarkdown(dir)
	if err != nil || skip != "" {
		t.Fatalf("RenderDirMarkdown: skip=%q err=%v", skip, err)
	}
	got, err := contentmd.RenderMarkdown(md)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `<img src="assets/images/shot.png"`) {
		t.Errorf("image not linked locally:\n%s", got)
	}
	if strings.Contains(string(got), "ac:image") {
		t.Errorf("storage markup leaked:\n%s", got)
	}
}
