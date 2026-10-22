package contentmd_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vkh/spacemosquito/internal/contentmd"
)

func TestMarkdownToHTML(t *testing.T) {
	md := "Intro\n\nUPDATE CATEGORY\nSET CODE = 'X'\n\n![shot.png](assets/images/shot.png)\n\n| a | b |\n| --- | --- |\n| 1 | 2 |\n\n<script>alert(1)</script>\n"
	got, err := contentmd.MarkdownToHTML("TST", "Export <all>", "https://example.atlassian.net/wiki/spaces/TST/pages/1", md)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<meta charset="utf-8">`,
		`<title>Export &lt;all&gt;</title>`,
		`<a href="../index.html">← TST index</a>`,
		`<a href="https://example.atlassian.net/wiki/spaces/TST/pages/1">View in Confluence</a>`,
		`<img src="assets/images/shot.png" alt="shot.png">`,
		`UPDATE CATEGORY<br>`,
		`<table>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "<script>") {
		t.Errorf("raw HTML not escaped:\n%s", got)
	}
}

func TestRefreshDirIndexHTML_storageRendersImages(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "metadata.json", `{"title":"Export","confluence_url":"https://example.atlassian.net/wiki/spaces/TST/pages/1","body_format":"storage"}`)
	writeFile(t, dir, "raw.html", `<p>Query options</p><ac:image ac:alt="shot.png"><ri:attachment ri:filename="shot.png" /></ac:image>`)

	md, skip, err := contentmd.RenderDirMarkdown(dir)
	if err != nil || skip != "" {
		t.Fatalf("RenderDirMarkdown: skip=%q err=%v", skip, err)
	}
	wrote, err := contentmd.RefreshDirIndexHTML(dir, md)
	if err != nil || !wrote {
		t.Fatalf("RefreshDirIndexHTML: wrote=%v err=%v", wrote, err)
	}
	b, err := os.ReadFile(filepath.Join(dir, contentmd.IndexFileName))
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if !strings.Contains(got, `<img src="assets/images/shot.png"`) {
		t.Errorf("image not linked locally:\n%s", got)
	}
	if strings.Contains(got, "ac:image") {
		t.Errorf("storage markup leaked into index.html:\n%s", got)
	}
	if !strings.Contains(got, "<h1>Export</h1>") {
		t.Errorf("title missing:\n%s", got)
	}
}

func TestRefreshDirIndexHTML_renderedUntouched(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "metadata.json", `{"title":"Page","body_format":"rendered"}`)
	writeFile(t, dir, "index.html", `<p>extracted</p>`)

	wrote, err := contentmd.RefreshDirIndexHTML(dir, "extracted")
	if err != nil || wrote {
		t.Fatalf("wrote=%v err=%v, want untouched", wrote, err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	if string(b) != `<p>extracted</p>` {
		t.Fatalf("index.html changed: %s", b)
	}
}
