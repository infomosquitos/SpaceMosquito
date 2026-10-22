package csf

import (
	"strings"
	"testing"
)

func TestADF_decisionList(t *testing.T) {
	in := `<h2>Decisions</h2><ac:adf-extension><ac:adf-node type="decision-list"><ac:adf-attribute key="local-id">d7a0fb66-dceb-44c6-88cd-3f400f0e1db8</ac:adf-attribute>` +
		`<ac:adf-node type="decision-item"><ac:adf-attribute key="local-id">05e99ba7-8f39-46a2-9d2a-5d821d1d453a</ac:adf-attribute><ac:adf-attribute key="state">DECIDED</ac:adf-attribute><ac:adf-content>No decisions <strong>taken</strong></ac:adf-content></ac:adf-node>` +
		`<ac:adf-node type="decision-item"><ac:adf-attribute key="state">UNDECIDED</ac:adf-attribute><ac:adf-content>Pick a vendor</ac:adf-content></ac:adf-node>` +
		`</ac:adf-node><ac:adf-fallback><ul class="decision-list"><li>No decisions taken</li></ul></ac:adf-fallback></ac:adf-extension><p>after</p>`
	got, _ := convert(t, in, nil)
	for _, want := range []string{"> **Decision:** No decisions **taken**\n>\n> **Decision (open):** Pick a vendor", "after"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	for _, bad := range []string{"d7a0fb66", "05e99ba7", "DECIDED\n", "decision-list"} {
		if strings.Contains(got, bad) {
			t.Errorf("leaked %q in:\n%s", bad, got)
		}
	}
	if strings.Count(got, "No decisions") != 1 {
		t.Errorf("fallback rendered as well:\n%s", got)
	}
}

func TestADF_panelAndFallback(t *testing.T) {
	in := `<ac:adf-extension><ac:adf-node type="panel"><ac:adf-attribute key="panel-type">note</ac:adf-attribute><ac:adf-attribute key="local-id">3906a751</ac:adf-attribute>` +
		`<ac:adf-content><h4 local-id="84bc">Open questions</h4><ol start="1"><li><p>Move storage?</p></li></ol></ac:adf-content></ac:adf-node></ac:adf-extension>`
	got, _ := convert(t, in, nil)
	for _, want := range []string{"> **Note:**", "> #### Open questions", "> 1. Move storage?"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "3906a751") || strings.Contains(got, "note\n") {
		t.Errorf("attribute leaked:\n%s", got)
	}
	// Unknown node type: Confluence's HTML fallback, without attributes.
	got, _ = convert(t, `<ac:adf-extension><ac:adf-node type="future-thing"><ac:adf-attribute key="local-id">abc-123</ac:adf-attribute></ac:adf-node><ac:adf-fallback><p>Fallback text</p></ac:adf-fallback></ac:adf-extension>`, nil)
	if strings.TrimSpace(got) != "Fallback text" {
		t.Errorf("fallback: %q", got)
	}
}
