package main

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"arenasuite/internal/models"
	"arenasuite/internal/validator"
)

func (app *application) teamView(w http.ResponseWriter, r *http.Request) {
	team, ok := app.getTeam(w, r)
	if !ok {
		return
	}

	members, err := app.teams.Members(team.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	team.Members = members

	user := app.contextGetUser(r)

	data := app.newTemplateData(r)
	data.Team = team
	data.IsCaptain = team.CaptainedBy(user)
	data.IsMember = user != nil && team.HasMember(user.ID)

	if data.IsCaptain {
		data.JoinRequests, err = app.joinRequests.PendingForTeam(team.ID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
	}

	if user != nil {
		requests, err := app.joinRequests.PendingForUser(user.ID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		data.MyRequestID = requests[team.ID]
	}

	app.render(w, r, http.StatusOK, "team.tmpl", data)
}

// findTeam has two tabs, done as links (?tab=...) because each loads
// different data: teams looking for members, and players looking for a team.
func (app *application) findTeam(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()

	tab := qs.Get("tab")
	if tab != "players" {
		tab = "teams"
	}

	competitionID, _ := strconv.ParseInt(qs.Get("competitionId"), 10, 64)

	filter := models.FindTeamFilter{
		CompetitionID: competitionID,
		Category:      qs.Get("category"),
		Role:          qs.Get("role"),
	}

	// Competitions people can still form teams for, for the filter and the
	// post form.
	competitions, err := app.competitions.List(models.CompetitionFilter{Status: "published", Limit: 100})
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	data := app.newTemplateData(r)
	data.Tab = tab
	data.FindFilter = filter
	data.Competitions = competitions

	if tab == "teams" {
		data.Teams, err = app.teams.Recruiting(filter)
	} else {
		data.Posts, err = app.posts.Open(filter)
	}
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	if user := app.contextGetUser(r); user != nil {
		data.MyRequests, err = app.joinRequests.PendingForUser(user.ID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}

		myTeams, err := app.teams.ForUser(user.ID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}

		for _, t := range myTeams {
			if t.CaptainID == user.ID {
				data.CaptainOf = append(data.CaptainOf, t)
			}
		}
	}

	app.render(w, r, http.StatusOK, "find-team.tmpl", data)
}

type teamForm struct {
	Name              string `form:"name"`
	Description       string `form:"description"`
	MaxMembers        int64  `form:"maxMembers"`
	LookingForMembers bool   `form:"lookingForMembers"`
	LookingForRole    string `form:"lookingForRole"`
	RequiredSkills    string `form:"requiredSkills"`
}

// teamSizeProblem checks that a team's size fits the competition's limit
// (0 = no limit) and isn't smaller than the members it already has. It
// returns a message for the user, or "" if the size is fine.
func teamSizeProblem(size, limit, currentMembers int64) string {
	if limit > 0 && size > limit {
		return fmt.Sprintf("Teams in this competition can have at most %d members", limit)
	}
	if size < currentMembers {
		return fmt.Sprintf("This team already has %d members", currentMembers)
	}
	return ""
}

// teamCreatePost makes the user captain of a new team in a competition.
func (app *application) teamCreatePost(w http.ResponseWriter, r *http.Request) {
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

	var form teamForm

	err = app.decodePostForm(r, &form)
	if err != nil {
		app.clientError(w, r, http.StatusBadRequest)
		return
	}

	user := app.contextGetUser(r)

	if !validator.NotBlank(form.Name) {
		app.redirectBackWithError(w, r, "Team name is required")
		return
	}

	if c.OrganizedBy(user) {
		app.redirectBackWithError(w, r, organizerMessage)
		return
	}

	if c.Status != "published" && c.Status != "ongoing" {
		app.redirectBackWithError(w, r, "Teams can only be created for published competitions")
		return
	}

	// Default team size: the competition's limit if it has one, otherwise 4.
	if form.MaxMembers <= 0 {
		form.MaxMembers = c.MaxTeamSize
	}
	if form.MaxMembers <= 0 {
		form.MaxMembers = 4
	}

	if problem := teamSizeProblem(form.MaxMembers, c.MaxTeamSize, 1); problem != "" {
		app.redirectBackWithError(w, r, problem)
		return
	}

	team := &models.Team{
		CompetitionID:     c.ID,
		Name:              form.Name,
		Description:       form.Description,
		CaptainID:         user.ID,
		MaxMembers:        form.MaxMembers,
		RequiredSkills:    form.RequiredSkills,
		LookingForMembers: form.LookingForMembers,
		LookingForRole:    form.LookingForRole,
	}

	teamID, err := app.teams.Insert(team)
	if err != nil {
		switch {
		case errors.Is(err, models.ErrAlreadyOnTeam):
			app.redirectBackWithError(w, r, "You are already on a team for this competition")
		case errors.Is(err, models.ErrDuplicateName):
			app.redirectBackWithError(w, r, "A team with this name already exists in this competition")
		default:
			app.serverError(w, r, err)
		}
		return
	}

	app.sessionManager.Put(r.Context(), "flash", "Team created")
	http.Redirect(w, r, fmt.Sprintf("/teams/%d", teamID), http.StatusSeeOther)
}

func (app *application) teamUpdatePost(w http.ResponseWriter, r *http.Request) {
	team, ok := app.getTeam(w, r)
	if !ok {
		return
	}

	if !team.CaptainedBy(app.contextGetUser(r)) {
		app.clientError(w, r, http.StatusForbidden)
		return
	}

	var form teamForm

	err := app.decodePostForm(r, &form)
	if err != nil {
		app.clientError(w, r, http.StatusBadRequest)
		return
	}

	if !validator.NotBlank(form.Name) {
		app.redirectBackWithError(w, r, "Team name is required")
		return
	}

	if form.MaxMembers <= 0 {
		form.MaxMembers = team.MaxMembers
	}

	if problem := teamSizeProblem(form.MaxMembers, team.MaxTeamSize, team.MemberCount); problem != "" {
		app.redirectBackWithError(w, r, problem)
		return
	}

	team.Name = form.Name
	team.Description = form.Description
	team.MaxMembers = form.MaxMembers
	team.RequiredSkills = form.RequiredSkills
	team.LookingForMembers = form.LookingForMembers
	team.LookingForRole = form.LookingForRole

	err = app.teams.Update(team)
	if err != nil {
		if errors.Is(err, models.ErrDuplicateName) {
			app.redirectBackWithError(w, r, "A team with this name already exists in this competition")
		} else {
			app.serverError(w, r, err)
		}
		return
	}

	app.redirectBack(w, r, "Team saved")
}

// teamDeletePost disbands a team. Its registration goes too.
func (app *application) teamDeletePost(w http.ResponseWriter, r *http.Request) {
	team, ok := app.getTeam(w, r)
	if !ok {
		return
	}

	if !team.CaptainedBy(app.contextGetUser(r)) {
		app.clientError(w, r, http.StatusForbidden)
		return
	}

	err := app.teams.Delete(team.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.sessionManager.Put(r.Context(), "flash", "Team disbanded")
	http.Redirect(w, r, fmt.Sprintf("/competitions/%d#teams", team.CompetitionID), http.StatusSeeOther)
}

func (app *application) teamLeavePost(w http.ResponseWriter, r *http.Request) {
	team, ok := app.getTeam(w, r)
	if !ok {
		return
	}

	user := app.contextGetUser(r)

	if team.CaptainID == user.ID {
		app.redirectBackWithError(w, r, "Transfer captaincy before leaving the team")
		return
	}

	err := app.teams.SetMemberStatus(team.ID, user.ID, "left")
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.redirectBackWithError(w, r, "You are not an active member of this team")
		} else {
			app.serverError(w, r, err)
		}
		return
	}

	app.sessionManager.Put(r.Context(), "flash", "You left the team")
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (app *application) teamTransferCaptainPost(w http.ResponseWriter, r *http.Request) {
	team, ok := app.getTeam(w, r)
	if !ok {
		return
	}

	if !team.CaptainedBy(app.contextGetUser(r)) {
		app.clientError(w, r, http.StatusForbidden)
		return
	}

	newCaptainID, err := strconv.ParseInt(r.PostFormValue("userId"), 10, 64)
	if err != nil {
		app.clientError(w, r, http.StatusBadRequest)
		return
	}

	isMember, err := app.teams.IsActiveMember(team.ID, newCaptainID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	if !isMember {
		app.redirectBackWithError(w, r, "Target user is not an active member of this team")
		return
	}

	err = app.teams.TransferCaptain(team.ID, team.CaptainID, newCaptainID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.redirectBack(w, r, "Captaincy transferred")
}

func (app *application) teamMemberRemovePost(w http.ResponseWriter, r *http.Request) {
	team, ok := app.getTeam(w, r)
	if !ok {
		return
	}

	if !team.CaptainedBy(app.contextGetUser(r)) {
		app.clientError(w, r, http.StatusForbidden)
		return
	}

	memberID, err := app.readIDParam(r, "userId")
	if err != nil {
		app.notFound(w, r)
		return
	}

	if memberID == team.CaptainID {
		app.redirectBackWithError(w, r, "Transfer captaincy before removing the captain")
		return
	}

	err = app.teams.SetMemberStatus(team.ID, memberID, "removed")
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.redirectBackWithError(w, r, "Member not found on this team")
		} else {
			app.serverError(w, r, err)
		}
		return
	}

	app.redirectBack(w, r, "Member removed")
}

// ---------- join requests ----------

func (app *application) joinRequestCreatePost(w http.ResponseWriter, r *http.Request) {
	team, ok := app.getTeam(w, r)
	if !ok {
		return
	}

	user := app.contextGetUser(r)

	if team.OrganizedBy(user) {
		app.redirectBackWithError(w, r, organizerMessage)
		return
	}

	if !team.LookingForMembers {
		app.redirectBackWithError(w, r, "This team is not looking for members")
		return
	}

	if team.IsFull() {
		app.redirectBackWithError(w, r, "This team is full")
		return
	}

	onTeam, err := app.teams.IsOnTeamIn(team.CompetitionID, user.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	if onTeam {
		app.redirectBackWithError(w, r, "You are already on a team for this competition")
		return
	}

	pending, err := app.joinRequests.HasPending(team.ID, user.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	if pending {
		app.redirectBackWithError(w, r, "You already have a pending request for this team")
		return
	}

	requestID, err := app.joinRequests.Insert(team.ID, user.ID, r.PostFormValue("message"))
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.notify(team.CaptainID, "teamJoinRequest", user.Name+" requested to join "+team.Name, team.ID)
	app.broadcast(team.CompetitionID, "teamJoinRequest", map[string]int64{"teamId": team.ID, "requestId": requestID})

	app.redirectBack(w, r, "Join request sent to the captain")
}

func (app *application) joinRequestAcceptPost(w http.ResponseWriter, r *http.Request) {
	app.respondJoinRequest(w, r, true)
}

func (app *application) joinRequestRejectPost(w http.ResponseWriter, r *http.Request) {
	app.respondJoinRequest(w, r, false)
}

// respondJoinRequest is the captain accepting or rejecting a join request.
func (app *application) respondJoinRequest(w http.ResponseWriter, r *http.Request, accept bool) {
	id, err := app.readIDParam(r, "id")
	if err != nil {
		app.notFound(w, r)
		return
	}

	req, err := app.joinRequests.Get(id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return
	}

	team, err := app.teams.Get(req.TeamID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	if !team.CaptainedBy(app.contextGetUser(r)) {
		app.clientError(w, r, http.StatusForbidden)
		return
	}

	if req.Status != "pending" {
		app.redirectBackWithError(w, r, "This request has already been resolved")
		return
	}

	verb, flash := "rejected", "Request rejected"

	if accept {
		verb, flash = "accepted", "Request accepted — they're on the team"
		err = app.joinRequests.Accept(req, team)
	} else {
		err = app.joinRequests.SetStatus(req.ID, "rejected")
	}
	if err != nil {
		switch {
		case errors.Is(err, models.ErrTeamFull):
			app.redirectBackWithError(w, r, "This team is full")
		case errors.Is(err, models.ErrAlreadyOnTeam):
			app.redirectBackWithError(w, r, "That user is already on a team for this competition")
		default:
			app.serverError(w, r, err)
		}
		return
	}

	app.notify(req.UserID, "teamJoinRequest", "Your request to join "+team.Name+" was "+verb, team.ID)

	app.redirectBack(w, r, flash)
}

func (app *application) joinRequestCancelPost(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r, "id")
	if err != nil {
		app.notFound(w, r)
		return
	}

	req, err := app.joinRequests.Get(id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return
	}

	if req.UserID != app.contextGetUser(r).ID {
		app.clientError(w, r, http.StatusForbidden)
		return
	}

	if req.Status != "pending" {
		app.redirectBackWithError(w, r, "This request has already been resolved")
		return
	}

	err = app.joinRequests.SetStatus(req.ID, "cancelled")
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.redirectBack(w, r, "Request withdrawn")
}

// ---------- invitations ----------

func (app *application) invitationCreatePost(w http.ResponseWriter, r *http.Request) {
	team, ok := app.getTeam(w, r)
	if !ok {
		return
	}

	user := app.contextGetUser(r)

	if !team.CaptainedBy(user) {
		app.clientError(w, r, http.StatusForbidden)
		return
	}

	inviteeID, err := strconv.ParseInt(r.PostFormValue("userId"), 10, 64)
	if err != nil {
		app.clientError(w, r, http.StatusBadRequest)
		return
	}

	if team.OrganizerID == inviteeID {
		app.redirectBackWithError(w, r, "That person is organizing this competition, so they can't join a team in it")
		return
	}

	if team.IsFull() {
		app.redirectBackWithError(w, r, "This team is full")
		return
	}

	onTeam, err := app.teams.IsOnTeamIn(team.CompetitionID, inviteeID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	if onTeam {
		app.redirectBackWithError(w, r, "That user is already on a team for this competition")
		return
	}

	pending, err := app.invitations.HasPending(team.ID, inviteeID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	if pending {
		app.redirectBackWithError(w, r, "An invitation is already pending for this user")
		return
	}

	err = app.invitations.Insert(team.ID, inviteeID, user.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.notify(inviteeID, "teamInvitation", "You have been invited to join "+team.Name, team.ID)

	app.redirectBack(w, r, "Invitation sent")
}

func (app *application) invitationAcceptPost(w http.ResponseWriter, r *http.Request) {
	app.respondInvitation(w, r, true)
}

func (app *application) invitationRejectPost(w http.ResponseWriter, r *http.Request) {
	app.respondInvitation(w, r, false)
}

// respondInvitation is the invited user accepting or declining.
func (app *application) respondInvitation(w http.ResponseWriter, r *http.Request, accept bool) {
	id, err := app.readIDParam(r, "id")
	if err != nil {
		app.notFound(w, r)
		return
	}

	invitation, err := app.invitations.Get(id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return
	}

	user := app.contextGetUser(r)

	if invitation.UserID != user.ID {
		app.clientError(w, r, http.StatusForbidden)
		return
	}

	if invitation.Status != "pending" {
		app.redirectBackWithError(w, r, "This invitation has already been resolved")
		return
	}

	team, err := app.teams.Get(invitation.TeamID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	verb, flash := "declined", "Invitation declined"

	if accept {
		if team.OrganizedBy(user) {
			app.redirectBackWithError(w, r, organizerMessage)
			return
		}

		verb, flash = "accepted", "Welcome to the team!"
		err = app.invitations.Accept(invitation, team)
	} else {
		err = app.invitations.SetStatus(invitation.ID, "rejected")
	}
	if err != nil {
		switch {
		case errors.Is(err, models.ErrTeamFull):
			app.redirectBackWithError(w, r, "This team is full")
		case errors.Is(err, models.ErrAlreadyOnTeam):
			app.redirectBackWithError(w, r, "You are already on a team for this competition")
		default:
			app.serverError(w, r, err)
		}
		return
	}

	app.notify(team.CaptainID, "teamInvitationResponse", user.Name+" "+verb+" your invitation to "+team.Name, team.ID)

	app.redirectBack(w, r, flash)
}

func (app *application) invitationCancelPost(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r, "id")
	if err != nil {
		app.notFound(w, r)
		return
	}

	invitation, err := app.invitations.Get(id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return
	}

	team, err := app.teams.Get(invitation.TeamID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	if !team.CaptainedBy(app.contextGetUser(r)) {
		app.clientError(w, r, http.StatusForbidden)
		return
	}

	err = app.invitations.SetStatus(invitation.ID, "cancelled")
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.redirectBack(w, r, "Invitation cancelled")
}

// ---------- "looking for a team" posts ----------

type findTeamPostForm struct {
	CompetitionID int64  `form:"competitionId"`
	Role          string `form:"role"`
	Skills        string `form:"skills"`
	Experience    string `form:"experience"`
	Description   string `form:"description"`
}

func (app *application) findTeamPostCreatePost(w http.ResponseWriter, r *http.Request) {
	var form findTeamPostForm

	err := app.decodePostForm(r, &form)
	if err != nil {
		app.clientError(w, r, http.StatusBadRequest)
		return
	}

	if form.CompetitionID == 0 || !validator.NotBlank(form.Role) {
		app.redirectBackWithError(w, r, "Pick a competition and the role you're looking for")
		return
	}

	if form.Experience != "" && !validator.PermittedValue(form.Experience, models.ExperienceLevels...) {
		app.clientError(w, r, http.StatusBadRequest)
		return
	}

	c, err := app.competitions.Get(form.CompetitionID)
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

	post := &models.Post{
		CompetitionID: c.ID,
		UserID:        user.ID,
		Role:          form.Role,
		Skills:        form.Skills,
		Experience:    form.Experience,
		Description:   form.Description,
	}

	err = app.posts.Insert(post)
	if err != nil {
		if errors.Is(err, models.ErrDuplicatePost) {
			app.redirectBackWithError(w, r, `You already have a "looking for a team" post for this competition`)
		} else {
			app.serverError(w, r, err)
		}
		return
	}

	app.sessionManager.Put(r.Context(), "flash", "Your post is live — captains can now invite you")
	http.Redirect(w, r, "/find-team?tab=players", http.StatusSeeOther)
}

func (app *application) findTeamPostDeletePost(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r, "id")
	if err != nil {
		app.notFound(w, r)
		return
	}

	post, err := app.posts.Get(id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return
	}

	user := app.contextGetUser(r)
	if post.UserID != user.ID && !user.IsAdmin() {
		app.clientError(w, r, http.StatusForbidden)
		return
	}

	err = app.posts.Delete(id)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.redirectBack(w, r, "Post removed")
}
