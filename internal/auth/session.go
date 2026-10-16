// Package auth handles signing in with Google and the login cookie.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const sessionCookie = "session"

// Sessions creates and checks the login cookie.
//
// The cookie value is "<userID>.<expiresUnix>.<signature>", where the
// signature is an HMAC of the first two parts using a server secret. Nobody
// can forge or edit it without knowing the secret. Only the user id is stored;
// the user row is re-loaded on every request, so role changes and
// suspensions take effect immediately.
type Sessions struct {
	secret []byte
	ttl    time.Duration
	secure bool // send the cookie over HTTPS only (true when BASE_URL is https)
}

func NewSessions(secret string, ttl time.Duration, secure bool) *Sessions {
	return &Sessions{secret: []byte(secret), ttl: ttl, secure: secure}
}

func (s *Sessions) sign(payload string) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Token returns a signed session token for a user. Exposed so the seed
// command can print ready-to-use tokens.
func (s *Sessions) Token(userID int64) string {
	payload := fmt.Sprintf("%d.%d", userID, time.Now().Add(s.ttl).Unix())
	return payload + "." + s.sign(payload)
}

// Start sets the login cookie after a successful sign-in.
func (s *Sessions) Start(w http.ResponseWriter, userID int64) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    s.Token(userID),
		Path:     "/",
		MaxAge:   int(s.ttl.Seconds()),
		HttpOnly: true, // JavaScript can't read it
		Secure:   s.secure,
		// Lax: the browser won't send the cookie on POSTs from other sites,
		// which protects our forms against cross-site request forgery.
		SameSite: http.SameSiteLaxMode,
	})
}

// End clears the login cookie.
func (s *Sessions) End(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
}

// UserID returns the signed-in user's id, or 0 if the cookie is missing,
// tampered with or expired.
func (s *Sessions) UserID(r *http.Request) int64 {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return 0
	}
	parts := strings.Split(c.Value, ".")
	if len(parts) != 3 {
		return 0
	}
	payload := parts[0] + "." + parts[1]
	if !hmac.Equal([]byte(parts[2]), []byte(s.sign(payload))) {
		return 0
	}
	expires, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().Unix() > expires {
		return 0
	}
	id, _ := strconv.ParseInt(parts[0], 10, 64)
	return id
}

// RandomString returns a random URL-safe string, used for the OAuth "state".
func RandomString() string {
	b := make([]byte, 24)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
