package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"arenasuite/internal/models"

	"golang.org/x/time/rate"
)

func commonHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Images can come from any https URL (Google profile pictures and
		// banner links), and the templates use a few inline style attributes.
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; img-src 'self' https: data:; style-src 'self' 'unsafe-inline'")

		w.Header().Set("Referrer-Policy", "origin-when-cross-origin")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "deny")
		w.Header().Set("X-XSS-Protection", "0")

		w.Header().Set("Server", "Go")

		next.ServeHTTP(w, r)
	})
}

func (app *application) logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var (
			ip     = app.clientIP(r)
			proto  = r.Proto
			method = r.Method
			uri    = r.URL.RequestURI()
		)

		app.logger.Info("received request", "ip", ip, "proto", proto, "method", method, "uri", uri)

		next.ServeHTTP(w, r)
	})
}

func (app *application) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Create a deferred function (which will always be run in the event
		// of a panic as Go unwinds the stack).
		defer func() {
			// Use the builtin recover function to check if there has been a
			// panic or not.
			pv := recover()
			if pv != nil {
				// Set a "Connection: close" header on the response.
				w.Header().Set("Connection", "close")
				// Call the app.serverError helper method to return a 500
				// Internal Server response.
				app.serverError(w, r, fmt.Errorf("%v", pv))
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// rateLimit allows each client IP address a steady rate of requests, with
// some room for bursts, which is enough to stop a script from hammering the
// server.
func (app *application) rateLimit(next http.Handler) http.Handler {
	// Define a client struct to hold the rate limiter and last seen time for
	// each client.
	type client struct {
		limiter  *rate.Limiter
		lastSeen time.Time
	}

	var (
		mu      sync.Mutex
		clients = make(map[string]*client)
	)

	// Launch a background goroutine which removes old entries from the
	// clients map once every minute.
	go func() {
		for {
			time.Sleep(time.Minute)

			mu.Lock()

			for ip, client := range clients {
				if time.Since(client.lastSeen) > 3*time.Minute {
					delete(clients, ip)
				}
			}

			mu.Unlock()
		}
	}()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !app.config.limiter.enabled {
			next.ServeHTTP(w, r)
			return
		}

		ip := app.clientIP(r)

		mu.Lock()

		if _, found := clients[ip]; !found {
			clients[ip] = &client{
				limiter: rate.NewLimiter(rate.Limit(app.config.limiter.rps), app.config.limiter.burst),
			}
		}

		clients[ip].lastSeen = time.Now()

		if !clients[ip].limiter.Allow() {
			mu.Unlock()
			http.Error(w, "Too many requests, slow down a little.", http.StatusTooManyRequests)
			return
		}

		mu.Unlock()

		next.ServeHTTP(w, r)
	})
}

// clientIP is the visitor's address. Behind a hosting proxy (Render, Fly,
// Caddy...) every request arrives from the proxy, which puts the real
// address first in the X-Forwarded-For header. We only read that header when
// -trust-proxy is on, because without a proxy anyone could fake it.
func (app *application) clientIP(r *http.Request) string {
	if app.config.limiter.trustProxy {
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

// preventCSRF rejects cross-origin POST requests. It uses the browser's
// Sec-Fetch-Site header (and the Origin header as a fallback), so the forms
// don't need hidden CSRF token fields.
func preventCSRF(next http.Handler) http.Handler {
	cop := http.NewCrossOriginProtection()
	return cop.Handler(next)
}

// authenticate loads the signed-in user from the database and adds them to
// the request context. The user is re-loaded on every request, so a role
// change or suspension takes effect immediately.
func (app *application) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Retrieve the authenticatedUserID value from the session using the
		// GetInt64() method. This will return the zero value for an int64 (0)
		// if no "authenticatedUserID" value is in the session -- in which case
		// we call the next handler in the chain as normal and return.
		id := app.sessionManager.GetInt64(r.Context(), "authenticatedUserID")
		if id == 0 {
			next.ServeHTTP(w, r)
			return
		}

		user, err := app.users.Get(id)
		if err != nil {
			if errors.Is(err, models.ErrNoRecord) {
				next.ServeHTTP(w, r)
			} else {
				app.serverError(w, r, err)
			}
			return
		}

		// A suspended user is treated as signed out.
		if user.Status != "active" {
			next.ServeHTTP(w, r)
			return
		}

		r = app.contextSetUser(r, user)

		next.ServeHTTP(w, r)
	})
}

func (app *application) requireAuthentication(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If the user is not authenticated, redirect them to the login page
		// and return from the middleware chain so that no subsequent handlers
		// in the chain are executed.
		if !app.isAuthenticated(r) {
			if r.Method != http.MethodGet {
				app.sessionManager.Put(r.Context(), "flashError", "Please sign in first")
			}
			http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
			return
		}

		// Otherwise set the "Cache-Control: no-store" header so that pages
		// which require authentication are not stored in the users browser
		// cache (or other intermediary cache).
		w.Header().Add("Cache-Control", "no-store")

		// And call the next handler in the chain.
		next.ServeHTTP(w, r)
	})
}

// requireCompleteProfile sends users who haven't filled in their roll number
// and branch to their profile page first. It must run after
// requireAuthentication.
func (app *application) requireCompleteProfile(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !app.contextGetUser(r).ProfileComplete() {
			http.Redirect(w, r, "/profile", http.StatusSeeOther)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// requireOrganizer and requireAdmin check the user's role. They must run
// after requireAuthentication.
func (app *application) requireOrganizer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !app.contextGetUser(r).IsOrganizer() {
			app.clientError(w, r, http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (app *application) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !app.contextGetUser(r).IsAdmin() {
			app.clientError(w, r, http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}
