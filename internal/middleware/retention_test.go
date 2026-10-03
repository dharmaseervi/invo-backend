package middleware

import (
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// An idle key must not be forgotten before its bucket would have refilled, or waiting
// out the sweep is a way around the limit.
func TestRetentionCoversRefill(t *testing.T) {
	cases := []struct {
		name  string
		r     rate.Limit
		burst int
		want  time.Duration
	}{
		// The email quota: 8 an hour, burst 4 — 30 minutes to refill.
		{"email quota", rate.Limit(8.0 / 3600), 4, 30 * time.Minute},
		// Credential limiter: 10 a minute, burst 10 — a minute, so the floor applies.
		{"credential", rate.Limit(10.0 / 60), 10, 10 * time.Minute},
		// General per-IP: 5 a second — seconds, so the floor applies.
		{"per-ip", rate.Limit(5), 10, 10 * time.Minute},
	}
	for _, c := range cases {
		l := newKeyedRateLimiter(c.r, c.burst)
		if got := l.retention(); got != c.want {
			t.Errorf("%s: retention = %v, want %v", c.name, got, c.want)
		}
	}
}
