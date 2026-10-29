package main

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"arenasuite/internal/models"
	"arenasuite/internal/realtime"
	"arenasuite/internal/validator"
)

func (app *application) home(w http.ResponseWriter, r *http.Request) {
	sections := []struct {
		title  string
		filter models.CompetitionFilter
	}{
		{"Upcoming Competitions", models.CompetitionFilter{Status: "published", Limit: 6}},
		{"Registration Closing Soon", models.CompetitionFilter{ClosingSoonDays: 14, Sort: "closingSoon", Limit: 6}},
		{"Popular Competitions", models.CompetitionFilter{Sort: "popular", Limit: 6}},
		{"Recently Added", models.CompetitionFilter{Sort: "recent", Limit: 6}},
	}

	data := app.newTemplateData(r)

	for _, s := range sections {
		competitions, err := app.competitions.List(s.filter)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		data.Sections = append(data.Sections, homeSection{Title: s.title, Competitions: competitions})
	}

	app.render(w, r, http.StatusOK, "home.tmpl", data)
}

// competitionList is the search page at /competitions. The same page is used
// for /sports, /academic and /esports, locked to that category.
func (app *application) competitionList(w http.ResponseWriter, r *http.Request) {
	lockedCategories := map[string]struct{ category, heading string }{
		"/sports":   {"SPORTS", "Sports"},
		"/academic": {"ACADEMIC", "Academic"},
		"/esports":  {"ESPORTS", "Esports"},
	}

	qs := r.URL.Query()

	filter := models.CompetitionFilter{
		Q:                qs.Get("q"),
		Category:         qs.Get("category"),
		Mode:             qs.Get("mode"),
		Free:             qs.Get("free"),
		Sort:             qs.Get("sort"),
		RegistrationOpen: qs.Get("open") == "true",
		Limit:            60,
	}

	heading := "Explore Competitions"
	locked, isLocked := lockedCategories[r.URL.Path]
	if isLocked {
		filter.Category = locked.category
		heading = locked.heading
	}

	// An unknown category in the query string just means "all categories".
	if !validator.PermittedValue(filter.Category, models.Categories...) {
		filter.Category = ""
	}

	competitions, err := app.competitions.List(filter)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	data := app.newTemplateData(r)
	data.Competitions = competitions
	data.Filter = filter
	data.Heading = heading
	data.LockedCategory = locked.category

	app.render(w, r, http.StatusOK, "explore.tmpl", data)
}

func (app *application) competitionView(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r, "id")
	if err != nil {
		app.notFound(w, r)
		return
	}

	competition, err := app.competitions.Get(id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return
	}

	user := app.contextGetUser(r)

	// Drafts are hidden (reported as "not found") from everyone except the
	// people who manage the competition.
	if competition.Status == "draft" && !competition.ManagedBy(user) {
		app.notFound(w, r)
		return
	}

	data := app.newTemplateData(r)
	data.Competition = competition
	data.CanManage = competition.ManagedBy(user)
	data.LiveID = competition.ID

	data.Teams, err = app.teams.ForCompetition(id)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	data.Matches, err = app.matches.ForCompetition(id)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	data.Standings, err = app.standings.ForCompetition(id)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	data.Announcements, err = app.announcements.ForCompetition(id)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	if user != nil {
		data.MyRegistration, err = app.registrations.GetActiveForUser(user.ID, id)
		if err != nil && !errors.Is(err, models.ErrNoRecord) {
			app.serverError(w, r, err)
			return
		}

		data.MyRequests, err = app.joinRequests.PendingForUser(user.ID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}

		data.OnTeam, err = app.teams.IsOnTeamIn(id, user.ID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}

		for _, t := range data.Teams {
			if t.CaptainID == user.ID {
				data.CaptainOf = append(data.CaptainOf, t)
			}
		}
	}

	app.render(w, r, http.StatusOK, "competition.tmpl", data)
}

func (app *application) organizationView(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r, "id")
	if err != nil {
		app.notFound(w, r)
		return
	}

	org, err := app.organizations.Get(id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return
	}

	upcoming, past, err := app.competitions.ForOrganization(org.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	data := app.newTemplateData(r)
	data.Org = org
	data.UpcomingCompetitions = upcoming
	data.PastCompetitions = past

	app.render(w, r, http.StatusOK, "organization.tmpl", data)
}

func (app *application) organize(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)

	if user := app.contextGetUser(r); user != nil {
		orgs, err := app.organizations.OwnedBy(user.ID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		data.Orgs = orgs
	}

	app.render(w, r, http.StatusOK, "organize.tmpl", data)
}

type organizationForm struct {
	Name        string `form:"name"`
	Description string `form:"description"`
	Logo        string `form:"logo"`
	College     string `form:"college"`
}

// organizationCreatePost is also how someone becomes an organizer: a
// participant who creates an organization is promoted in the same step.
func (app *application) organizationCreatePost(w http.ResponseWriter, r *http.Request) {
	var form organizationForm

	err := app.decodePostForm(r, &form)
	if err != nil {
		app.clientError(w, r, http.StatusBadRequest)
		return
	}

	if !validator.NotBlank(form.Name) {
		app.redirectBackWithError(w, r, "Organization name is required")
		return
	}

	user := app.contextGetUser(r)

	_, err = app.organizations.Insert(user.ID, form.Name, form.Description, form.Logo, form.College)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.sessionManager.Put(r.Context(), "flash", "Organization created — you are now an organizer")
	http.Redirect(w, r, "/organizer", http.StatusSeeOther)
}

func (app *application) organizationUpdatePost(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r, "id")
	if err != nil {
		app.notFound(w, r)
		return
	}

	org, err := app.organizations.Get(id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return
	}

	user := app.contextGetUser(r)
	if !user.IsAdmin() && org.CreatedBy != user.ID {
		app.clientError(w, r, http.StatusForbidden)
		return
	}

	var form organizationForm

	err = app.decodePostForm(r, &form)
	if err != nil {
		app.clientError(w, r, http.StatusBadRequest)
		return
	}

	if !validator.NotBlank(form.Name) {
		app.redirectBackWithError(w, r, "Organization name is required")
		return
	}

	err = app.organizations.Update(id, form.Name, form.Description, form.Logo, form.College)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.redirectBack(w, r, "Organization saved")
}

// ---------- signed-in user pages ----------

type userProfileForm struct {
	College             string `form:"college"`
	RollNumber          string `form:"rollNumber"`
	Branch              string `form:"branch"`
	validator.Validator `form:"-"`
}

// userProfile doubles as the onboarding screen: new users land here until
// they add their roll number and branch.
func (app *application) userProfile(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)
	data.DefaultCollege = app.config.defaultCollege

	app.render(w, r, http.StatusOK, "profile.tmpl", data)
}

func (app *application) userProfilePost(w http.ResponseWriter, r *http.Request) {
	var form userProfileForm

	err := app.decodePostForm(r, &form)
	if err != nil {
		app.clientError(w, r, http.StatusBadRequest)
		return
	}

	form.CheckField(validator.NotBlank(form.RollNumber), "rollNumber", "Roll number is required")
	form.CheckField(validator.NotBlank(form.Branch), "branch", "Branch is required")

	if !form.Valid() {
		app.redirectBackWithError(w, r, "Roll number and branch are required")
		return
	}

	user := app.contextGetUser(r)

	err = app.users.UpdateProfile(user.ID, form.College, form.RollNumber, form.Branch)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// After the first-time setup, carry on to the dashboard.
	redirectTo := "/profile"
	if !user.ProfileComplete() {
		redirectTo = "/dashboard"
	}

	app.sessionManager.Put(r.Context(), "flash", "Profile updated")
	http.Redirect(w, r, redirectTo, http.StatusSeeOther)
}

func (app *application) userDashboard(w http.ResponseWriter, r *http.Request) {
	user := app.contextGetUser(r)

	registrations, err := app.registrations.ForUser(user.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	teams, err := app.teams.ForUser(user.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	matches, err := app.matches.ForUser(user.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	invitations, err := app.invitations.PendingForUser(user.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	data := app.newTemplateData(r)
	data.Registrations = registrations
	data.Teams = teams
	data.Invitations = invitations

	for _, m := range matches {
		switch m.Status {
		case "scheduled", "live":
			data.UpcomingMatches = append(data.UpcomingMatches, m)
		case "completed":
			data.RecentResults = append([]models.Match{m}, data.RecentResults...) // newest first
		}
	}

	data.Counts = map[string]int64{
		"Teams":    int64(len(teams)),
		"Upcoming": int64(len(data.UpcomingMatches)),
	}
	for _, reg := range registrations {
		if reg.Status != "cancelled" {
			data.Counts["Registered"]++
		}
		if reg.Status == "approved" {
			data.Counts["Approved"]++
		}
	}

	app.render(w, r, http.StatusOK, "dashboard.tmpl", data)
}

func (app *application) notificationList(w http.ResponseWriter, r *http.Request) {
	notifications, err := app.notifications.ForUser(app.contextGetUser(r).ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	data := app.newTemplateData(r)
	data.Notifications = notifications

	app.render(w, r, http.StatusOK, "notifications.tmpl", data)
}

func (app *application) notificationReadPost(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r, "id")
	if err != nil {
		app.notFound(w, r)
		return
	}

	n, err := app.notifications.Get(id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return
	}

	if n.UserID != app.contextGetUser(r).ID {
		app.clientError(w, r, http.StatusForbidden)
		return
	}

	err = app.notifications.MarkRead(id)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.redirectBack(w, r, "")
}

func (app *application) notificationReadAllPost(w http.ResponseWriter, r *http.Request) {
	err := app.notifications.MarkAllRead(app.contextGetUser(r).ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.redirectBack(w, r, "All caught up")
}

type reportForm struct {
	TargetType string `form:"targetType"`
	TargetID   int64  `form:"targetId"`
	Reason     string `form:"reason"`
}

func (app *application) reportCreatePost(w http.ResponseWriter, r *http.Request) {
	var form reportForm

	err := app.decodePostForm(r, &form)
	if err != nil {
		app.clientError(w, r, http.StatusBadRequest)
		return
	}

	if !validator.PermittedValue(form.TargetType, models.ReportTargetTypes...) || form.TargetID < 1 {
		app.clientError(w, r, http.StatusBadRequest)
		return
	}

	if !validator.NotBlank(form.Reason) {
		app.redirectBackWithError(w, r, "Please describe the problem")
		return
	}

	err = app.reports.Insert(app.contextGetUser(r).ID, form.TargetType, form.TargetID, form.Reason)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.redirectBack(w, r, "Thanks — an admin will take a look")
}

// ---------- infrastructure ----------

// healthz is used by hosting platforms to check the app is up and can reach
// the database.
func (app *application) healthz(w http.ResponseWriter, r *http.Request) {
	err := app.users.DB.PingContext(r.Context())
	if err != nil {
		http.Error(w, "database unreachable", http.StatusServiceUnavailable)
		return
	}

	w.Write([]byte("ok"))
}

// events is the live-update stream (Server-Sent Events). A signed-in tab
// joins its user's room; a competition page also joins that competition's
// room via ?competition=ID.
func (app *application) events(w http.ResponseWriter, r *http.Request) {
	var rooms []string

	user := app.contextGetUser(r)
	if user != nil {
		rooms = append(rooms, realtime.UserRoom(user.ID))
	}

	competitionID, _ := strconv.ParseInt(r.URL.Query().Get("competition"), 10, 64)
	if competitionID > 0 {
		// Only join if this visitor is allowed to see the competition.
		competition, err := app.competitions.Get(competitionID)
		if err == nil && (competition.Status != "draft" || competition.ManagedBy(user)) {
			rooms = append(rooms, realtime.CompetitionRoom(competitionID))
		}
	}

	if len(rooms) == 0 {
		w.WriteHeader(http.StatusNoContent) // tells EventSource to stop retrying
		return
	}

	// The stream stays open for as long as the tab does, so lift the server's
	// WriteTimeout for this one response.
	err := http.NewResponseController(w).SetWriteDeadline(time.Time{})
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.hub.Serve(w, r, rooms)
}
