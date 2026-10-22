package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRetryAfter(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		v    string
		want time.Duration
	}{
		{"12", 12 * time.Second},
		{"", DefaultWait},
		{"soon", DefaultWait},
		{"0", time.Second},
		{"100000", MaxWait},
		{now.Add(90 * time.Second).Format(http.TimeFormat), 90 * time.Second},
	} {
		h := http.Header{}
		if tc.v != "" {
			h.Set("Retry-After", tc.v)
		}
		if got := RetryAfter(h, now); got != tc.want {
			t.Errorf("Retry-After %q: %v, want %v", tc.v, got, tc.want)
		}
	}
}

func TestDo_retriesThrottled(t *testing.T) {
	var waits []time.Duration
	Sleep = func(d time.Duration) { waits = append(waits, d) }
	defer func() { Sleep = time.Sleep }()
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	resp, err := Do(srv.Client(), req, nil)
	if err != nil || resp.StatusCode != http.StatusOK || hits != 3 || len(waits) != 2 || waits[0] != 7*time.Second {
		t.Fatalf("status=%v err=%v hits=%d waits=%v", resp.StatusCode, err, hits, waits)
	}
	resp.Body.Close()

	// Still throttled after MaxAttempts: the 429 is returned to the caller.
	hits = -100
	waits = nil
	resp, _ = Do(srv.Client(), req, nil)
	if resp.StatusCode != http.StatusTooManyRequests || len(waits) != MaxAttempts-1 {
		t.Fatalf("gave up with %d after %d waits", resp.StatusCode, len(waits))
	}
	resp.Body.Close()
}
