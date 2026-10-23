package session

import (
	"errors"
	"testing"
	"time"

	"github.com/vkh/spacemosquito/pkg/logging"
)

func TestUnauthorizedHTTP(t *testing.T) {
	err := UnauthorizedHTTP(401)
	if !IsUnauthorized(err) {
		t.Fatal("expected IsUnauthorized")
	}
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatal("expected errors.Is ErrUnauthorized")
	}
}

func TestClearValidationForURL(t *testing.T) {
	key := "12345678901234567890123456789012"
	store := NewStore(t.TempDir()+"/session.enc", logging.Sugar{})
	now := time.Now()
	sess := &Session{
		ConfluenceURL: "https://wiki.example.com/wiki",
		Cookies:       []Cookie{{Name: "JSESSIONID", Value: "x"}},
		CapturedAt:    now,
		ValidatedAt:   &now,
	}
	if err := store.Upsert(sess, key); err != nil {
		t.Fatal(err)
	}

	if err := store.ClearValidationForURL(key, "https://wiki.example.com/wiki/spaces/X"); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetForURL(key, "https://wiki.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got.ValidatedAt != nil {
		t.Fatalf("ValidatedAt still set: %v", got.ValidatedAt)
	}

	// Idempotent
	if err := store.ClearValidationForURL(key, "https://wiki.example.com"); err != nil {
		t.Fatal(err)
	}
}

func TestValidateTTL(t *testing.T) {
	if ValidateTTL != 60*time.Minute {
		t.Fatalf("ValidateTTL = %v, want 60m", ValidateTTL)
	}
}

func TestMarkValidatedForURL(t *testing.T) {
	key := "12345678901234567890123456789012"
	store := NewStore(t.TempDir()+"/session.enc", logging.Sugar{})
	old := time.Now().Add(-2 * ValidateTTL)
	sess := &Session{
		ConfluenceURL: "https://wiki.example.com/wiki",
		Cookies:       []Cookie{{Name: "JSESSIONID", Value: "x"}},
		CapturedAt:    old,
		ValidatedAt:   &old,
	}
	if err := store.Upsert(sess, key); err != nil {
		t.Fatal(err)
	}

	if err := store.MarkValidatedForURL(key, "https://wiki.example.com/wiki/spaces/X"); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetForURL(key, "https://wiki.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got.ValidatedAt == nil || time.Since(*got.ValidatedAt) > time.Minute {
		t.Fatalf("ValidatedAt not refreshed: %v", got.ValidatedAt)
	}
	if len(got.Cookies) != 1 || got.Cookies[0].Value != "x" {
		t.Fatalf("cookies changed: %+v", got.Cookies)
	}

	if err := store.MarkValidatedForURL(key, "https://other.example.com"); !errors.Is(err, ErrNoSessionForHost) {
		t.Fatalf("other host: err = %v, want ErrNoSessionForHost", err)
	}
}
