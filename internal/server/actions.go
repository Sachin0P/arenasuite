package server

import (
	"fmt"
	"net/http"

	"arenasuite/internal/app"
)

// Form actions. Each one reads the form, calls package app (which checks
// permissions and rules), then redirects with a success or error message.
// Most redirect "back" to the page the form was on (see back()).

// ---------- organizations ----------

func (s *Server) createOrganization(w http.ResponseWriter, r *http.Request) {
	_, err := s.app.CreateOrganization(currentUser(r), orgForm(r))
	s.done(w, r, err, "Organization created — you are now an organizer", "/organizer")
}

func (s *Server) updateOrganization(w http.ResponseWriter, r *http.Request) {
	err := s.app.UpdateOrganization(currentUser(r), pathID(r, "id"), orgForm(r))
	s.done(w, r, err, "Organization saved", "")
}

func orgForm(r *http.Request) app.OrganizationInput {
	return app.OrganizationInput{
		Name: formText(r, "name"), Description: formText(r, "description"),
		Logo: formText(r, "logo"), College: formText(r, "college"),
	}
}

// ---------- competitions ----------

func (s *Server) setCompetitionStatus(w http.ResponseWriter, r *http.Request) {
	status := formText(r, "status")
	err := s.app.SetCompetitionStatus(currentUser(r), pathID(r, "id"), status)
	s.done(w, r, err, "Competition is now "+status, "")
}

func (s *Server) deleteCompetition(w http.ResponseWriter, r *http.Request) {
	err := s.app.DeleteCompetition(currentUser(r), pathID(r, "id"))
	s.done(w, r, err, "Competition deleted", "/organizer/competitions")
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	teamID := formInt(r, "teamId")
	err := s.app.Register(currentUser(r), pathID(r, "id"), teamID)
	msg := "Registration submitted — awaiting organizer approval"
	if teamID != 0 {
		msg = "Team registration submitted — awaiting organizer approval"
	}
	s.done(w, r, err, msg, "")
}

// ---------- registrations ----------

func (s *Server) reviewRegistration(approve bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, err := s.app.ReviewRegistration(currentUser(r), pathID(r, "id"), approve)
		msg := "Registration rejected"
		if approve {
			msg = "Registration approved"
		}
		s.done(w, r, err, msg, "")
	}
}

func (s *Server) cancelRegistration(w http.ResponseWriter, r *http.Request) {
	_, err := s.app.CancelRegistration(currentUser(r), pathID(r, "id"))
	s.done(w, r, err, "Registration cancelled", "")
}

// ---------- teams ----------

func teamForm(r *http.Request) app.TeamInput {
	return app.TeamInput{
		Name:              formText(r, "name"),
		Description:       formText(r, "description"),
		MaxMembers:        formInt(r, "maxMembers"),
		LookingForMembers: r.FormValue("lookingForMembers") == "on",
		LookingForRole:    formText(r, "lookingForRole"),
		RequiredSkills:    formText(r, "requiredSkills"),
	}
}

func (s *Server) createTeam(w http.ResponseWriter, r *http.Request) {
	id, err := s.app.CreateTeam(currentUser(r), pathID(r, "id"), teamForm(r))
	s.done(w, r, err, "Team created", fmt.Sprintf("/teams/%d", id))
}

func (s *Server) updateTeam(w http.ResponseWriter, r *http.Request) {
	err := s.app.UpdateTeam(currentUser(r), pathID(r, "id"), teamForm(r))
	s.done(w, r, err, "Team saved", "")
}

func (s *Server) deleteTeam(w http.ResponseWriter, r *http.Request) {
	competitionID, err := s.app.DeleteTeam(currentUser(r), pathID(r, "id"))
	s.done(w, r, err, "Team disbanded", fmt.Sprintf("/competitions/%d#teams", competitionID))
}

func (s *Server) leaveTeam(w http.ResponseWriter, r *http.Request) {
	err := s.app.LeaveTeam(currentUser(r), pathID(r, "id"))
	s.done(w, r, err, "You left the team", "/dashboard")
}

func (s *Server) transferCaptain(w http.ResponseWriter, r *http.Request) {
	err := s.app.TransferCaptain(currentUser(r), pathID(r, "id"), formInt(r, "userId"))
	s.done(w, r, err, "Captaincy transferred", "")
}

func (s *Server) removeMember(w http.ResponseWriter, r *http.Request) {
	err := s.app.RemoveMember(currentUser(r), pathID(r, "id"), pathID(r, "userId"))
	s.done(w, r, err, "Member removed", "")
}

func (s *Server) requestToJoin(w http.ResponseWriter, r *http.Request) {
	err := s.app.RequestToJoin(currentUser(r), pathID(r, "id"), formText(r, "message"))
	s.done(w, r, err, "Join request sent to the captain", "")
}

func (s *Server) respondJoinRequest(accept bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := s.app.RespondJoinRequest(currentUser(r), pathID(r, "id"), accept)
		msg := "Request rejected"
		if accept {
			msg = "Request accepted — they're on the team"
		}
		s.done(w, r, err, msg, "")
	}
}

func (s *Server) cancelJoinRequest(w http.ResponseWriter, r *http.Request) {
	s.done(w, r, s.app.CancelJoinRequest(currentUser(r), pathID(r, "id")), "Request withdrawn", "")
}

func (s *Server) inviteMember(w http.ResponseWriter, r *http.Request) {
	err := s.app.InviteMember(currentUser(r), pathID(r, "id"), formInt(r, "userId"))
	s.done(w, r, err, "Invitation sent", "")
}

func (s *Server) respondInvitation(accept bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := s.app.RespondInvitation(currentUser(r), pathID(r, "id"), accept)
		msg := "Invitation declined"
		if accept {
			msg = "Welcome to the team!"
		}
		s.done(w, r, err, msg, "")
	}
}

func (s *Server) cancelInvitation(w http.ResponseWriter, r *http.Request) {
	s.done(w, r, s.app.CancelInvitation(currentUser(r), pathID(r, "id")), "Invitation cancelled", "")
}

// ---------- find a team ----------

func (s *Server) createPost(w http.ResponseWriter, r *http.Request) {
	err := s.app.CreatePost(currentUser(r), app.PostInput{
		CompetitionID: formInt(r, "competitionId"),
		Role:          formText(r, "role"),
		Skills:        formText(r, "skills"),
		Experience:    formText(r, "experience"),
		Description:   formText(r, "description"),
	})
	s.done(w, r, err, "Your post is live — captains can now invite you", "/find-team?tab=players")
}

func (s *Server) deletePost(w http.ResponseWriter, r *http.Request) {
	s.done(w, r, s.app.DeletePost(currentUser(r), pathID(r, "id")), "Post removed", "")
}

// ---------- matches & standings ----------

func (s *Server) createMatch(w http.ResponseWriter, r *http.Request) {
	err := s.app.CreateMatch(currentUser(r), pathID(r, "id"), app.MatchInput{
		Round:       formText(r, "round"),
		TeamAID:     formInt(r, "teamAId"),
		TeamBID:     formInt(r, "teamBId"),
		ScheduledAt: formText(r, "scheduledAt"),
		Venue:       formText(r, "venue"),
	})
	s.done(w, r, err, "Match added", "")
}

func (s *Server) generateFixtures(w http.ResponseWriter, r *http.Request) {
	err := s.app.GenerateFixtures(currentUser(r), pathID(r, "id"), r.FormValue("regenerate") == "true")
	s.done(w, r, err, "Fixtures generated", "")
}

func (s *Server) updateMatch(w http.ResponseWriter, r *http.Request) {
	_, err := s.app.UpdateMatch(currentUser(r), pathID(r, "id"), app.MatchUpdate{
		ScoreA:      formOptionalInt(r, "scoreA"),
		ScoreB:      formOptionalInt(r, "scoreB"),
		Status:      formText(r, "status"),
		ScheduledAt: formText(r, "scheduledAt"),
		Venue:       formText(r, "venue"),
	})
	s.done(w, r, err, "Match saved", "")
}

func (s *Server) deleteMatch(w http.ResponseWriter, r *http.Request) {
	_, err := s.app.DeleteMatch(currentUser(r), pathID(r, "id"))
	s.done(w, r, err, "Match deleted", "")
}

func (s *Server) recomputeStandings(w http.ResponseWriter, r *http.Request) {
	s.done(w, r, s.app.RecomputeStandings(currentUser(r), pathID(r, "id")), "Standings recalculated", "")
}

func (s *Server) setAcademicScore(w http.ResponseWriter, r *http.Request) {
	err := s.app.SetAcademicScore(currentUser(r), pathID(r, "id"), formInt(r, "teamId"), formInt(r, "points"))
	s.done(w, r, err, "Score saved", "")
}

// ---------- announcements, notifications, reports ----------

func (s *Server) createAnnouncement(w http.ResponseWriter, r *http.Request) {
	err := s.app.CreateAnnouncement(currentUser(r), pathID(r, "id"), formText(r, "title"), formText(r, "message"))
	s.done(w, r, err, "Announcement published", "")
}

func (s *Server) deleteAnnouncement(w http.ResponseWriter, r *http.Request) {
	_, err := s.app.DeleteAnnouncement(currentUser(r), pathID(r, "id"))
	s.done(w, r, err, "Announcement deleted", "")
}

func (s *Server) markRead(w http.ResponseWriter, r *http.Request) {
	s.done(w, r, s.app.MarkRead(currentUser(r), pathID(r, "id")), "", "")
}

func (s *Server) markAllRead(w http.ResponseWriter, r *http.Request) {
	s.done(w, r, s.app.MarkAllRead(currentUser(r)), "All caught up", "")
}

func (s *Server) createReport(w http.ResponseWriter, r *http.Request) {
	err := s.app.CreateReport(currentUser(r), formText(r, "targetType"), formInt(r, "targetId"), formText(r, "reason"))
	s.done(w, r, err, "Thanks — an admin will take a look", "")
}
