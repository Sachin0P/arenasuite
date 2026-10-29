package main

import (
	"context"
	"net/http"

	"arenasuite/internal/models"
)

// Define a custom contextKey type, with the underlying type string, so our
// keys can't collide with keys set by other packages.
type contextKey string

const authenticatedUserContextKey = contextKey("authenticatedUser")

// contextSetUser returns a new copy of the request with the signed-in user
// added to the request context.
func (app *application) contextSetUser(r *http.Request, user *models.User) *http.Request {
	ctx := context.WithValue(r.Context(), authenticatedUserContextKey, user)
	return r.WithContext(ctx)
}

// contextGetUser returns the signed-in user, or nil for a visitor. The
// authenticate middleware only adds active users, so a suspended user is
// treated as signed out.
func (app *application) contextGetUser(r *http.Request) *models.User {
	user, ok := r.Context().Value(authenticatedUserContextKey).(*models.User)
	if !ok {
		return nil
	}
	return user
}

// isAuthenticated returns true if the current request is from an
// authenticated user, otherwise false.
func (app *application) isAuthenticated(r *http.Request) bool {
	return app.contextGetUser(r) != nil
}
