// Package server is the HTTP layer: it maps URLs to handlers, reads forms,
// calls package app for the actual work, and renders Go templates.
//
// Pages are plain GET requests. Every button that changes something is an
// HTML form that POSTs to an "action" URL; the action does its work, stores a
// flash message ("Team created") and redirects back to a page
// (Post/Redirect/Get), so refreshing never re-submits a form.
package server

import (
	"context"
	"errors"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"arenasuite/internal/app"
	"arenasuite/internal/auth"
	"arenasuite/web"
)

type Server struct {
	app       *app.App
	sessions  *auth.Sessions
	google    *auth.Google
	templates map[string]*template.Template
	mux       *http.ServeMux
	limiter   *rateLimiter
}

func New(a *app.App) *Server {
	s := &Server{
		app:       a,
		sessions:  auth.NewSessions(a.Cfg.SessionSecret, a.Cfg.SessionTTL, strings.HasPrefix(a.Cfg.BaseURL, "https://")),
		google:    auth.NewGoogle(a.Cfg.GoogleClientID, a.Cfg.GoogleClientSecret, a.Cfg.BaseURL+"/auth/google/callback"),
		templates: loadTemplates(),
		mux:       http.NewServeMux(),
		limiter:   newRateLimiter(300), // requests per minute per IP
	}
	s.routes()
	return s
}

// ServeHTTP runs for every request: rate limit, then load the signed-in
// user (if any) into the request context, then route.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.limiter.allow(clientIP(r)) {
		http.Error(w, "Too many requests, slow down a little.", http.StatusTooManyRequests)
		return
	}
	// The user is re-loaded from the database on every request, so a role
	// change or suspension takes effect immediately.
	if id := s.sessions.UserID(r); id != 0 {
		if user, err := s.app.UserByID(id); err == nil && user != nil && user.Status == "active" {
			r = r.WithContext(context.WithValue(r.Context(), userKey{}, user))
		}
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	m := s.mux
	static, _ := fs.Sub(web.FS, "static")
	m.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(static))))
	m.HandleFunc("GET /events", s.events)

	// Public pages
	m.HandleFunc("GET /{$}", s.homePage)
	m.HandleFunc("GET /sports", s.categoryPage("SPORTS"))
	m.HandleFunc("GET /academic", s.categoryPage("ACADEMIC"))
	m.HandleFunc("GET /esports", s.categoryPage("ESPORTS"))
	m.HandleFunc("GET /competitions", s.explorePage)
	m.HandleFunc("GET /competitions/{id}", s.competitionPage)
	m.HandleFunc("GET /organizations/{id}", s.organizationPage)
	m.HandleFunc("GET /teams/{id}", s.teamPage)
	m.HandleFunc("GET /find-team", s.findTeamPage)
	m.HandleFunc("GET /organize", s.organizePage)

	// Sign in / out
	m.HandleFunc("GET /auth/login", s.loginPage)
	m.HandleFunc("GET /auth/google", s.googleStart)
	m.HandleFunc("GET /auth/google/callback", s.googleCallback)
	m.HandleFunc("POST /auth/dev-login", s.devLogin)
	m.HandleFunc("POST /auth/logout", s.logout)

	// Signed-in pages
	m.HandleFunc("GET /profile", s.signedIn(s.profilePage))
	m.HandleFunc("POST /profile", s.signedIn(s.saveProfile))
	m.HandleFunc("GET /dashboard", s.signedIn(s.dashboardPage))
	m.HandleFunc("GET /notifications", s.signedIn(s.notificationsPage))

	// Organizer pages
	m.HandleFunc("GET /organizer", s.organizerOnly(s.organizerOverviewPage))
	m.HandleFunc("GET /organizer/competitions", s.organizerOnly(s.organizerCompetitionsPage))
	m.HandleFunc("GET /organizer/competitions/new", s.organizerOnly(s.newCompetitionPage))
	m.HandleFunc("POST /organizer/competitions/new", s.organizerOnly(s.createCompetition))
	m.HandleFunc("GET /organizer/competitions/{id}/edit", s.organizerOnly(s.editCompetitionPage))
	m.HandleFunc("POST /organizer/competitions/{id}/edit", s.organizerOnly(s.updateCompetition))
	m.HandleFunc("GET /organizer/competitions/{id}/manage", s.organizerOnly(s.manageCompetitionPage))
	m.HandleFunc("GET /organizer/settings", s.organizerOnly(s.organizerSettingsPage))

	// Admin pages
	m.HandleFunc("GET /admin", s.adminOnly(s.adminOverviewPage))
	m.HandleFunc("GET /admin/users", s.adminOnly(s.adminUsersPage))
	m.HandleFunc("GET /admin/organizations", s.adminOnly(s.adminOrganizationsPage))
	m.HandleFunc("GET /admin/competitions", s.adminOnly(s.adminCompetitionsPage))
	m.HandleFunc("GET /admin/reports", s.adminOnly(s.adminReportsPage))

	// Form actions (all POST). Each one checks permissions in package app.
	post := func(pattern string, h http.HandlerFunc) { m.HandleFunc("POST "+pattern, s.mustSignIn(h)) }

	post("/organizations", s.createOrganization)
	post("/organizations/{id}", s.updateOrganization)

	post("/competitions/{id}/status", s.setCompetitionStatus)
	post("/competitions/{id}/delete", s.deleteCompetition)
	post("/competitions/{id}/register", s.register)
	post("/competitions/{id}/teams", s.createTeam)
	post("/competitions/{id}/matches", s.createMatch)
	post("/competitions/{id}/matches/generate", s.generateFixtures)
	post("/competitions/{id}/standings/recompute", s.recomputeStandings)
	post("/competitions/{id}/standings/score", s.setAcademicScore)
	post("/competitions/{id}/announcements", s.createAnnouncement)

	post("/registrations/{id}/approve", s.reviewRegistration(true))
	post("/registrations/{id}/reject", s.reviewRegistration(false))
	post("/registrations/{id}/cancel", s.cancelRegistration)

	post("/teams/{id}/update", s.updateTeam)
	post("/teams/{id}/delete", s.deleteTeam)
	post("/teams/{id}/leave", s.leaveTeam)
	post("/teams/{id}/transfer-captain", s.transferCaptain)
	post("/teams/{id}/members/{userId}/remove", s.removeMember)
	post("/teams/{id}/join-request", s.requestToJoin)
	post("/teams/{id}/invite", s.inviteMember)
	post("/join-requests/{id}/accept", s.respondJoinRequest(true))
	post("/join-requests/{id}/reject", s.respondJoinRequest(false))
	post("/join-requests/{id}/cancel", s.cancelJoinRequest)
	post("/invitations/{id}/accept", s.respondInvitation(true))
	post("/invitations/{id}/reject", s.respondInvitation(false))
	post("/invitations/{id}/cancel", s.cancelInvitation)

	post("/find-team/posts", s.createPost)
	post("/find-team/posts/{id}/delete", s.deletePost)

	post("/matches/{id}", s.updateMatch)
	post("/matches/{id}/delete", s.deleteMatch)
	post("/announcements/{id}/delete", s.deleteAnnouncement)

	post("/notifications/{id}/read", s.markRead)
	post("/notifications/read-all", s.markAllRead)
	post("/reports", s.createReport)

	admin := func(pattern string, h http.HandlerFunc) { m.HandleFunc("POST "+pattern, s.adminOnly(h)) }
	admin("/admin/users/{id}/status", s.adminSetUserStatus)
	admin("/admin/users/{id}/role", s.adminSetUserRole)
	admin("/admin/organizations/{id}/verify", s.adminVerifyOrganization)
	admin("/admin/competitions/{id}/hide", s.adminHideCompetition)
	admin("/admin/reports/{id}", s.adminResolveReport)

	// Anything else is a 404 page.
	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { s.notFound(w, r) })
}

// ---------- the signed-in user ----------

type userKey struct{}

// currentUser returns the signed-in user, or nil for visitors.
func currentUser(r *http.Request) *app.User {
	u, _ := r.Context().Value(userKey{}).(*app.User)
	return u
}

// signedIn protects a page: visitors go to the login page, and users who
// haven't filled in roll number + branch are sent to their profile first.
func (s *Server) signedIn(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := currentUser(r)
		if user == nil {
			http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
			return
		}
		if !user.ProfileComplete() && r.URL.Path != "/profile" {
			http.Redirect(w, r, "/profile", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

// mustSignIn protects a form action (no profile redirect, just a message).
func (s *Server) mustSignIn(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if currentUser(r) == nil {
			setFlash(w, "error", "Please sign in first")
			http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

func (s *Server) organizerOnly(next http.HandlerFunc) http.HandlerFunc {
	return s.signedIn(func(w http.ResponseWriter, r *http.Request) {
		if !currentUser(r).IsOrganizer() {
			s.forbidden(w, r)
			return
		}
		next(w, r)
	})
}

func (s *Server) adminOnly(next http.HandlerFunc) http.HandlerFunc {
	return s.signedIn(func(w http.ResponseWriter, r *http.Request) {
		if !currentUser(r).IsAdmin() {
			s.forbidden(w, r)
			return
		}
		next(w, r)
	})
}

// ---------- small helpers used by handlers ----------

// pathID reads a numeric URL segment like {id}.
func pathID(r *http.Request, name string) int64 {
	n, _ := strconv.ParseInt(r.PathValue(name), 10, 64)
	return n
}

// formInt reads a numeric form field; empty or invalid means 0.
func formInt(r *http.Request, name string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue(name)), 10, 64)
	return n
}

// formOptionalInt returns nil for an empty field (e.g. "no score yet").
func formOptionalInt(r *http.Request, name string) *int64 {
	v := strings.TrimSpace(r.FormValue(name))
	if v == "" {
		return nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return nil
	}
	return &n
}

func formText(r *http.Request, name string) string {
	return strings.TrimSpace(r.FormValue(name))
}

// back is where an action redirects to: the form's hidden "next" field if
// it's a local path, otherwise the page the form was on.
func back(r *http.Request) string {
	next := r.FormValue("next")
	if strings.HasPrefix(next, "/") && !strings.HasPrefix(next, "//") {
		return next
	}
	if ref, err := url.Parse(r.Referer()); err == nil && ref.Path != "" {
		return ref.RequestURI()
	}
	return "/"
}

// done finishes a form action: on success show okMsg, on failure show the
// error, then redirect (to "to", or back where the user came from).
func (s *Server) done(w http.ResponseWriter, r *http.Request, err error, okMsg, to string) {
	if to == "" {
		to = back(r)
	}
	if err != nil {
		setFlash(w, "error", userMessage(err))
		http.Redirect(w, r, back(r), http.StatusSeeOther)
		return
	}
	if okMsg != "" {
		setFlash(w, "success", okMsg)
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

// userMessage shows app.Error messages as-is and hides unexpected errors.
func userMessage(err error) string {
	var appErr *app.Error
	if errors.As(err, &appErr) {
		return appErr.Msg
	}
	log.Printf("unexpected error: %v", err)
	return "Something went wrong. Please try again."
}

// pageError renders the right error page for a failed page load.
func (s *Server) pageError(w http.ResponseWriter, r *http.Request, err error) {
	var appErr *app.Error
	if errors.As(err, &appErr) {
		switch appErr.Status {
		case 404:
			s.notFound(w, r)
			return
		case 403:
			s.forbidden(w, r)
			return
		}
	}
	s.render(w, r, http.StatusInternalServerError, "error", "Error", map[string]any{
		"Heading": "Something went wrong", "Message": userMessage(err),
	})
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusNotFound, "error", "Not found", map[string]any{
		"Heading": "Page not found", "Message": "The page you're looking for doesn't exist or isn't public.",
	})
}

func (s *Server) forbidden(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusForbidden, "error", "Not allowed", map[string]any{
		"Heading": "Not allowed", "Message": "You don't have permission to view this page.",
	})
}
