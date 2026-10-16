package server

import (
	"log"
	"net/http"

	"arenasuite/internal/auth"
	"arenasuite/internal/realtime"
)

func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	if currentUser(r) != nil {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	data := map[string]any{
		"GoogleReady": s.google.Configured(),
		"Domain":      s.app.Cfg.AllowedEmailDomain,
		"DevLogin":    s.app.Cfg.DevLogin,
	}
	if s.app.Cfg.DevLogin {
		users, err := s.app.AllUsers()
		if err != nil {
			s.pageError(w, r, err)
			return
		}
		data["Users"] = users
	}
	s.render(w, r, http.StatusOK, "login", "Sign in", data)
}

// googleStart sends the browser to Google's sign-in page.
func (s *Server) googleStart(w http.ResponseWriter, r *http.Request) {
	if !s.google.Configured() {
		setFlash(w, "error", "Google sign-in isn't set up yet (missing GOOGLE_CLIENT_ID / GOOGLE_CLIENT_SECRET)")
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}
	// The random state goes in a cookie and in the URL; the callback checks
	// they match, so nobody can trick a browser into a login it didn't start.
	state := auth.RandomString()
	http.SetCookie(w, &http.Cookie{Name: "oauth_state", Value: state, Path: "/auth", MaxAge: 600, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, s.google.AuthURL(state, s.app.Cfg.AllowedEmailDomain), http.StatusFound)
}

func (s *Server) googleCallback(w http.ResponseWriter, r *http.Request) {
	fail := func(msg string) {
		setFlash(w, "error", msg)
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
	}
	cookie, err := r.Cookie("oauth_state")
	if err != nil || cookie.Value == "" || cookie.Value != r.URL.Query().Get("state") {
		fail("Sign-in expired, please try again")
		return
	}
	if r.URL.Query().Get("code") == "" {
		fail("Sign-in was cancelled")
		return
	}

	googleUser, err := s.google.Exchange(r.URL.Query().Get("code"))
	if err != nil {
		log.Printf("google login: %v", err)
		fail("Could not verify your Google account")
		return
	}
	user, err := s.app.LoginWithGoogle(googleUser)
	if err != nil {
		fail(userMessage(err))
		return
	}
	if user.Status == "suspended" {
		fail("Your account is suspended")
		return
	}
	s.sessions.Start(w, user.ID)
	// Land on the dashboard; it sends new users to their profile first.
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// devLogin signs in as any user without Google. Only works with DEV_LOGIN=true.
func (s *Server) devLogin(w http.ResponseWriter, r *http.Request) {
	if !s.app.Cfg.DevLogin {
		s.notFound(w, r)
		return
	}
	user, err := s.app.UserByID(formInt(r, "userId"))
	if err != nil || user == nil {
		s.done(w, r, err, "", "/auth/login")
		return
	}
	s.sessions.Start(w, user.ID)
	setFlash(w, "success", "Signed in as "+user.Name)
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	s.sessions.End(w)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// events is the live-update stream (Server-Sent Events). A signed-in tab
// joins its user's room; a competition page also joins that competition's
// room via ?competition=ID.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	var rooms []string
	if user := currentUser(r); user != nil {
		rooms = append(rooms, realtime.UserRoom(user.ID))
	}
	if id := formInt(r, "competition"); id != 0 {
		// Only join if this visitor is allowed to see the competition.
		if _, err := s.app.CompetitionByID(currentUser(r), id); err == nil {
			rooms = append(rooms, realtime.CompetitionRoom(id))
		}
	}
	if len(rooms) == 0 {
		w.WriteHeader(http.StatusNoContent) // tells EventSource to stop retrying
		return
	}
	s.app.Hub.Serve(w, r, rooms)
}
