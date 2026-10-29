package main

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"arenasuite/internal/auth"
	"arenasuite/internal/models"
)

func (app *application) userLogin(w http.ResponseWriter, r *http.Request) {
	if app.isAuthenticated(r) {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	data := app.newTemplateData(r)
	data.GoogleReady = app.google.Configured()
	data.Domain = app.config.allowedEmailDomain
	data.DevLogin = app.config.devLogin

	if app.config.devLogin {
		users, err := app.users.All()
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		data.Users = users
	}

	app.render(w, r, http.StatusOK, "login.tmpl", data)
}

// userLoginGoogle sends the browser to Google's sign-in page.
func (app *application) userLoginGoogle(w http.ResponseWriter, r *http.Request) {
	if !app.google.Configured() {
		app.sessionManager.Put(r.Context(), "flashError", "Google sign-in isn't set up yet (missing GOOGLE_CLIENT_ID / GOOGLE_CLIENT_SECRET)")
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	// The random state goes in the session and in the URL; the callback
	// checks they match, so nobody can trick a browser into a login it
	// didn't start.
	state := auth.NewState()
	app.sessionManager.Put(r.Context(), "oauthState", state)

	http.Redirect(w, r, app.google.AuthURL(state, app.config.allowedEmailDomain), http.StatusFound)
}

func (app *application) userLoginGoogleCallback(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()

	state := app.sessionManager.PopString(r.Context(), "oauthState")
	if state == "" || state != qs.Get("state") {
		app.loginFailed(w, r, "Sign-in expired, please try again")
		return
	}

	if qs.Get("code") == "" {
		app.loginFailed(w, r, "Sign-in was cancelled")
		return
	}

	googleUser, err := app.google.Exchange(qs.Get("code"))
	if err != nil {
		app.logger.Error(err.Error(), "uri", r.URL.Path)
		app.loginFailed(w, r, "Could not verify your Google account")
		return
	}

	if !googleUser.EmailVerified {
		app.loginFailed(w, r, "Google email is not verified")
		return
	}

	email := strings.ToLower(googleUser.Email)
	_, domain, _ := strings.Cut(email, "@")
	if domain != app.config.allowedEmailDomain {
		app.loginFailed(w, r, "Only @"+app.config.allowedEmailDomain+" accounts may sign in")
		return
	}

	// Emails on the admin list become admin when they sign in, on the first
	// login or any later one. It never demotes: removing an email from the
	// list later doesn't take admin away.
	isBootstrapAdmin := slices.Contains(app.config.adminEmails, email)

	user, err := app.users.GetByGoogleIDOrEmail(googleUser.Sub, email)
	switch {
	case err == nil:
		err = app.users.UpdateGoogleProfile(user.ID, googleUser.Name, googleUser.Picture)
		if err != nil {
			app.serverError(w, r, err)
			return
		}

		if isBootstrapAdmin && user.Role != "admin" {
			err = app.users.SetRole(user.ID, "admin")
			if err != nil {
				app.serverError(w, r, err)
				return
			}
		}

	case errors.Is(err, models.ErrNoRecord):
		// Brand-new users start as participants (unless they're a bootstrap
		// admin). Everyone signing in is from the same college, so fill it
		// in for them.
		role := "participant"
		if isBootstrapAdmin {
			role = "admin"
		}

		user = &models.User{Status: "active"}
		user.ID, err = app.users.Insert(googleUser.Sub, googleUser.Name, email, googleUser.Picture, role, app.config.defaultCollege)
		if err != nil {
			app.serverError(w, r, err)
			return
		}

	default:
		app.serverError(w, r, err)
		return
	}

	if user.Status == "suspended" {
		app.loginFailed(w, r, "Your account is suspended")
		return
	}

	// Use the RenewToken() method on the current session to change the
	// session ID. It's good practice to generate a new session ID when the
	// authentication state or privilege levels changes for the user (e.g.
	// login and logout operations).
	err = app.sessionManager.RenewToken(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Add the ID of the current user to the session, so that they are now
	// 'logged in'.
	app.sessionManager.Put(r.Context(), "authenticatedUserID", user.ID)

	// Land on the dashboard; it sends new users to their profile first.
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// loginFailed sends the user back to the login page with an error message.
func (app *application) loginFailed(w http.ResponseWriter, r *http.Request, message string) {
	app.sessionManager.Put(r.Context(), "flashError", message)
	http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
}

// userLoginDevPost signs in as any user without Google. It only works with
// the -dev-login flag.
func (app *application) userLoginDevPost(w http.ResponseWriter, r *http.Request) {
	if !app.config.devLogin {
		app.notFound(w, r)
		return
	}

	id, err := strconv.ParseInt(r.PostFormValue("userId"), 10, 64)
	if err != nil {
		app.clientError(w, r, http.StatusBadRequest)
		return
	}

	user, err := app.users.Get(id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.loginFailed(w, r, "No such user")
		} else {
			app.serverError(w, r, err)
		}
		return
	}

	err = app.sessionManager.RenewToken(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.sessionManager.Put(r.Context(), "authenticatedUserID", user.ID)
	app.sessionManager.Put(r.Context(), "flash", "Signed in as "+user.Name)

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (app *application) userLogoutPost(w http.ResponseWriter, r *http.Request) {
	// Use the RenewToken() method on the current session to change the
	// session ID again.
	err := app.sessionManager.RenewToken(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Remove the authenticatedUserID from the session data so that the user
	// is 'logged out'.
	app.sessionManager.Remove(r.Context(), "authenticatedUserID")

	// Add a flash message to the session to confirm to the user that they've
	// been logged out.
	app.sessionManager.Put(r.Context(), "flash", "You've been signed out")

	http.Redirect(w, r, "/", http.StatusSeeOther)
}
