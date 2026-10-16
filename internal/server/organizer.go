package server

import (
	"net/http"
	"strconv"
	"strings"

	"arenasuite/internal/app"
)

func (s *Server) organizerOverviewPage(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	comps, err := s.app.ListCompetitions(user, app.Filter{Mine: true, Limit: 100})
	if err != nil {
		s.pageError(w, r, err)
		return
	}
	stats := map[string]int64{"Total": int64(len(comps))}
	for _, c := range comps {
		if c.Status == "published" || c.Status == "ongoing" {
			stats["Active"]++
		}
		stats["Registrations"] += c.RegistrationsCount
		stats["Teams"] += c.TeamsCount
		matches, err := s.app.MatchesFor(c.ID)
		if err != nil {
			s.pageError(w, r, err)
			return
		}
		for _, m := range matches {
			switch m.Status {
			case "scheduled", "live":
				stats["UpcomingMatches"]++
			case "completed":
				stats["CompletedMatches"]++
			}
		}
	}
	if len(comps) > 6 {
		comps = comps[:6]
	}
	s.render(w, r, http.StatusOK, "organizer-overview", "Organizer Dashboard", map[string]any{
		"Competitions": comps, "Stats": stats, "Section": "overview",
	})
}

func (s *Server) organizerCompetitionsPage(w http.ResponseWriter, r *http.Request) {
	comps, err := s.app.ListCompetitions(currentUser(r), app.Filter{Mine: true, Sort: "recent", Limit: 100})
	if err != nil {
		s.pageError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "organizer-competitions", "My Competitions", map[string]any{
		"Competitions": comps, "Section": "competitions",
	})
}

// ---------- create / edit wizard ----------

// The create and edit pages share one template ("competition-form").
// On a validation error we re-render the form with what was typed, rather
// than redirecting and losing it.

func (s *Server) newCompetitionPage(w http.ResponseWriter, r *http.Request) {
	in := app.CompetitionInput{
		Category: "SPORTS", Format: "knockout", Mode: "offline",
		Stages: []string{"Registration", "Team Formation", "Submission", "Evaluation", "Leaderboard", "Results"},
	}
	s.competitionForm(w, r, http.StatusOK, in, 0, "")
}

func (s *Server) editCompetitionPage(w http.ResponseWriter, r *http.Request) {
	c, err := s.app.CompetitionByID(currentUser(r), pathID(r, "id"))
	if err != nil {
		s.pageError(w, r, err)
		return
	}
	if !app.CanManage(currentUser(r), c) {
		s.forbidden(w, r)
		return
	}
	in := app.CompetitionInput{
		OrganizationID: c.OrganizationID, Name: c.Name, Description: c.Description, Category: c.Category,
		Type: c.Type, Banner: c.Banner, StartDate: c.StartDate, EndDate: c.EndDate,
		RegistrationDeadline: c.RegistrationDeadline, Venue: c.Venue, Mode: c.Mode,
		MaxTeamSize: c.MaxTeamSize, MaxTeams: c.MaxTeams, RegistrationFee: c.RegistrationFee,
		Rules: c.Rules, Format: c.Format, Stages: c.Stages(),
	}
	s.competitionForm(w, r, http.StatusOK, in, c.ID, "")
}

func (s *Server) competitionForm(w http.ResponseWriter, r *http.Request, status int, in app.CompetitionInput, editingID int64, errMsg string) {
	orgs, err := s.app.OrganizationsOwnedBy(currentUser(r).ID)
	if err != nil {
		s.pageError(w, r, err)
		return
	}
	if in.OrganizationID == 0 && len(orgs) == 1 {
		in.OrganizationID = orgs[0].ID
	}
	title := "Create Competition"
	if editingID != 0 {
		title = "Edit Competition"
	}
	s.render(w, r, status, "competition-form", title, map[string]any{
		"Form": in, "Stages": strings.Join(in.Stages, ", "), "Orgs": orgs,
		"EditingID": editingID, "Error": errMsg, "Section": "competitions", "Heading": title,
	})
}

func readCompetitionForm(r *http.Request) app.CompetitionInput {
	fee, _ := strconv.ParseFloat(formText(r, "registrationFee"), 64)
	var stages []string
	for _, st := range strings.Split(r.FormValue("stages"), ",") {
		if st = strings.TrimSpace(st); st != "" {
			stages = append(stages, st)
		}
	}
	return app.CompetitionInput{
		OrganizationID:       formInt(r, "organizationId"),
		Name:                 formText(r, "name"),
		Description:          formText(r, "description"),
		Category:             formText(r, "category"),
		Type:                 formText(r, "type"),
		Banner:               formText(r, "banner"),
		StartDate:            formText(r, "startDate"),
		EndDate:              formText(r, "endDate"),
		RegistrationDeadline: formText(r, "registrationDeadline"),
		Venue:                formText(r, "venue"),
		Mode:                 formText(r, "mode"),
		MaxTeamSize:          formInt(r, "maxTeamSize"),
		MaxTeams:             formInt(r, "maxTeams"),
		RegistrationFee:      fee,
		Rules:                formText(r, "rules"),
		Format:               formText(r, "format"),
		Stages:               stages,
		Status:               formText(r, "status"), // which button was pressed: draft or published
	}
}

func (s *Server) createCompetition(w http.ResponseWriter, r *http.Request) {
	in := readCompetitionForm(r)
	id, err := s.app.CreateCompetition(currentUser(r), in)
	if err != nil {
		s.competitionForm(w, r, http.StatusBadRequest, in, 0, userMessage(err))
		return
	}
	msg := "Draft saved"
	if in.Status == "published" {
		msg = "Competition published"
	}
	s.done(w, r, nil, msg, "/organizer/competitions/"+strconv.FormatInt(id, 10)+"/manage")
}

func (s *Server) updateCompetition(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	in := readCompetitionForm(r)
	if err := s.app.UpdateCompetition(currentUser(r), id, in); err != nil {
		s.competitionForm(w, r, http.StatusBadRequest, in, id, userMessage(err))
		return
	}
	s.done(w, r, nil, "Competition updated", "/organizer/competitions/"+strconv.FormatInt(id, 10)+"/manage")
}

// ---------- manage one competition ----------

func (s *Server) manageCompetitionPage(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	id := pathID(r, "id")
	c, err := s.app.CompetitionByID(user, id)
	if err != nil {
		s.pageError(w, r, err)
		return
	}
	if !app.CanManage(user, c) {
		s.forbidden(w, r)
		return
	}
	registrations, err1 := s.app.RegistrationsFor(user, id, "")
	teams, err2 := s.app.TeamsForCompetition(id)
	matches, err3 := s.app.MatchesFor(id)
	standings, err4 := s.app.StandingsFor(id)
	announcements, err5 := s.app.AnnouncementsFor(id)
	if err := firstError(err1, err2, err3, err4, err5); err != nil {
		s.pageError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "manage-competition", "Manage "+c.Name, map[string]any{
		"Competition": c, "Registrations": registrations, "Teams": teams, "Matches": matches,
		"Standings": standings, "Announcements": announcements, "Section": "competitions",
	})
}

func (s *Server) organizerSettingsPage(w http.ResponseWriter, r *http.Request) {
	orgs, err := s.app.OrganizationsOwnedBy(currentUser(r).ID)
	if err != nil {
		s.pageError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "organizer-settings", "Settings", map[string]any{"Orgs": orgs, "Section": "settings"})
}
