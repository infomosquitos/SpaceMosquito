package confluence

import "testing"

func TestBaseURL(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"not-a-url", ""},
		{"https://company.atlassian.net/wiki/spaces/PROJ", "https://company.atlassian.net"},
		{"https://company.atlassian.net/wiki/spaces/PROJ/pages/1/Title", "https://company.atlassian.net"},
		{"https://wiki.example.com/display/KEY", "https://wiki.example.com"},
		{"https://wiki.example.com/display/KEY/Home", "https://wiki.example.com"},
		{"https://wiki.example.com/confluence/display/KEY", "https://wiki.example.com/confluence"},
		{"https://wiki.example.com/confluence/display/KEY/Home", "https://wiki.example.com/confluence"},
		{"https://wiki.example.com/confluence/spaces/KEY", "https://wiki.example.com/confluence"},
		{"https://wiki.company.net/spaces/SP/pages/1/Title", "https://wiki.company.net"},
		{"https://confluence.example.com:8443/display/KEY", "https://confluence.example.com:8443"},
		// No browse marker: cannot infer context path (degraded to origin).
		{"https://wiki.example.com/confluence/pages/viewpage.action?pageId=1", "https://wiki.example.com"},
	}
	for _, tc := range tests {
		if got := BaseURL(tc.in); got != tc.want {
			t.Errorf("BaseURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDeriveSpaceURL(t *testing.T) {
	tests := []struct {
		pageURL  string
		spaceKey string
		want     string
	}{
		{"", "KEY", ""},
		{"https://x.atlassian.net/wiki/spaces/KEY/pages/1", "KEY", "https://x.atlassian.net/wiki/spaces/KEY"},
		{"https://wiki.example.com/confluence/display/ENG/Home", "ENG", "https://wiki.example.com/confluence/display/ENG"},
		{"https://wiki.example.com/spaces/ENG/pages/9", "ENG", "https://wiki.example.com/spaces/ENG"},
	}
	for _, tc := range tests {
		if got := DeriveSpaceURL(tc.pageURL, tc.spaceKey); got != tc.want {
			t.Errorf("DeriveSpaceURL(%q, %q) = %q, want %q", tc.pageURL, tc.spaceKey, got, tc.want)
		}
	}
}

func TestWebUIURL(t *testing.T) {
	cases := []struct {
		base, webui string
		cloud       bool
		want        string
	}{
		{"https://x.atlassian.net", "/spaces/KEY/pages/1/T", true, "https://x.atlassian.net/wiki/spaces/KEY/pages/1/T"},
		{"https://x.atlassian.net/", "/wiki/spaces/KEY/pages/1", true, "https://x.atlassian.net/wiki/spaces/KEY/pages/1"},
		{"https://wiki.example.com/confluence", "/display/KEY/T", false, "https://wiki.example.com/confluence/display/KEY/T"},
		{"https://x.atlassian.net", "https://other/abs", true, "https://other/abs"},
		{"https://x.atlassian.net", "", true, ""},
	}
	for _, c := range cases {
		if got := WebUIURL(c.base, c.webui, c.cloud); got != c.want {
			t.Errorf("WebUIURL(%q, %q, %v) = %q, want %q", c.base, c.webui, c.cloud, got, c.want)
		}
	}
}
