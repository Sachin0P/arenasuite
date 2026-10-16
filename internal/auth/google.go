package auth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Google signs users in with the OAuth "authorization code" flow, done
// entirely on the server with net/http (no Google JavaScript needed):
//
//  1. We redirect the browser to Google's sign-in page (AuthURL).
//  2. Google redirects back to /auth/google/callback?code=...
//  3. We send that code to Google's token endpoint (Exchange) and get back
//     an ID token describing the user.
type Google struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string // must also be listed in Google Cloud Console
	client       *http.Client
}

// GoogleUser is the part of Google's ID token we use.
type GoogleUser struct {
	Sub           string `json:"sub"` // Google's permanent id for the account
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
	Audience      string `json:"aud"`
	Issuer        string `json:"iss"`
	Expires       int64  `json:"exp"`
}

func NewGoogle(clientID, clientSecret, redirectURL string) *Google {
	return &Google{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		client:       &http.Client{Timeout: 10 * time.Second},
	}
}

func (g *Google) Configured() bool {
	return g.ClientID != "" && g.ClientSecret != ""
}

// AuthURL is Google's sign-in page. "state" is a random value we also store
// in a cookie; checking it on the way back stops forged callbacks.
// "hd" asks Google to suggest accounts from our college domain (we still
// check the domain ourselves afterwards).
func (g *Google) AuthURL(state, domain string) string {
	q := url.Values{
		"client_id":     {g.ClientID},
		"redirect_uri":  {g.RedirectURL},
		"response_type": {"code"},
		"scope":         {"openid email profile"},
		"state":         {state},
		"hd":            {domain},
		"prompt":        {"select_account"},
	}
	return "https://accounts.google.com/o/oauth2/v2/auth?" + q.Encode()
}

// Exchange trades the one-time code for the user's details.
func (g *Google) Exchange(code string) (*GoogleUser, error) {
	resp, err := g.client.PostForm("https://oauth2.googleapis.com/token", url.Values{
		"code":          {code},
		"client_id":     {g.ClientID},
		"client_secret": {g.ClientSecret},
		"redirect_uri":  {g.RedirectURL},
		"grant_type":    {"authorization_code"},
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var body struct {
		IDToken string `json:"id_token"`
		Error   string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK || body.IDToken == "" {
		return nil, fmt.Errorf("google token exchange failed: %s", body.Error)
	}

	// The ID token is a JWT: header.payload.signature. Because we received it
	// directly from Google over HTTPS (not from the browser), Google's docs
	// say the signature check can be skipped. We still check who it's for.
	parts := strings.Split(body.IDToken, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed ID token")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	var user GoogleUser
	if err := json.Unmarshal(raw, &user); err != nil {
		return nil, err
	}
	if user.Audience != g.ClientID {
		return nil, errors.New("ID token was issued for a different app")
	}
	if user.Issuer != "https://accounts.google.com" && user.Issuer != "accounts.google.com" {
		return nil, errors.New("ID token has an unexpected issuer")
	}
	if time.Now().Unix() > user.Expires {
		return nil, errors.New("ID token has expired")
	}
	return &user, nil
}
