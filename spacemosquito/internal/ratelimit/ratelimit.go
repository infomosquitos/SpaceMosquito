// Package ratelimit handles Confluence throttling: HTTP 429 with a
// Retry-After header (seconds or an HTTP date) asks the client to wait.
package ratelimit

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultWait applies when a 429 has no usable Retry-After.
	DefaultWait = 30 * time.Second
	// MaxWait caps a single wait, so a bad header cannot stall a crawl.
	MaxWait = 5 * time.Minute
	// MaxAttempts bounds requests per call to Do (first try + retries).
	MaxAttempts = 4
)

// Sleep is replaced in tests.
var Sleep = time.Sleep

// RetryAfter returns how long a 429 response asks to wait.
func RetryAfter(h http.Header, now time.Time) time.Duration {
	v := strings.TrimSpace(h.Get("Retry-After"))
	wait := DefaultWait
	if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
		wait = time.Duration(secs) * time.Second
	} else if t, err := http.ParseTime(v); err == nil {
		wait = t.Sub(now)
	}
	switch {
	case wait < time.Second:
		wait = time.Second
	case wait > MaxWait:
		wait = MaxWait
	}
	return wait
}

// Do sends a request without a body and, while Confluence answers 429,
// waits as asked (Retry-After) and tries again, up to MaxAttempts requests.
// onThrottle, if set, is told each wait (for logging).
func Do(client *http.Client, req *http.Request, onThrottle func(wait time.Duration)) (*http.Response, error) {
	for attempt := 1; ; attempt++ {
		resp, err := client.Do(req)
		if err != nil || resp.StatusCode != http.StatusTooManyRequests || attempt >= MaxAttempts {
			return resp, err
		}
		wait := RetryAfter(resp.Header, time.Now())
		resp.Body.Close()
		if onThrottle != nil {
			onThrottle(wait)
		}
		Sleep(wait)
	}
}
