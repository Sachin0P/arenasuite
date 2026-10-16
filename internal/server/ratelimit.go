package server

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// rateLimiter allows each IP address a fixed number of requests per minute.
// Counters reset at the start of every minute; simple, but enough to stop a
// script from hammering the server.
type rateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Time
	counter map[string]int
}

func newRateLimiter(perMinute int) *rateLimiter {
	return &rateLimiter{limit: perMinute, counter: map[string]int{}}
}

func (l *rateLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now := time.Now().Truncate(time.Minute); now != l.window {
		l.window = now
		l.counter = map[string]int{}
	}
	l.counter[ip]++
	return l.counter[ip] <= l.limit
}

// clientIP is the visitor's address. Behind a hosting proxy (Render, Fly,
// Caddy...) every request arrives from the proxy, which puts the real
// address first in the X-Forwarded-For header. We only read that header when
// TRUST_PROXY is on, because without a proxy anyone could fake it.
func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			first, _, _ := strings.Cut(fwd, ",")
			return strings.TrimSpace(first)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
