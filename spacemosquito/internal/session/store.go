package session

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/vkh/spacemosquito/pkg/logging"
)

const storeVersion = 2

// ErrNoSessionForHost is returned when no session is stored for a hostname.
var ErrNoSessionForHost = errors.New("no session stored for host")

// ErrUnsupportedSessionFormat is returned when session.enc is not a recognized multi-session blob.
var ErrUnsupportedSessionFormat = errors.New(
	"unsupported session.enc format; delete the session and re-capture",
)

type Store struct {
	filePath string
	log      logging.Sugar
}

// sessionBlob is the plaintext JSON shape inside session.enc: hostname → session.
type sessionBlob struct {
	Version  int                 `json:"version"`
	Sessions map[string]*Session `json:"sessions"`
}

func NewStore(filePath string, log logging.Sugar) *Store {
	return &Store{
		filePath: filePath,
		log:      log,
	}
}

func (s *Store) GetLogger() logging.Sugar {
	return s.log
}

func (s *Store) SetLogger(log logging.Sugar) {
	s.log = log
}

// HostnameKey returns the session map key for a Confluence URL (hostname only, lowercased).
func HostnameKey(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		// Tolerate bare hostnames.
		host := strings.TrimSpace(rawURL)
		host = strings.TrimPrefix(host, "https://")
		host = strings.TrimPrefix(host, "http://")
		if i := strings.IndexAny(host, "/?#"); i >= 0 {
			host = host[:i]
		}
		return strings.ToLower(host)
	}
	return strings.ToLower(u.Host)
}

func (s *Store) HasSession() bool {
	st, err := os.Stat(s.filePath)
	if err != nil {
		if s.log.Enabled() {
			s.log.Debugw("checking session file", "path", s.filePath, "exists", false)
		}
		return false
	}
	if st.Size() == 0 {
		return false
	}
	if s.log.Enabled() {
		s.log.Debugw("session file exists", "path", s.filePath)
	}
	return true
}

// Upsert stores or replaces the session for the hostname derived from sess.ConfluenceURL.
// Other hosts in the blob are left unchanged.
func (s *Store) Upsert(sess *Session, key string) error {
	if sess == nil {
		return fmt.Errorf("session is required")
	}
	host := HostnameKey(sess.ConfluenceURL)
	if host == "" {
		return fmt.Errorf("confluence_url must include a hostname")
	}

	blob, err := s.loadBlob(key)
	if err != nil {
		if s.HasSession() && !isEmptyOrMissing(err) && !errors.Is(err, os.ErrNotExist) && !strings.Contains(err.Error(), "failed to read session file") {
			// Wrong key or corrupt existing file: do not wipe other sites.
			return err
		}
		blob = emptyBlob()
	}
	if blob.Sessions == nil {
		blob.Sessions = make(map[string]*Session)
	}

	sess.SetLogger(s.log)
	blob.Sessions[host] = sess
	blob.Version = storeVersion

	if err := s.saveBlob(blob, key); err != nil {
		return err
	}
	if s.log.Enabled() {
		s.log.Infow("session upserted",
			"path", s.filePath,
			"host", host,
			"url", sess.ConfluenceURL,
			"sites", len(blob.Sessions))
	}
	return nil
}

// GetForURL returns the session for the hostname of rawURL.
func (s *Store) GetForURL(key, rawURL string) (*Session, error) {
	host := HostnameKey(rawURL)
	if host == "" {
		return nil, fmt.Errorf("%w: empty or invalid url", ErrNoSessionForHost)
	}
	return s.GetForHostname(key, host)
}

// GetForHostname returns the session stored for host (case-insensitive).
func (s *Store) GetForHostname(key, host string) (*Session, error) {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return nil, fmt.Errorf("%w: empty hostname", ErrNoSessionForHost)
	}
	blob, err := s.loadBlob(key)
	if err != nil {
		return nil, err
	}
	sess, ok := blob.Sessions[host]
	if !ok || sess == nil {
		return nil, fmt.Errorf("%w %q", ErrNoSessionForHost, host)
	}
	sess.SetLogger(s.log)
	return sess, nil
}

// Count returns how many site sessions are stored, or 0 if none / unreadable.
func (s *Store) Count(key string) int {
	blob, err := s.loadBlob(key)
	if err != nil {
		return 0
	}
	return len(blob.Sessions)
}

func (s *Store) Delete() error {
	if !s.HasSession() {
		if s.log.Enabled() {
			s.log.Info("session delete: no session file to delete")
		}
		return nil
	}

	// In Docker, we can't easily remove/rename a volume-mounted file.
	// We truncate it instead to clear the sensitive data.
	if err := os.Truncate(s.filePath, 0); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		if s.log.Enabled() {
			s.log.Errorw("session delete failed: truncate", "path", s.filePath, "error", err)
		}
		return fmt.Errorf("failed to clear session file %s: %w", s.filePath, err)
	}

	if s.log.Enabled() {
		s.log.Infow("session cleared (truncated)", "path", s.filePath)
	}
	return nil
}

func emptyBlob() *sessionBlob {
	return &sessionBlob{
		Version:  storeVersion,
		Sessions: make(map[string]*Session),
	}
}

func isEmptyOrMissing(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "empty session file") || strings.Contains(msg, "ciphertext too short")
}

func (s *Store) loadBlob(key string) (*sessionBlob, error) {
	plain, err := s.decryptFile(key)
	if err != nil {
		return nil, err
	}

	var blob sessionBlob
	if err := json.Unmarshal(plain, &blob); err == nil && blob.Version >= storeVersion && blob.Sessions != nil {
		return &blob, nil
	}

	if s.log.Enabled() {
		s.log.Errorw("session load failed: unsupported on-disk format",
			"path", s.filePath,
			"hint", "delete session.enc and re-capture")
	}
	return nil, ErrUnsupportedSessionFormat
}

func (s *Store) saveBlob(blob *sessionBlob, key string) error {
	if key == "" {
		if s.log.Enabled() {
			s.log.Error("session save rejected: encryption key is required")
		}
		return fmt.Errorf("encryption key is required")
	}
	data, err := json.Marshal(blob)
	if err != nil {
		return fmt.Errorf("failed to marshal session blob: %w", err)
	}
	return s.encryptWrite(data, key)
}

func (s *Store) decryptFile(key string) ([]byte, error) {
	if key == "" {
		if s.log.Enabled() {
			s.log.Error("session load rejected: encryption key is required")
		}
		return nil, fmt.Errorf("encryption key is required")
	}

	ciphertext, err := os.ReadFile(s.filePath)
	if err != nil {
		if s.log.Enabled() {
			s.log.Errorw("session load failed: file read", "path", s.filePath, "error", err)
		}
		return nil, fmt.Errorf("failed to read session file %s: %w", s.filePath, err)
	}
	if len(ciphertext) == 0 {
		return nil, fmt.Errorf("empty session file")
	}

	keyBytes := normalizeKey(key)
	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}
	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ct := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plain, err := aesGCM.Open(nil, nonce, ct, nil)
	if err != nil {
		if s.log.Enabled() {
			s.log.Errorw("session load failed: decryption error (wrong key or corrupt file)", "error", err)
		}
		return nil, fmt.Errorf("failed to decrypt session (wrong key or corrupt file): %w", err)
	}
	return plain, nil
}

func (s *Store) encryptWrite(plain []byte, key string) error {
	keyBytes := normalizeKey(key)
	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return fmt.Errorf("failed to create cipher: %w", err)
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("failed to create GCM: %w", err)
	}
	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("failed to generate nonce: %w", err)
	}
	ciphertext := aesGCM.Seal(nonce, nonce, plain, nil)

	dir := filepath.Dir(s.filePath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}
	if err := os.WriteFile(s.filePath, ciphertext, 0600); err != nil {
		return fmt.Errorf("failed to write session file %s: %w", s.filePath, err)
	}
	return nil
}

func normalizeKey(key string) []byte {
	keyBytes := []byte(key)
	if len(keyBytes) > 32 {
		return keyBytes[:32]
	}
	if len(keyBytes) < 32 {
		padded := make([]byte, 32)
		copy(padded, keyBytes)
		return padded
	}
	return keyBytes
}
