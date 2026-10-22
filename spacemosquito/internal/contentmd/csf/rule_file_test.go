package csf

import (
	"strings"
	"testing"
)

func fileCtx() *RenderContext {
	return &RenderContext{PageID: 1, BaseURL: "https://x.atlassian.net", Cloud: true, Files: map[string]FileLink{
		"Workshop 10.pptx": {Href: "assets/attachments/Workshop%2010.pptx", Size: 2_500_000},
		"Spec.pdf":         {Href: "assets/attachments/Spec.pdf", Size: 900},
		"Huge [v2].zip":    {Href: "https://x.atlassian.net/wiki/download/attachments/1/Huge.zip", Size: 5 << 30},
	}}
}

func TestFile_viewFileMacros(t *testing.T) {
	for _, macro := range []string{"view-file", "viewppt"} {
		in := `<ac:structured-macro ac:name="` + macro + `"><ac:parameter ac:name="name"><ri:attachment ri:filename="Workshop 10.pptx" /></ac:parameter><ac:parameter ac:name="height">250</ac:parameter></ac:structured-macro>`
		got, _ := convert(t, in, fileCtx())
		if want := `📎 [Workshop 10.pptx](<assets/attachments/Workshop%2010.pptx> "attachment")`; !strings.Contains(got, want) {
			t.Errorf("%s: got %q, want %q", macro, got, want)
		}
		if strings.Contains(got, "250") {
			t.Errorf("%s: macro parameter leaked: %q", macro, got)
		}
	}
	// A file the page does not list (e.g. listing failed) stays its name.
	got, _ := convert(t, `<ac:structured-macro ac:name="view-file"><ac:parameter ac:name="name"><ri:attachment ri:filename="Gone_v1.pdf" /></ac:parameter></ac:structured-macro>`, fileCtx())
	if strings.TrimSpace(got) != `📎 Gone\_v1.pdf` {
		t.Errorf("unknown file: %q", got)
	}
}

func TestFile_attachmentsMacro(t *testing.T) {
	got, _ := convert(t, `<p>Files:</p><ac:structured-macro ac:name="attachments"><ac:parameter ac:name="upload">false</ac:parameter></ac:structured-macro>`, fileCtx())
	for _, want := range []string{
		"- 📎 [Huge \\[v2\\].zip](<https://x.atlassian.net/wiki/download/attachments/1/Huge.zip> \"attachment\") (5.0 GB)",
		"- 📎 [Spec.pdf](<assets/attachments/Spec.pdf> \"attachment\") (900 B)",
		"- 📎 [Workshop 10.pptx](<assets/attachments/Workshop%2010.pptx> \"attachment\") (2.4 MB)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "Huge") || strings.Index(got, "Huge") > strings.Index(got, "Spec") {
		t.Errorf("files not sorted by name:\n%s", got)
	}
	if got, _ := convert(t, `<ac:structured-macro ac:name="attachments"></ac:structured-macro>`, &RenderContext{}); strings.TrimSpace(got) != "" {
		t.Errorf("no files: %q", got)
	}
}

func TestLink_attachmentFile(t *testing.T) {
	got, _ := convert(t, `<p>See <ac:link><ri:attachment ri:filename="Spec.pdf" /><ac:plain-text-link-body><![CDATA[the spec]]></ac:plain-text-link-body></ac:link>.</p>`, fileCtx())
	if !strings.Contains(got, "(<assets/attachments/Spec.pdf>)") {
		t.Errorf("attachment link not local: %q", got)
	}
	got, _ = convert(t, `<p><ac:link><ri:attachment ri:filename="unknown.docx" /></ac:link></p>`, fileCtx())
	if strings.TrimSpace(got) != "unknown.docx" {
		t.Errorf("unknown attachment: %q", got)
	}
}
