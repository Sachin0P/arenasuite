package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"arenasuite/internal/models"
	"arenasuite/internal/validator"
)

// organizerMessage is shown when an organizer tries to take part in their own
// competition.
const organizerMessage = "You're organizing this competition, so you can't take part in it"

// ---------- organizer dashboard ----------

func (app *application) organizerOverview(w http.ResponseWriter, r *http.Request) {
	user := app.contextGetUser(r)

	competitions, err := app.competitions.List(models.CompetitionFilter{OwnerID: user.ID, IncludeDrafts: true, Limit: 100})
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	counts := map[string]int64{"Total": int64(len(competitions))}

	for _, c := range competitions {
		if c.Status == "published" || c.Status == "ongoing" {
			counts["Active"]++
		}
		counts["Registrations"] += c.RegistrationsCount
		counts["Teams"] += c.TeamsCount

		matches, err := app.matches.ForCompetition(c.ID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}

		for _, m := range matches {
			switch m.Status {
			case "scheduled", "live":
				counts["UpcomingMatches"]++
			case "completed":
				counts["CompletedMatches"]++
			}
		}
	}

	if len(competitions) > 6 {
		competitions = competitions[:6]
	}

	data := app.newTemplateData(r)
	data.Competitions = competitions
	data.Counts = counts
	data.Section = "overview"

	app.render(w, r, http.StatusOK, "organizer-overview.tmpl", data)
}

func (app *application) organizerCompetitionList(w http.ResponseWriter, r *http.Request) {
	user := app.contextGetUser(r)

	competitions, err := app.competitions.List(models.CompetitionFilter{OwnerID: user.ID, IncludeDrafts: true, Sort: "recent", Limit: 100})
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	data := app.newTemplateData(r)
	data.Competitions = competitions
	data.Section = "competitions"

	app.render(w, r, http.StatusOK, "organizer-competitions.tmpl", data)
}

func (app *application) organizerSettings(w http.ResponseWriter, r *http.Request) {
	orgs, err := app.organizations.OwnedBy(app.contextGetUser(r).ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	data := app.newTemplateData(r)
	data.Orgs = orgs
	data.Section = "settings"

	app.render(w, r, http.StatusOK, "organizer-settings.tmpl", data)
}

// ---------- create / edit wizard ----------

// competitionForm is what the four-step create/edit wizard submits. The
// create and edit pages share one template ("competition-form.tmpl").
type competitionForm struct {
	OrganizationID       int64   `form:"organizationId"`
	Name                 string  `form:"name"`
	Description          string  `form:"description"`
	Category             string  `form:"category"`
	Type                 string  `form:"type"`
	Banner               string  `form:"banner"`
	StartDate            string  `form:"startDate"`
	EndDate              string  `form:"endDate"`
	RegistrationDeadline string  `form:"registrationDeadline"`
	Venue                string  `form:"venue"`
	Mode                 string  `form:"mode"`
	MaxTeamSize          int64   `form:"maxTeamSize"`
	MaxTeams             int64   `form:"maxTeams"`
	RegistrationFee      float64 `form:"registrationFee"`
	Rules                string  `form:"rules"`
	Format               string  `form:"format"`
	Stages               string  `form:"stages"` // Academic only, comma separated
	Status               string  `form:"status"` // which button was pressed: "draft" or "published" (create only)
	validator.Validator  `form:"-"`
}

// validate checks the form and fills in defaults. It returns a Competition
// built from the form, with the dates parsed into real times; it's only
// meaningful if form.Valid() is true afterwards.
func (form *competitionForm) validate() *models.Competition {
	form.CheckField(validator.NotBlank(form.Name), "name", "This field cannot be blank")
	form.CheckField(validator.MaxChars(form.Name, 200), "name", "This field cannot be more than 200 characters long")
	form.CheckField(validator.PermittedValue(form.Category, models.Categories...), "category", "Please pick a category")

	if form.Format == "" {
		form.Format = "knockout"
		if form.Category == "ACADEMIC" {
			form.Format = "staged"
		}
	}
	form.CheckField(validator.PermittedValue(form.Format, models.CompetitionFormats...), "format", "Please pick a format")

	if form.Mode != "online" {
		form.Mode = "offline"
	}

	form.CheckField(form.MaxTeamSize >= 0, "maxTeamSize", "This can't be negative")
	form.CheckField(form.MaxTeams >= 0, "maxTeams", "This can't be negative")
	form.CheckField(form.RegistrationFee >= 0, "registrationFee", "This can't be negative")

	// Turn the form's "2026-10-20T10:00" text into times for the database.
	start, err := parseDateTime(form.StartDate)
	form.CheckField(err == nil, "startDate", "Please enter a valid start date")

	end, err := parseDateTime(form.EndDate)
	form.CheckField(err == nil, "endDate", "Please enter a valid end date")

	deadline, err := parseDateTime(form.RegistrationDeadline)
	form.CheckField(err == nil, "registrationDeadline", "Please enter a valid registration deadline")

	form.CheckField(!end.Before(start), "endDate", "End date must be after the start date")

	// Academic stage names are stored as a JSON array.
	var stagesJSON string
	if form.Category == "ACADEMIC" {
		var stages []string
		for _, s := range strings.Split(form.Stages, ",") {
			if s = strings.TrimSpace(s); s != "" {
				stages = append(stages, s)
			}
		}
		if len(stages) > 0 {
			b, _ := json.Marshal(stages)
			stagesJSON = string(b)
		}
	}

	return &models.Competition{
		OrganizationID:       form.OrganizationID,
		Name:                 strings.TrimSpace(form.Name),
		Description:          form.Description,
		Category:             form.Category,
		Type:                 form.Type,
		Banner:               form.Banner,
		StartDate:            start,
		EndDate:              end,
		RegistrationDeadline: deadline,
		Venue:                form.Venue,
		Mode:                 form.Mode,
		MaxTeamSize:          form.MaxTeamSize,
		MaxTeams:             form.MaxTeams,
		RegistrationFee:      form.RegistrationFee,
		Rules:                form.Rules,
		Format:               form.Format,
		StagesJSON:           stagesJSON,
	}
}

// renderCompetitionForm shows the create or edit wizard. On a validation
// error the form is re-rendered with what was typed, rather than
// redirecting and losing it.
func (app *application) renderCompetitionForm(w http.ResponseWriter, r *http.Request, status int, form competitionForm, editingID int64) {
	orgs, err := app.organizations.OwnedBy(app.contextGetUser(r).ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	if form.OrganizationID == 0 && len(orgs) == 1 {
		form.OrganizationID = orgs[0].ID
	}

	data := app.newTemplateData(r)
	data.Form = form
	data.Orgs = orgs
	data.EditingID = editingID
	data.Section = "competitions"
	data.Heading = "Create Competition"
	if editingID != 0 {
		data.Heading = "Edit Competition"
	}

	app.render(w, r, status, "competition-form.tmpl", data)
}

func (app *application) competitionCreate(w http.ResponseWriter, r *http.Request) {
	form := competitionForm{
		Category: "SPORTS",
		Format:   "knockout",
		Mode:     "offline",
		Stages:   "Registration, Team Formation, Submission, Evaluation, Leaderboard, Results",
	}

	app.renderCompetitionForm(w, r, http.StatusOK, form, 0)
}

func (app *application) competitionCreatePost(w http.ResponseWriter, r *http.Request) {
	var form competitionForm

	err := app.decodePostForm(r, &form)
	if err != nil {
		app.clientError(w, r, http.StatusBadRequest)
		return
	}

	competition := form.validate()

	user := app.contextGetUser(r)

	org, err := app.organizations.Get(form.OrganizationID)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			form.AddFieldError("organizationId", "Please pick an organization")
		} else {
			app.serverError(w, r, err)
			return
		}
	} else if !user.IsAdmin() && org.CreatedBy != user.ID {
		app.clientError(w, r, http.StatusForbidden)
		return
	}

	if !form.Valid() {
		app.renderCompetitionForm(w, r, http.StatusUnprocessableEntity, form, 0)
		return
	}

	competition.CreatedBy = user.ID
	competition.Status = "draft"
	if form.Status == "published" {
		competition.Status = "published"
	}

	id, err := app.competitions.Insert(competition)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	flash := "Draft saved"
	if competition.Status == "published" {
		flash = "Competition published"
	}

	app.sessionManager.Put(r.Context(), "flash", flash)
	http.Redirect(w, r, fmt.Sprintf("/organizer/competitions/%d/manage", id), http.StatusSeeOther)
}

func (app *application) competitionEdit(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r, "id")
	if err != nil {
		app.notFound(w, r)
		return
	}

	c, ok := app.getCompetitionToManage(w, r, id)
	if !ok {
		return
	}

	form := competitionForm{
		OrganizationID:       c.OrganizationID,
		Name:                 c.Name,
		Description:          c.Description,
		Category:             c.Category,
		Type:                 c.Type,
		Banner:               c.Banner,
		StartDate:            inputDateTime(c.StartDate),
		EndDate:              inputDateTime(c.EndDate),
		RegistrationDeadline: inputDateTime(c.RegistrationDeadline),
		Venue:                c.Venue,
		Mode:                 c.Mode,
		MaxTeamSize:          c.MaxTeamSize,
		MaxTeams:             c.MaxTeams,
		RegistrationFee:      c.RegistrationFee,
		Rules:                c.Rules,
		Format:               c.Format,
		Stages:               strings.Join(c.Stages(), ", "),
	}

	app.renderCompetitionForm(w, r, http.StatusOK, form, c.ID)
}

func (app *application) competitionEditPost(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r, "id")
	if err != nil {
		app.notFound(w, r)
		return
	}

	c, ok := app.getCompetitionToManage(w, r, id)
	if !ok {
		return
	}

	var form competitionForm

	err = app.decodePostForm(r, &form)
	if err != nil {
		app.clientError(w, r, http.StatusBadRequest)
		return
	}

	// Moving a competition to another organization isn't supported.
	form.OrganizationID = c.OrganizationID

	competition := form.validate()

	if !form.Valid() {
		app.renderCompetitionForm(w, r, http.StatusUnprocessableEntity, form, c.ID)
		return
	}

	competition.ID = c.ID

	err = app.competitions.Update(competition)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.sessionManager.Put(r.Context(), "flash", "Competition updated")
	http.Redirect(w, r, fmt.Sprintf("/organizer/competitions/%d/manage", c.ID), http.StatusSeeOther)
}

// ---------- manage one competition ----------

func (app *application) competitionManage(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r, "id")
	if err != nil {
		app.notFound(w, r)
		return
	}

	c, ok := app.getCompetitionToManage(w, r, id)
	if !ok {
		return
	}

	data := app.newTemplateData(r)
	data.Competition = c
	data.Section = "competitions"
	data.LiveID = c.ID // new registrations show up without a reload

	data.Registrations, err = app.registrations.ForCompetition(id)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

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

	app.render(w, r, http.StatusOK, "manage-competition.tmpl", data)
}

func (app *application) competitionStatusPost(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r, "id")
	if err != nil {
		app.notFound(w, r)
		return
	}

	_, ok := app.getCompetitionToManage(w, r, id)
	if !ok {
		return
	}

	status := r.PostFormValue("status")
	if !validator.PermittedValue(status, models.CompetitionStatuses...) {
		app.clientError(w, r, http.StatusBadRequest)
		return
	}

	err = app.competitions.SetStatus(id, status)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.redirectBack(w, r, "Competition is now "+status)
}

func (app *application) competitionDeletePost(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r, "id")
	if err != nil {
		app.notFound(w, r)
		return
	}

	_, ok := app.getCompetitionToManage(w, r, id)
	if !ok {
		return
	}

	err = app.competitions.Delete(id)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.sessionManager.Put(r.Context(), "flash", "Competition deleted")
	http.Redirect(w, r, "/organizer/competitions", http.StatusSeeOther)
}

// ---------- registrations ----------

// registrationCreatePost signs a team up for a competition. With no teamId
// in the form it's "Register Individually", which creates a one-person team.
// Registrations start as "pending" until the organizer approves them.
func (app *application) registrationCreatePost(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r, "id")
	if err != nil {
		app.notFound(w, r)
		return
	}

	c, err := app.competitions.Get(id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return
	}

	user := app.contextGetUser(r)

	if c.OrganizedBy(user) {
		app.redirectBackWithError(w, r, organizerMessage)
		return
	}

	if !c.RegistrationOpen() {
		app.redirectBackWithError(w, r, "Registration is closed for this competition")
		return
	}

	var teamID int64
	if v := r.PostFormValue("teamId"); v != "" {
		teamID, err = strconv.ParseInt(v, 10, 64)
		if err != nil {
			app.clientError(w, r, http.StatusBadRequest)
			return
		}
	}

	who := user.Name
	flash := "Registration submitted — awaiting organizer approval"

	// Registering an existing team: check it's in this competition and that
	// the user is its captain.
	if teamID != 0 {
		team, err := app.teams.Get(teamID)
		if err != nil {
			if errors.Is(err, models.ErrNoRecord) {
				app.notFound(w, r)
			} else {
				app.serverError(w, r, err)
			}
			return
		}

		if team.CompetitionID != c.ID {
			app.notFound(w, r)
			return
		}

		if !team.CaptainedBy(user) {
			app.clientError(w, r, http.StatusForbidden)
			return
		}

		who = "Team " + team.Name
		flash = "Team registration submitted — awaiting organizer approval"
	}

	if teamID == 0 {
		err = app.registrations.InsertSolo(c, user)
	} else {
		err = app.registrations.Insert(c, user.ID, teamID)
	}
	if err != nil {
		switch {
		case errors.Is(err, models.ErrAlreadyOnTeam):
			app.redirectBackWithError(w, r, "You're already on a team in this competition, so register with that team instead")
		case errors.Is(err, models.ErrMaxTeamsReached):
			app.redirectBackWithError(w, r, "This competition has reached its maximum number of teams")
		case errors.Is(err, models.ErrAlreadyRegistered) && teamID == 0:
			app.redirectBackWithError(w, r, "You are already registered for this competition")
		case errors.Is(err, models.ErrAlreadyRegistered):
			app.redirectBackWithError(w, r, "This team, or one of its members, is already registered for this competition")
		default:
			app.serverError(w, r, err)
		}
		return
	}

	app.notify(c.OrgOwnerID, "registrationSubmitted", who+" registered for "+c.Name+" and is awaiting your approval", c.ID)
	app.broadcast(c.ID, "registrationSubmitted", map[string]int64{"competitionId": c.ID})

	app.redirectBack(w, r, flash)
}

func (app *application) registrationApprovePost(w http.ResponseWriter, r *http.Request) {
	app.reviewRegistration(w, r, "approved")
}

func (app *application) registrationRejectPost(w http.ResponseWriter, r *http.Request) {
	app.reviewRegistration(w, r, "rejected")
}

// reviewRegistration approves or rejects a pending registration and tells
// everyone affected (every active member of the registered team).
func (app *application) reviewRegistration(w http.ResponseWriter, r *http.Request, status string) {
	id, err := app.readIDParam(r, "id")
	if err != nil {
		app.notFound(w, r)
		return
	}

	reg, err := app.registrations.Get(id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return
	}

	_, ok := app.getCompetitionToManage(w, r, reg.CompetitionID)
	if !ok {
		return
	}

	if reg.Status != "pending" {
		app.redirectBackWithError(w, r, "This registration has already been reviewed")
		return
	}

	err = app.registrations.SetStatus(reg.ID, status)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	affected := []int64{reg.UserID}
	if reg.TeamID != 0 {
		affected, err = app.teams.ActiveMemberIDs(reg.TeamID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
	}

	// The notification type and live event are "registrationApproved" or
	// "registrationRejected".
	kind := "registration" + strings.ToUpper(status[:1]) + status[1:]

	for _, userID := range affected {
		app.notify(userID, kind, "Your registration for "+reg.CompetitionName+" was "+status, reg.CompetitionID)
	}
	app.broadcast(reg.CompetitionID, kind, map[string]int64{"registrationId": reg.ID})

	app.redirectBack(w, r, "Registration "+status)
}

// registrationCancelPost can be done by whoever registered (the captain for a
// team) or an admin.
func (app *application) registrationCancelPost(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r, "id")
	if err != nil {
		app.notFound(w, r)
		return
	}

	reg, err := app.registrations.Get(id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return
	}

	user := app.contextGetUser(r)
	if reg.UserID != user.ID && !user.IsAdmin() {
		app.clientError(w, r, http.StatusForbidden)
		return
	}

	if reg.Status == "cancelled" {
		app.redirectBackWithError(w, r, "This registration is already cancelled")
		return
	}

	err = app.registrations.SetStatus(reg.ID, "cancelled")
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.redirectBack(w, r, "Registration cancelled")
}

// ---------- matches & standings ----------

type matchCreateForm struct {
	Round       string `form:"round"`
	TeamAID     int64  `form:"teamAId"`
	TeamBID     int64  `form:"teamBId"`
	ScheduledAt string `form:"scheduledAt"`
	Venue       string `form:"venue"`
}

// matchCreatePost adds a single match by hand (outside the fixture generator).
func (app *application) matchCreatePost(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r, "id")
	if err != nil {
		app.notFound(w, r)
		return
	}

	_, ok := app.getCompetitionToManage(w, r, id)
	if !ok {
		return
	}

	var form matchCreateForm

	err = app.decodePostForm(r, &form)
	if err != nil {
		app.clientError(w, r, http.StatusBadRequest)
		return
	}

	if !validator.NotBlank(form.Round) {
		app.redirectBackWithError(w, r, "Round is required")
		return
	}

	scheduledAt, ok := optionalDateTime(form.ScheduledAt)
	if !ok {
		app.redirectBackWithError(w, r, "Please enter a valid date and time")
		return
	}

	err = app.matches.Insert(id, form.Round, form.TeamAID, form.TeamBID, scheduledAt, form.Venue)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.broadcast(id, "matchStatusUpdated", map[string]int64{"competitionId": id})

	app.redirectBack(w, r, "Match added")
}

func (app *application) fixturesGeneratePost(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r, "id")
	if err != nil {
		app.notFound(w, r)
		return
	}

	c, ok := app.getCompetitionToManage(w, r, id)
	if !ok {
		return
	}

	regenerate := r.PostFormValue("regenerate") == "true"

	err = app.matches.Generate(id, c.IsKnockout(), regenerate)
	if err != nil {
		switch {
		case errors.Is(err, models.ErrFixturesExist):
			app.redirectBackWithError(w, r, "Fixtures already exist for this competition")
		case errors.Is(err, models.ErrMatchesStarted):
			app.redirectBackWithError(w, r, "Cannot regenerate fixtures once matches are live or completed")
		case errors.Is(err, models.ErrNotEnoughTeams):
			app.redirectBackWithError(w, r, "At least 2 approved teams are required to generate fixtures")
		default:
			app.serverError(w, r, err)
		}
		return
	}

	app.broadcast(id, "standingsUpdated", map[string]int64{"competitionId": id})
	app.broadcast(id, "matchStatusUpdated", map[string]int64{"competitionId": id})

	app.redirectBack(w, r, "Fixtures generated")
}

// matchUpdateForm is what the organizer submits from the live-scores screen.
// An empty score field decodes to nil, which means "no score yet".
type matchUpdateForm struct {
	ScoreA      *int64 `form:"scoreA"`
	ScoreB      *int64 `form:"scoreB"`
	Status      string `form:"status"`
	ScheduledAt string `form:"scheduledAt"`
	Venue       string `form:"venue"`
}

// matchUpdatePost saves scores and status. When a match is completed the
// winner is worked out from the scores (equal scores = draw, no winner),
// moved into its next bracket match, and the standings are recalculated.
func (app *application) matchUpdatePost(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r, "id")
	if err != nil {
		app.notFound(w, r)
		return
	}

	match, err := app.matches.Get(id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return
	}

	_, ok := app.getCompetitionToManage(w, r, match.CompetitionID)
	if !ok {
		return
	}

	var form matchUpdateForm

	err = app.decodePostForm(r, &form)
	if err != nil {
		app.clientError(w, r, http.StatusBadRequest)
		return
	}

	if form.Status == "" {
		form.Status = match.Status
	}
	if !validator.PermittedValue(form.Status, models.MatchStatuses...) {
		app.clientError(w, r, http.StatusBadRequest)
		return
	}

	scheduledAt, ok := optionalDateTime(form.ScheduledAt)
	if !ok {
		app.redirectBackWithError(w, r, "Please enter a valid date and time")
		return
	}

	// Keep a copy of the match as it was, to work out which live events to send.
	before := *match

	match.ScoreA = form.ScoreA
	match.ScoreB = form.ScoreB
	match.Status = form.Status
	match.ScheduledAt = scheduledAt
	match.Venue = form.Venue

	if match.Status == "completed" && match.ScoreA != nil && match.ScoreB != nil {
		switch {
		case *match.ScoreA > *match.ScoreB:
			match.WinnerID = match.TeamAID
		case *match.ScoreB > *match.ScoreA:
			match.WinnerID = match.TeamBID
		default:
			match.WinnerID = 0 // draw
		}
	}

	err = app.matches.Update(match)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Re-load the match so the live events carry the saved state (including
	// the winner's name).
	updated, err := app.matches.Get(id)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	if updated.Status == "completed" {
		app.broadcast(updated.CompetitionID, "standingsUpdated", map[string]int64{"competitionId": updated.CompetitionID})
	}
	if updated.Status != before.Status {
		app.broadcast(updated.CompetitionID, "matchStatusUpdated", updated)
	}
	if !sameScore(updated.ScoreA, before.ScoreA) || !sameScore(updated.ScoreB, before.ScoreB) {
		app.broadcast(updated.CompetitionID, "scoreUpdated", updated)
	}

	// Tell both teams' members.
	title := orTBD(updated.TeamAName) + " vs " + orTBD(updated.TeamBName)

	for _, teamID := range []int64{updated.TeamAID, updated.TeamBID} {
		if teamID == 0 {
			continue
		}

		members, err := app.teams.ActiveMemberIDs(teamID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}

		for _, userID := range members {
			app.notify(userID, "matchUpdated", title+" was updated", updated.ID)
		}
	}

	app.redirectBack(w, r, "Match saved")
}

func (app *application) matchDeletePost(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r, "id")
	if err != nil {
		app.notFound(w, r)
		return
	}

	match, err := app.matches.Get(id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return
	}

	_, ok := app.getCompetitionToManage(w, r, match.CompetitionID)
	if !ok {
		return
	}

	err = app.matches.Delete(id)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.redirectBack(w, r, "Match deleted")
}

func (app *application) standingsRecomputePost(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r, "id")
	if err != nil {
		app.notFound(w, r)
		return
	}

	_, ok := app.getCompetitionToManage(w, r, id)
	if !ok {
		return
	}

	err = app.standings.Recompute(id)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.broadcast(id, "standingsUpdated", map[string]int64{"competitionId": id})

	app.redirectBack(w, r, "Standings recalculated")
}

type standingsScoreForm struct {
	TeamID int64 `form:"teamId"`
	Points int64 `form:"points"`
}

// standingsScorePost is for Academic competitions, which have no matches to
// derive a table from: the organizer enters a team's score directly.
func (app *application) standingsScorePost(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r, "id")
	if err != nil {
		app.notFound(w, r)
		return
	}

	c, ok := app.getCompetitionToManage(w, r, id)
	if !ok {
		return
	}

	if c.Category != "ACADEMIC" {
		app.redirectBackWithError(w, r, "Direct score entry is only for Academic competitions; other categories use match results")
		return
	}

	var form standingsScoreForm

	err = app.decodePostForm(r, &form)
	if err != nil {
		app.clientError(w, r, http.StatusBadRequest)
		return
	}

	if form.TeamID == 0 {
		app.redirectBackWithError(w, r, "Pick a team")
		return
	}

	team, err := app.teams.Get(form.TeamID)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return
	}

	if team.CompetitionID != c.ID {
		app.notFound(w, r)
		return
	}

	err = app.standings.SetScore(c.ID, team.ID, form.Points)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.broadcast(c.ID, "standingsUpdated", map[string]int64{"competitionId": c.ID})

	app.redirectBack(w, r, "Score saved")
}

// ---------- announcements ----------

type announcementForm struct {
	Title   string `form:"title"`
	Message string `form:"message"`
}

// announcementCreatePost posts an announcement, pushes it live to everyone on
// the competition page, and notifies every approved participant.
func (app *application) announcementCreatePost(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r, "id")
	if err != nil {
		app.notFound(w, r)
		return
	}

	c, ok := app.getCompetitionToManage(w, r, id)
	if !ok {
		return
	}

	var form announcementForm

	err = app.decodePostForm(r, &form)
	if err != nil {
		app.clientError(w, r, http.StatusBadRequest)
		return
	}

	if !validator.NotBlank(form.Title) || !validator.NotBlank(form.Message) {
		app.redirectBackWithError(w, r, "Title and message are required")
		return
	}

	announcementID, err := app.announcements.Insert(c.ID, form.Title, form.Message, app.contextGetUser(r).ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.broadcast(c.ID, "announcementCreated", map[string]any{"id": announcementID, "title": form.Title})

	participants, err := app.registrations.ApprovedParticipantIDs(c.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	for _, userID := range participants {
		app.notify(userID, "announcement", c.Name+": "+form.Title, announcementID)
	}

	app.redirectBack(w, r, "Announcement published")
}

func (app *application) announcementDeletePost(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r, "id")
	if err != nil {
		app.notFound(w, r)
		return
	}

	announcement, err := app.announcements.Get(id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return
	}

	_, ok := app.getCompetitionToManage(w, r, announcement.CompetitionID)
	if !ok {
		return
	}

	err = app.announcements.Delete(id)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.redirectBack(w, r, "Announcement deleted")
}

// optionalDateTime parses an optional datetime-local form value. An empty
// value means "not set" and returns nil; ok is false if the value is invalid.
func optionalDateTime(value string) (t *time.Time, ok bool) {
	if strings.TrimSpace(value) == "" {
		return nil, true
	}

	parsed, err := parseDateTime(value)
	if err != nil {
		return nil, false
	}

	return &parsed, true
}

func sameScore(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func orTBD(name string) string {
	if name == "" {
		return "TBD"
	}
	return name
}
