package main

import (
	"net/http"

	"arenasuite/ui"

	"github.com/justinas/alice"
)

// The routes() method returns a servemux containing our application routes.
//
// Pages are plain GET requests. Every button that changes something is an
// HTML form that POSTs to an action URL; the action does its work, stores a
// flash message ("Team created") and redirects back to a page.
func (app *application) routes() http.Handler {
	mux := http.NewServeMux()

	// Use the http.FileServerFS() function to create a HTTP handler which
	// serves the embedded files in ui.Files.
	mux.Handle("GET /static/", http.FileServerFS(ui.Files))

	// Used by the hosting platform to check the app is up. It doesn't need a
	// session, so it isn't wrapped in the dynamic middleware.
	mux.HandleFunc("GET /healthz", app.healthz)

	// Unprotected application routes using the "dynamic" middleware chain.
	dynamic := alice.New(app.sessionManager.LoadAndSave, preventCSRF, app.authenticate)

	mux.Handle("GET /{$}", dynamic.ThenFunc(app.home))
	mux.Handle("GET /events", dynamic.ThenFunc(app.events))
	mux.Handle("GET /competitions", dynamic.ThenFunc(app.competitionList))
	mux.Handle("GET /sports", dynamic.ThenFunc(app.competitionList))
	mux.Handle("GET /academic", dynamic.ThenFunc(app.competitionList))
	mux.Handle("GET /esports", dynamic.ThenFunc(app.competitionList))
	mux.Handle("GET /competitions/{id}", dynamic.ThenFunc(app.competitionView))
	mux.Handle("GET /organizations/{id}", dynamic.ThenFunc(app.organizationView))
	mux.Handle("GET /teams/{id}", dynamic.ThenFunc(app.teamView))
	mux.Handle("GET /find-team", dynamic.ThenFunc(app.findTeam))
	mux.Handle("GET /organize", dynamic.ThenFunc(app.organize))

	mux.Handle("GET /auth/login", dynamic.ThenFunc(app.userLogin))
	mux.Handle("GET /auth/google", dynamic.ThenFunc(app.userLoginGoogle))
	mux.Handle("GET /auth/google/callback", dynamic.ThenFunc(app.userLoginGoogleCallback))
	mux.Handle("POST /auth/dev-login", dynamic.ThenFunc(app.userLoginDevPost))

	// Protected (authenticated-only) application routes, using a new
	// "protected" middleware chain which includes the requireAuthentication
	// middleware.
	protected := dynamic.Append(app.requireAuthentication)

	mux.Handle("POST /auth/logout", protected.ThenFunc(app.userLogoutPost))
	mux.Handle("GET /profile", protected.ThenFunc(app.userProfile))
	mux.Handle("POST /profile", protected.ThenFunc(app.userProfilePost))

	// Signed-in pages that need a complete profile (roll number and branch).
	onboarded := protected.Append(app.requireCompleteProfile)

	mux.Handle("GET /dashboard", onboarded.ThenFunc(app.userDashboard))
	mux.Handle("GET /notifications", onboarded.ThenFunc(app.notificationList))

	organizer := onboarded.Append(app.requireOrganizer)

	mux.Handle("GET /organizer", organizer.ThenFunc(app.organizerOverview))
	mux.Handle("GET /organizer/competitions", organizer.ThenFunc(app.organizerCompetitionList))
	mux.Handle("GET /organizer/competitions/new", organizer.ThenFunc(app.competitionCreate))
	mux.Handle("POST /organizer/competitions/new", organizer.ThenFunc(app.competitionCreatePost))
	mux.Handle("GET /organizer/competitions/{id}/edit", organizer.ThenFunc(app.competitionEdit))
	mux.Handle("POST /organizer/competitions/{id}/edit", organizer.ThenFunc(app.competitionEditPost))
	mux.Handle("GET /organizer/competitions/{id}/manage", organizer.ThenFunc(app.competitionManage))
	mux.Handle("GET /organizer/settings", organizer.ThenFunc(app.organizerSettings))

	admin := onboarded.Append(app.requireAdmin)

	mux.Handle("GET /admin", admin.ThenFunc(app.adminOverview))
	mux.Handle("GET /admin/users", admin.ThenFunc(app.adminUserList))
	mux.Handle("GET /admin/organizations", admin.ThenFunc(app.adminOrganizationList))
	mux.Handle("GET /admin/competitions", admin.ThenFunc(app.adminCompetitionList))
	mux.Handle("GET /admin/reports", admin.ThenFunc(app.adminReportList))
	mux.Handle("POST /admin/users/{id}/status", admin.ThenFunc(app.adminUserStatusPost))
	mux.Handle("POST /admin/users/{id}/role", admin.ThenFunc(app.adminUserRolePost))
	mux.Handle("POST /admin/organizations/{id}/verify", admin.ThenFunc(app.adminOrganizationVerifyPost))
	mux.Handle("POST /admin/competitions/{id}/hide", admin.ThenFunc(app.adminCompetitionHidePost))
	mux.Handle("POST /admin/reports/{id}", admin.ThenFunc(app.adminReportResolvePost))

	// Form actions. Each handler checks that the user may do what they asked.
	mux.Handle("POST /organizations", protected.ThenFunc(app.organizationCreatePost))
	mux.Handle("POST /organizations/{id}", protected.ThenFunc(app.organizationUpdatePost))

	mux.Handle("POST /competitions/{id}/status", protected.ThenFunc(app.competitionStatusPost))
	mux.Handle("POST /competitions/{id}/delete", protected.ThenFunc(app.competitionDeletePost))
	mux.Handle("POST /competitions/{id}/register", protected.ThenFunc(app.registrationCreatePost))
	mux.Handle("POST /competitions/{id}/teams", protected.ThenFunc(app.teamCreatePost))
	mux.Handle("POST /competitions/{id}/matches", protected.ThenFunc(app.matchCreatePost))
	mux.Handle("POST /competitions/{id}/matches/generate", protected.ThenFunc(app.fixturesGeneratePost))
	mux.Handle("POST /competitions/{id}/standings/recompute", protected.ThenFunc(app.standingsRecomputePost))
	mux.Handle("POST /competitions/{id}/standings/score", protected.ThenFunc(app.standingsScorePost))
	mux.Handle("POST /competitions/{id}/announcements", protected.ThenFunc(app.announcementCreatePost))

	mux.Handle("POST /registrations/{id}/approve", protected.ThenFunc(app.registrationApprovePost))
	mux.Handle("POST /registrations/{id}/reject", protected.ThenFunc(app.registrationRejectPost))
	mux.Handle("POST /registrations/{id}/cancel", protected.ThenFunc(app.registrationCancelPost))

	mux.Handle("POST /teams/{id}/update", protected.ThenFunc(app.teamUpdatePost))
	mux.Handle("POST /teams/{id}/delete", protected.ThenFunc(app.teamDeletePost))
	mux.Handle("POST /teams/{id}/leave", protected.ThenFunc(app.teamLeavePost))
	mux.Handle("POST /teams/{id}/transfer-captain", protected.ThenFunc(app.teamTransferCaptainPost))
	mux.Handle("POST /teams/{id}/members/{userId}/remove", protected.ThenFunc(app.teamMemberRemovePost))
	mux.Handle("POST /teams/{id}/join-request", protected.ThenFunc(app.joinRequestCreatePost))
	mux.Handle("POST /teams/{id}/invite", protected.ThenFunc(app.invitationCreatePost))
	mux.Handle("POST /join-requests/{id}/accept", protected.ThenFunc(app.joinRequestAcceptPost))
	mux.Handle("POST /join-requests/{id}/reject", protected.ThenFunc(app.joinRequestRejectPost))
	mux.Handle("POST /join-requests/{id}/cancel", protected.ThenFunc(app.joinRequestCancelPost))
	mux.Handle("POST /invitations/{id}/accept", protected.ThenFunc(app.invitationAcceptPost))
	mux.Handle("POST /invitations/{id}/reject", protected.ThenFunc(app.invitationRejectPost))
	mux.Handle("POST /invitations/{id}/cancel", protected.ThenFunc(app.invitationCancelPost))

	mux.Handle("POST /find-team/posts", protected.ThenFunc(app.findTeamPostCreatePost))
	mux.Handle("POST /find-team/posts/{id}/delete", protected.ThenFunc(app.findTeamPostDeletePost))

	mux.Handle("POST /matches/{id}", protected.ThenFunc(app.matchUpdatePost))
	mux.Handle("POST /matches/{id}/delete", protected.ThenFunc(app.matchDeletePost))
	mux.Handle("POST /announcements/{id}/delete", protected.ThenFunc(app.announcementDeletePost))

	mux.Handle("POST /notifications/{id}/read", protected.ThenFunc(app.notificationReadPost))
	mux.Handle("POST /notifications/read-all", protected.ThenFunc(app.notificationReadAllPost))
	mux.Handle("POST /reports", protected.ThenFunc(app.reportCreatePost))

	// Anything else gets the 404 page (rendered with the site's layout, so it
	// needs the dynamic chain for the nav bar).
	mux.Handle("/", dynamic.ThenFunc(app.notFound))

	// Create a middleware chain containing our 'standard' middleware which
	// will be used for every request our application receives.
	standard := alice.New(app.recoverPanic, app.logRequest, commonHeaders, app.rateLimit)

	// Return the 'standard' middleware chain followed by the servemux.
	return standard.Then(mux)
}
