package server

import (
	"net"
	"net/http"
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

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
