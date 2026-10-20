package session

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSession_AsHeaders(t *testing.T) {
	t.Run("no cookies", func(t *testing.T) {
		s := &Session{}
		h := s.AsHeaders()
		if _, ok := h["Cookie"]; ok {
			t.Fatal("expected no Cookie header")
		}
		if h["X-Atlassian-Token"] != "no-check" {
			t.Errorf("X-Atlassian-Token = %q", h["X-Atlassian-Token"])
		}
		if h["Accept"] != "application/json" {
			t.Errorf("Accept = %q", h["Accept"])
		}
	})

	t.Run("multiple cookies", func(t *testing.T) {
		s := &Session{
			Cookies: []Cookie{
				{Name: "a", Value: "1"},
				{Name: "b", Value: "2"},
			},
		}
		h := s.AsHeaders()
		if h["Cookie"] != "a=1; b=2" {
			t.Errorf("Cookie = %q", h["Cookie"])
		}
	})
}

func TestGetSpaceKeyFromURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://company.atlassian.net/wiki/spaces/PROJ/overview", "PROJ"},
		{"https://wiki.company.com/spaces/PROJ", "PROJ"},
		{"https://confluence.company.com/display/PROJ/Home", "PROJ"},
		{"https://company.atlassian.net/wiki/spaces/MY-SPACE/pages/1", "MY-SPACE"},
		{"https://example.com/no-space", ""},
	}
	for _, tc := range tests {
		if got := GetSpaceKeyFromURL(tc.url); got != tc.want {
			t.Errorf("GetSpaceKeyFromURL(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}

func TestGetSpaceNameFromURL(t *testing.T) {
	got := GetSpaceNameFromURL("https://company.atlassian.net/wiki/spaces/proj/overview")
	if got != "PROJ" {
		t.Errorf("got %q, want PROJ", got)
	}
}

func TestExtractConfluenceRoot(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://company.atlassian.net/wiki/spaces/PROJ", "https://company.atlassian.net"},
		{"https://confluence.example.com:8443/display/KEY", "https://confluence.example.com:8443"},
		{"https://wiki.example.com/confluence/display/KEY", "https://wiki.example.com/confluence"},
		{"", ""},
		{"not-a-url", ""},
	}
	for _, tc := range tests {
		if got := extractConfluenceRoot(tc.url); got != tc.want {
			t.Errorf("extractConfluenceRoot(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}

func TestStore_corruptCiphertext(t *testing.T) {
	tmpFile := t.TempDir() + "/session.enc"
	if err := os.WriteFile(tmpFile, []byte("tooshort"), 0600); err != nil {
		t.Fatal(err)
	}
	store := NewStore(tmpFile, nilSugar())
	_, err := store.GetForURL(testKey, "https://example.atlassian.net")
	if err == nil {
		t.Fatal("expected error loading corrupt ciphertext")
	}
}

func TestStore_nestedPath(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "sub", "dir", "session.enc")
	store := NewStore(tmpFile, nilSugar())
	sess := &Session{
		ConfluenceURL: "https://example.atlassian.net",
		Cookies:       []Cookie{{Name: "t", Value: "v", Domain: ".example.com"}},
		CapturedAt:    time.Now(),
	}
	if err := store.Upsert(sess, testKey); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(tmpFile); err != nil {
		t.Fatal(err)
	}
}

func TestValidateWithConfluence_mockServer(t *testing.T) {
	t.Run("200 json cloud", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/wiki/rest/api/user/current" {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"displayName":"Ada Lovelace"}`))
				return
			}
			http.NotFound(w, r)
		}))
		defer srv.Close()

		sess := &Session{
			ConfluenceURL: srv.URL + "/wiki/spaces/PROJ",
			Cookies:       []Cookie{{Name: "t", Value: "v"}},
		}
		result, err := sess.ValidateWithConfluence("", 5, "127.0.0.1")
		if err != nil {
			t.Fatal(err)
		}
		if !result.Valid || result.Flavor != FlavorCloud {
			t.Fatalf("result = %+v", result)
		}
		if !strings.Contains(result.Message, "Ada Lovelace") {
			t.Errorf("message = %q", result.Message)
		}
		if sess.ValidatedAt == nil {
			t.Fatal("expected ValidatedAt set on success")
		}
	})

	t.Run("401 unauthorized", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		}))
		defer srv.Close()

		sess := &Session{
			ConfluenceURL: srv.URL,
			Cookies:       []Cookie{{Name: "t", Value: "v"}},
		}
		result, err := sess.ValidateWithConfluence("", 5, "127.0.0.1")
		if err != nil {
			t.Fatal(err)
		}
		if result.Valid {
			t.Fatal("expected invalid session")
		}
		if !strings.Contains(result.Message, "expired") {
			t.Errorf("message = %q", result.Message)
		}
		if sess.ValidatedAt != nil {
			t.Fatal("ValidatedAt must not be set on failure")
		}
	})

	t.Run("403 forbidden", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "forbidden", http.StatusForbidden)
		}))
		defer srv.Close()

		sess := &Session{
			ConfluenceURL: srv.URL,
			Cookies:       []Cookie{{Name: "t", Value: "v"}},
		}
		result, err := sess.ValidateWithConfluence("", 5, "127.0.0.1")
		if err != nil {
			t.Fatal(err)
		}
		if result.Valid {
			t.Fatal("expected invalid session")
		}
	})

	t.Run("all probes 404", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		}))
		defer srv.Close()

		sess := &Session{
			ConfluenceURL: srv.URL,
			Cookies:       []Cookie{{Name: "t", Value: "v"}},
		}
		result, err := sess.ValidateWithConfluence("", 5, "127.0.0.1")
		if err != nil {
			t.Fatal(err)
		}
		if result.Valid {
			t.Fatal("expected invalid session")
		}
		if !strings.Contains(result.Message, "404") {
			t.Errorf("message = %q", result.Message)
		}
	})

	t.Run("cloud path SSO html then server json", func(t *testing.T) {
		// Cloud-first URL shape: Cloud probe returns Azure AD HTML 200;
		// must continue and accept Server JSON (must not short-circuit).
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/wiki/rest/api/user/current":
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Write([]byte(`<!DOCTYPE html><html><body>login.microsoftonline.com</body></html>`))
			case "/rest/api/latest/myself", "/rest/api/user/current":
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"displayName":"Ruslan Bes"}`))
			default:
				http.NotFound(w, r)
			}
		}))
		defer srv.Close()

		sess := &Session{
			ConfluenceURL: srv.URL + "/wiki/spaces/PROJ/pages/1",
			Cookies:       []Cookie{{Name: "JSESSIONID", Value: "stale"}},
		}
		result, err := sess.ValidateWithConfluence("", 5, "127.0.0.1")
		if err != nil {
			t.Fatal(err)
		}
		if !result.Valid || result.Flavor != FlavorServer {
			t.Fatalf("result = %+v", result)
		}
		if !strings.Contains(result.Message, "Ruslan Bes") {
			t.Errorf("message = %q", result.Message)
		}
	})

	t.Run("server url prefers server probe", func(t *testing.T) {
		var paths []string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			paths = append(paths, r.URL.Path)
			if r.URL.Path == "/rest/api/latest/myself" {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"displayName":"Server User"}`))
				return
			}
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<html>should not matter</html>`))
		}))
		defer srv.Close()

		sess := &Session{
			ConfluenceURL: srv.URL + "/spaces/MAPI/pages/1/Home",
			Cookies:       []Cookie{{Name: "t", Value: "v"}},
		}
		result, err := sess.ValidateWithConfluence("", 5, "127.0.0.1")
		if err != nil {
			t.Fatal(err)
		}
		if !result.Valid || result.Flavor != FlavorServer {
			t.Fatalf("result = %+v", result)
		}
		if len(paths) == 0 || paths[0] != "/rest/api/latest/myself" {
			t.Fatalf("expected server probe first, got %v", paths)
		}
	})

	t.Run("sso html only is invalid", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<html>login</html>`))
		}))
		defer srv.Close()

		prev := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		sess := &Session{
			ConfluenceURL: srv.URL + "/spaces/MAPI/pages/1",
			Cookies:       []Cookie{{Name: "t", Value: "v"}},
			ValidatedAt:   &prev,
			Flavor:        FlavorServer,
		}
		result, err := sess.ValidateWithConfluence("", 5, "127.0.0.1")
		if err != nil {
			t.Fatal(err)
		}
		if result.Valid {
			t.Fatal("expected invalid")
		}
		if !strings.Contains(result.Message, "SSO") {
			t.Errorf("message = %q", result.Message)
		}
		// Must not refresh ValidatedAt on failure (handler only saves when Valid).
		if sess.ValidatedAt == nil || !sess.ValidatedAt.Equal(prev) {
			t.Fatalf("ValidatedAt mutated: %v", sess.ValidatedAt)
		}
	})

	t.Run("redirect not followed", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "https://login.microsoftonline.com/", http.StatusFound)
		}))
		defer srv.Close()

		sess := &Session{
			ConfluenceURL: srv.URL + "/spaces/X",
			Cookies:       []Cookie{{Name: "t", Value: "v"}},
		}
		result, err := sess.ValidateWithConfluence("", 5, "127.0.0.1")
		if err != nil {
			t.Fatal(err)
		}
		if result.Valid {
			t.Fatal("expected invalid on redirect")
		}
		if !strings.Contains(result.Message, "SSO") {
			t.Errorf("message = %q", result.Message)
		}
	})

	t.Run("json without identity is invalid", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{}`))
		}))
		defer srv.Close()

		sess := &Session{
			ConfluenceURL: srv.URL + "/wiki/spaces/PROJ",
			Cookies:       []Cookie{{Name: "t", Value: "v"}},
		}
		result, err := sess.ValidateWithConfluence("", 5, "127.0.0.1")
		if err != nil {
			t.Fatal(err)
		}
		if result.Valid {
			t.Fatal("expected invalid without displayName")
		}
	})

	t.Run("followed redirect html formerly false positive", func(t *testing.T) {
		// Old client followed 302→200 HTML and treated it as authenticated.
		login := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<html>Azure AD</html>`))
		}))
		defer login.Close()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, login.URL, http.StatusFound)
		}))
		defer srv.Close()

		sess := &Session{
			ConfluenceURL: srv.URL + "/spaces/MAPI/pages/1",
			Cookies:       []Cookie{{Name: "t", Value: "v"}},
		}
		result, err := sess.ValidateWithConfluence("", 5, "127.0.0.1")
		if err != nil {
			t.Fatal(err)
		}
		if result.Valid {
			t.Fatalf("false positive: %+v", result)
		}
	})
}

func TestPreferServerProbes(t *testing.T) {
	if !preferServerProbes(FlavorServer, "https://x.atlassian.net/wiki/spaces/A") {
		t.Fatal("FlavorServer should prefer server")
	}
	if preferServerProbes(FlavorCloud, "https://wiki.example.com/spaces/A") {
		t.Fatal("FlavorCloud should prefer cloud")
	}
	if !preferServerProbes("", "https://wiki.example.net/spaces/MAPI/pages/1") {
		t.Fatal("server URL shape should prefer server")
	}
	if preferServerProbes("", "https://ex.atlassian.net/wiki/spaces/P") {
		t.Fatal("cloud URL should prefer cloud")
	}
}

