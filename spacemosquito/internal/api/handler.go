package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/vkh/spacemosquito/internal/config"
	"github.com/vkh/spacemosquito/internal/session"
	"github.com/vkh/spacemosquito/pkg/logging"
)

type Handler struct {
	store *session.Store
	cfg   *config.Config
	log   logging.Sugar
}

func New(store *session.Store, cfg *config.Config, log logging.Sugar) *Handler {
	return &Handler{
		store: store,
		cfg:   cfg,
		log:   log,
	}
}

func LoggingMiddleware(next http.Handler, log logging.Sugar) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		requestID := uuid.New().String()[:8]

		log.Debugw("request started",
			"method", r.Method,
			"path", r.URL.Path,
			"remote_addr", r.RemoteAddr,
			"user_agent", r.UserAgent(),
			"request_id", requestID)

		status := writeResponse(w, r, next)

		duration := time.Since(start)
		log.Debugw("request completed",
			"method", r.Method,
			"path", r.URL.Path,
			"status", status,
			"duration_ms", duration.Milliseconds(),
			"remote_addr", r.RemoteAddr,
			"user_agent", r.UserAgent(),
			"request_id", requestID)
	})
}

func writeResponse(w http.ResponseWriter, r *http.Request, next http.Handler) int {
	rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
	next.ServeHTTP(rw, r)
	return rw.status
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

type createSessionRequest struct {
	ConfluenceURL string           `json:"confluence_url"`
	Cookies       []session.Cookie `json:"cookies"`
}

func (h *Handler) CreateSession(w http.ResponseWriter, r *http.Request) {
	var req createSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.Warnw("create session: invalid request body", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error":   "invalid request body",
			"details": err.Error(),
		})
		return
	}

	if req.ConfluenceURL == "" {
		h.log.Warn("create session: missing confluence_url")
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "confluence_url is required"})
		return
	}

	if len(req.Cookies) == 0 {
		h.log.Warn("create session: no cookies provided")
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "at least one cookie is required"})
		return
	}

	if session.HostnameKey(req.ConfluenceURL) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "confluence_url must include a hostname"})
		return
	}

	sess := &session.Session{
		ConfluenceURL: req.ConfluenceURL,
		Cookies:       req.Cookies,
		CapturedAt:    time.Now(),
	}
	sess.SetLogger(h.store.GetLogger())

	encKey := h.cfg.Session.EncryptionKey
	if encKey == "" {
		h.log.Error("create session: encryption key not configured")
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "encryption key not configured",
		})
		return
	}

	if err := h.store.Upsert(sess, encKey); err != nil {
		h.log.Errorw("create session: store upsert failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to save session: " + err.Error(),
		})
		return
	}

	h.log.Infow("session upserted",
		"url", req.ConfluenceURL,
		"host", session.HostnameKey(req.ConfluenceURL),
		"cookie_count", len(req.Cookies))

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"message":        "session saved",
		"confluence_url": req.ConfluenceURL,
		"cookie_count":   len(req.Cookies),
	})
}

func (h *Handler) DeleteSession(w http.ResponseWriter, r *http.Request) {
	if !h.store.HasSession() {
		h.log.Info("delete session: no session stored")
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no session stored"})
		return
	}

	if err := h.store.Delete(); err != nil {
		h.log.Errorw("delete session: store delete failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to delete session: " + err.Error(),
		})
		return
	}

	h.log.Info("session deleted")
	writeJSON(w, http.StatusOK, map[string]string{"message": "session deleted"})
}

func (h *Handler) SessionStatus(w http.ResponseWriter, r *http.Request) {
	if !h.store.HasSession() {
		h.log.Infow("session status",
			"valid", false,
			"exists", false,
			"decision", "no_session")
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"valid":   false,
			"message": "no session stored",
			"exists":  false,
		})
		return
	}

	encKey := h.cfg.Session.EncryptionKey
	confluenceURL := r.URL.Query().Get("url")

	var sess *session.Session
	var err error
	if confluenceURL != "" {
		sess, err = h.store.GetForURL(encKey, confluenceURL)
	} else {
		sess, err = h.store.Load(encKey)
	}
	if err != nil {
		h.log.Errorw("session status: load failed", "error", err)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"valid":   false,
			"message": err.Error(),
			"exists":  h.store.HasSession(),
		})
		return
	}

	host := session.HostnameKey(sess.ConfluenceURL)
	ttl := session.ValidateTTL
	status := map[string]interface{}{
		"exists":  true,
		"valid":   false,
		"message": "session requires validation",
		"host":    host,
	}
	decision := "requires_validation"

	if sess.ValidatedAt != nil {
		sinceValidation := time.Since(*sess.ValidatedAt)
		status["validated_at"] = sess.ValidatedAt.UTC().Format(time.RFC3339)
		status["validated_age_seconds"] = int(sinceValidation.Seconds())
		if sinceValidation > ttl {
			status["message"] = "session validation expired"
			status["valid"] = false
			decision = "ttl_expired"
			h.log.Warnw("session status",
				"valid", false,
				"exists", true,
				"host", host,
				"decision", decision,
				"validated_at", sess.ValidatedAt,
				"validated_age_seconds", int(sinceValidation.Seconds()),
				"ttl_seconds", int(ttl.Seconds()))
		} else {
			status["valid"] = true
			status["message"] = "session is valid"
			decision = "ttl_trust"
			h.log.Infow("session status",
				"valid", true,
				"exists", true,
				"host", host,
				"decision", decision,
				"validated_at", sess.ValidatedAt,
				"validated_age_seconds", int(sinceValidation.Seconds()),
				"ttl_seconds", int(ttl.Seconds()))
		}
	} else {
		h.log.Infow("session status",
			"valid", false,
			"exists", true,
			"host", host,
			"decision", decision)
	}

	if sess.IsExpired(ttl) && sess.ValidatedAt == nil {
		h.log.Warnw("session is stale", "host", host, "captured_at", sess.CapturedAt)
		status["message"] = "session is stale"
	}

	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) ValidateSession(w http.ResponseWriter, r *http.Request) {
	encKey := h.cfg.Session.EncryptionKey

	var req struct {
		ConfluenceURL string `json:"confluence_url"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	var sess *session.Session
	var err error
	if req.ConfluenceURL != "" {
		sess, err = h.store.GetForURL(encKey, req.ConfluenceURL)
	} else {
		sess, err = h.store.Load(encKey)
	}
	if err != nil {
		h.log.Errorw("validate session: load failed", "error", err)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"valid":   false,
			"message": err.Error(),
		})
		return
	}

	timeout := 10
	if h.cfg.MCP.Timeout > 0 && h.cfg.MCP.Timeout < 60 {
		timeout = h.cfg.MCP.Timeout
	}

	host := session.HostnameKey(sess.ConfluenceURL)
	validateURL := req.ConfluenceURL
	sess.SetLogger(h.log)
	h.log.Infow("validate session: start", "host", host, "url", validateURL)

	result, err := sess.ValidateWithConfluence(validateURL, timeout, r.RemoteAddr)
	if err != nil {
		h.log.Errorw("validate session: unexpected error", "host", host, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "validation failed: " + err.Error(),
		})
		return
	}

	if result.Valid {
		if err := h.store.Upsert(sess, encKey); err != nil {
			h.log.Errorw("validate session: failed to persist validated session", "host", host, "error", err)
		} else {
			h.log.Infow("validate session: ok",
				"host", host,
				"flavor", result.Flavor,
				"message", result.Message,
				"validated_at_cleared", false)
		}
	} else {
		hadValidation := sess.ValidatedAt != nil
		sess.ClearValidation()
		if err := h.store.Upsert(sess, encKey); err != nil {
			h.log.Errorw("validate session: failed to persist invalidation", "host", host, "error", err)
		}
		h.log.Warnw("validate session: failed",
			"host", host,
			"message", result.Message,
			"remote_addr", r.RemoteAddr,
			"validated_at_cleared", hadValidation)
	}

	writeJSON(w, http.StatusOK, result)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
