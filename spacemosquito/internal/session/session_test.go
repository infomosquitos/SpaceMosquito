package session

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/vkh/spacemosquito/pkg/logging"
)

const testKey = "abcdefghijklmnop0123456789ABCDEF"

func nilSugar() logging.Sugar {
	return logging.Sugar{}
}

func init() {
	if len(testKey) != 32 {
		panic("testKey must be exactly 32 chars")
	}
}

func TestStore_RoundTrip(t *testing.T) {
	tmpFile := t.TempDir() + "/session.enc"

	store := NewStore(tmpFile, nilSugar())
	if store.HasSession() {
		t.Fatal("expected no session before save")
	}

	sess := &Session{
		ConfluenceURL: "https://example.atlassian.net",
		Cookies: []Cookie{
			{Name: "ATLSSO", Value: "abc123", Domain: ".atlassian.net"},
			{Name: "JSESSIONID", Value: "xyz789", Domain: "example.atlassian.net"},
		},
		CapturedAt: time.Now(),
	}

	if err := store.Upsert(sess, testKey); err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	if !store.HasSession() {
		t.Fatal("expected session file after save")
	}

	loaded, err := store.GetForURL(testKey, sess.ConfluenceURL)
	if err != nil {
		t.Fatalf("GetForURL failed: %v", err)
	}

	if loaded.ConfluenceURL != sess.ConfluenceURL {
		t.Errorf("expected URL %q, got %q", sess.ConfluenceURL, loaded.ConfluenceURL)
	}

	if len(loaded.Cookies) != len(sess.Cookies) {
		t.Fatalf("expected %d cookies, got %d", len(sess.Cookies), len(loaded.Cookies))
	}

	for i, c := range loaded.Cookies {
		if c.Name != sess.Cookies[i].Name || c.Value != sess.Cookies[i].Value {
			t.Errorf("cookie %d mismatch: got %+v", i, c)
		}
	}
}

func TestStore_WrongKey(t *testing.T) {
	tmpFile := t.TempDir() + "/session.enc"
	wrongKey := "zyxwvutsrqponmlk0987654321FEDCBA"

	store := NewStore(tmpFile, nilSugar())
	sess := &Session{
		ConfluenceURL: "https://example.atlassian.net",
		Cookies:       []Cookie{{Name: "test", Value: "val", Domain: ".example.com"}},
		CapturedAt:    time.Now(),
	}

	if err := store.Upsert(sess, testKey); err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	_, err := store.GetForURL(wrongKey, sess.ConfluenceURL)
	if err == nil {
		t.Fatal("expected error with wrong key, got nil")
	}
}

func TestStore_Delete(t *testing.T) {
	tmpFile := t.TempDir() + "/session.enc"

	store := NewStore(tmpFile, nilSugar())
	sess := &Session{
		ConfluenceURL: "https://example.atlassian.net",
		Cookies:       []Cookie{{Name: "test", Value: "val", Domain: ".example.com"}},
		CapturedAt:    time.Now(),
	}

	if err := store.Upsert(sess, testKey); err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	if err := store.Delete(); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Delete truncates the file (Docker volume compatibility); file may still exist.
	_, err := store.GetForURL(testKey, sess.ConfluenceURL)
	if err == nil {
		t.Fatal("expected error loading session after delete")
	}

	if err := store.Delete(); err != nil {
		t.Fatalf("Delete on missing file should not error: %v", err)
	}
}

func TestStore_MissingFile(t *testing.T) {
	tmpFile := t.TempDir() + "/nonexistent.enc"
	store := NewStore(tmpFile, nilSugar())
	if store.HasSession() {
		t.Fatal("expected no session for missing file")
	}

	_, err := store.GetForURL("any-key", "https://example.atlassian.net")
	if err == nil {
		t.Fatal("expected error loading missing file")
	}
}

func TestStore_EmptyKey(t *testing.T) {
	tmpFile := t.TempDir() + "/session.enc"
	store := NewStore(tmpFile, nilSugar())

	sess := &Session{
		ConfluenceURL: "https://example.atlassian.net",
		Cookies:       []Cookie{{Name: "test", Value: "val", Domain: ".example.com"}},
		CapturedAt:    time.Now(),
	}

	err := store.Upsert(sess, "")
	if err == nil {
		t.Fatal("expected error with empty key")
	}

	_, err = store.GetForURL("", sess.ConfluenceURL)
	if err == nil {
		t.Fatal("expected error loading with empty key")
	}
}

func TestSession_IsExpired(t *testing.T) {
	sess := &Session{CapturedAt: time.Now().Add(-25 * time.Hour)}
	if !sess.IsExpired(24 * time.Hour) {
		t.Fatal("expected session to be expired")
	}

	sess2 := &Session{CapturedAt: time.Now().Add(-1 * time.Hour)}
	if sess2.IsExpired(24 * time.Hour) {
		t.Fatal("expected session to not be expired")
	}
}

func TestSession_ValidateWithConfluence_NoURL(t *testing.T) {
	sess := &Session{
		Cookies: []Cookie{{Name: "test", Value: "val", Domain: ".example.com"}},
	}
	result, err := sess.ValidateWithConfluence("", 5, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Valid {
		t.Fatal("expected invalid session with no URL")
	}
}

func TestSession_ValidateWithConfluence_NoCookies(t *testing.T) {
	sess := &Session{ConfluenceURL: "https://example.atlassian.net"}
	result, err := sess.ValidateWithConfluence("", 5, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Valid {
		t.Fatal("expected invalid session with no cookies")
	}
}

func TestSession_ValidateWithConfluence_BadURL(t *testing.T) {
	sess := &Session{
		ConfluenceURL: "http://localhost:99999",
		Cookies:       []Cookie{{Name: "test", Value: "val", Domain: ".example.com"}},
	}
	result, err := sess.ValidateWithConfluence("", 2, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Valid {
		t.Fatal("expected invalid session with unreachable URL")
	}
}

func TestStore_FilePermissions(t *testing.T) {
	tmpFile := t.TempDir() + "/session.enc"

	store := NewStore(tmpFile, nilSugar())
	sess := &Session{
		ConfluenceURL: "https://example.atlassian.net",
		Cookies:       []Cookie{{Name: "test", Value: "val", Domain: ".example.com"}},
		CapturedAt:    time.Now(),
	}

	if err := store.Upsert(sess, testKey); err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	info, err := os.Stat(tmpFile)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}

	expectedPerm := os.FileMode(0600)
	if info.Mode().Perm() != expectedPerm {
		t.Errorf("file permissions = %o, want %o", info.Mode().Perm(), expectedPerm)
	}
}

func TestStore_KeyTruncation(t *testing.T) {
	tmpFile := t.TempDir() + "/session.enc"
	longKey := "this-is-a-very-long-key-that-exceeds-thirty-two-chars!"

	store := NewStore(tmpFile, nilSugar())
	sess := &Session{
		ConfluenceURL: "https://example.atlassian.net",
		Cookies:       []Cookie{{Name: "test", Value: "val", Domain: ".example.com"}},
		CapturedAt:    time.Now(),
	}

	if err := store.Upsert(sess, longKey); err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	loaded, err := store.GetForURL(longKey, sess.ConfluenceURL)
	if err != nil {
		t.Fatalf("GetForURL failed: %v", err)
	}

	if loaded.ConfluenceURL != sess.ConfluenceURL {
		t.Errorf("expected URL %q, got %q", sess.ConfluenceURL, loaded.ConfluenceURL)
	}
}

func TestStore_KeyPadding(t *testing.T) {
	tmpFile := t.TempDir() + "/session.enc"
	shortKey := "short"

	store := NewStore(tmpFile, nilSugar())
	sess := &Session{
		ConfluenceURL: "https://example.atlassian.net",
		Cookies:       []Cookie{{Name: "test", Value: "val", Domain: ".example.com"}},
		CapturedAt:    time.Now(),
	}

	if err := store.Upsert(sess, shortKey); err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	loaded, err := store.GetForURL(shortKey, sess.ConfluenceURL)
	if err != nil {
		t.Fatalf("GetForURL failed: %v", err)
	}

	if loaded.ConfluenceURL != sess.ConfluenceURL {
		t.Errorf("expected URL %q, got %q", sess.ConfluenceURL, loaded.ConfluenceURL)
	}
}

func TestStore_RejectsLegacySingleSession(t *testing.T) {
	tmpFile := t.TempDir() + "/session.enc"
	store := NewStore(tmpFile, nilSugar())

	// Not a {version, sessions} blob — should be rejected.
	legacy := &Session{
		ConfluenceURL: "https://legacy.example.com/wiki/spaces/L",
		Cookies:       []Cookie{{Name: "t", Value: "legacy"}},
		CapturedAt:    time.Now(),
	}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.encryptWrite(raw, testKey); err != nil {
		t.Fatal(err)
	}

	_, err = store.GetForURL(testKey, "https://legacy.example.com/display/X")
	if !errors.Is(err, ErrUnsupportedSessionFormat) {
		t.Fatalf("want ErrUnsupportedSessionFormat, got %v", err)
	}

	// Upsert must not wipe an unreadable legacy blob.
	err = store.Upsert(&Session{
		ConfluenceURL: "https://other.example.com/x",
		Cookies:       []Cookie{{Name: "t", Value: "new"}},
		CapturedAt:    time.Now(),
	}, testKey)
	if !errors.Is(err, ErrUnsupportedSessionFormat) {
		t.Fatalf("Upsert want ErrUnsupportedSessionFormat, got %v", err)
	}
}
