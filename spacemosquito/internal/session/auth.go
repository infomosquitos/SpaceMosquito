package session

import (
	"errors"
	"fmt"
	"time"
)

// ValidateTTL is how long a successful Confluence probe keeps status.valid true
// without a live re-check (GET /api/session/status).
const ValidateTTL = 60 * time.Minute

// ErrUnauthorized is returned when Confluence rejects the stored session (HTTP 401/403).
var ErrUnauthorized = errors.New("session unauthorized")

// UnauthorizedHTTP wraps ErrUnauthorized with the response status code.
func UnauthorizedHTTP(statusCode int) error {
	return fmt.Errorf("%w (%d)", ErrUnauthorized, statusCode)
}

// IsUnauthorized reports whether err is (or wraps) ErrUnauthorized.
func IsUnauthorized(err error) bool {
	return errors.Is(err, ErrUnauthorized)
}

// ClearValidation drops ValidatedAt so status reports invalid until the next
// successful validate. Cookies are left in place for a later recapture/validate.
func (s *Session) ClearValidation() {
	if s != nil {
		s.ValidatedAt = nil
	}
}

// ClearValidationForURL loads the host session, clears ValidatedAt, and persists.
// No-op (nil error) if there is no session for the host or ValidatedAt was already nil.
func (st *Store) ClearValidationForURL(encKey, confluenceURL string) error {
	sess, err := st.GetForURL(encKey, confluenceURL)
	if err != nil {
		if errors.Is(err, ErrNoSessionForHost) {
			return nil
		}
		return err
	}
	if sess.ValidatedAt == nil {
		return nil
	}
	sess.ClearValidation()
	if err := st.Upsert(sess, encKey); err != nil {
		return err
	}
	if st.log.Enabled() {
		st.log.Warnw("session validation cleared",
			"host", HostnameKey(confluenceURL),
			"url", sess.ConfluenceURL)
	}
	return nil
}

// MarkValidatedForURL records that Confluence just accepted the host session
// (e.g. a crawl request succeeded), so status stays valid while it is in use
// instead of expiring after ValidateTTL mid-crawl. The session is reloaded
// first so a concurrent recapture is not overwritten.
func (st *Store) MarkValidatedForURL(encKey, confluenceURL string) error {
	sess, err := st.GetForURL(encKey, confluenceURL)
	if err != nil {
		return err
	}
	now := time.Now()
	sess.ValidatedAt = &now
	return st.Upsert(sess, encKey)
}
