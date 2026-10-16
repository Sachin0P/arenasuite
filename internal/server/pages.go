package server

import (
	"net/http"
	"strconv"

	"arenasuite/internal/app"
)

func (s *Server) homePage(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	sections := []struct {
		Title  string
		Filter app.Filter
	}{
		{"Upcoming Competitions", app.Filter{Status: "published", Limit: 6}},
		{"Registration Closing Soon", app.Filter{ClosingSoonDays: 14, Sort: "closingSoon", Limit: 6}},
		{"Popular Competitions", app.Filter{Sort: "popular", Limit: 6}},
		{"Recently Added", app.Filter{Sort: "recent", Limit: 6}},
	}
	type section struct {
		Title        string
		Competitions []app.Competition
	}
	var out []section
	for _, sec := range sections {
		list, err := s.app.ListCompetitions(user, sec.Filter)
		if err != nil {
			s.pageError(w, r, err)
			return
		}
		out = append(out, section{sec.Title, list})
	}
	s.render(w, r, http.StatusOK, "home", "", map[string]any{"Sections": out})
}

// explorePage is the search page; categoryPage is the same thing locked to
// one category, with a tinted title band.
func (s *Server) explorePage(w http.ResponseWriter, r *http.Request) {
	s.listing(w, r, "", "Explore Competitions")
}

func (s *Server) categoryPage(category string) http.HandlerFunc {
	titles := map[string]string{"SPORTS": "Sports", "ACADEMIC": "Academic", "ESPORTS": "Esports"}
	return func(w http.ResponseWriter, r *http.Request) {
		s.listing(w, r, category, titles[category])
	}
}

func (s *Server) listing(w http.ResponseWriter, r *http.Request, lockedCategory, title string) {
	q := r.URL.Query()
	f := app.Filter{
		Q:        q.Get("q"),
		Category: q.Get("category"),
		Mode:     q.Get("mode"),
		Free:     q.Get("free"),
		Sort:     q.Get("sort"),
		Limit:    60,
	}
	if lockedCategory != "" {
		f.Category = lockedCategory
	}
	if q.Get("open") == "true" {
		f.RegistrationOpen = true
	}
	list, err := s.app.ListCompetitions(currentUser(r), f)
	if err != nil {
		s.pageError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "explore", title, map[string]any{
		"Competitions":   list,
		"Filter":         f,
		"Open":           f.RegistrationOpen,
		"LockedCategory": lockedCategory,
		"Heading":        title,
	})
}

func (s *Server) competitionPage(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	id := pathID(r, "id")
	c, err := s.app.CompetitionByID(user, id)
	if err != nil {
		s.pageError(w, r, err)
		return
	}
	teams, err1 := s.app.TeamsForCompetition(id)
	matches, err2 := s.app.MatchesFor(id)
	standings, err3 := s.app.StandingsFor(id)
	announcements, err4 := s.app.AnnouncementsFor(id)
	if err := firstError(err1, err2, err3, err4); err != nil {
		s.pageError(w, r, err)
		return
	}

	data := map[string]any{
		"Competition":   c,
		"Teams":         teams,
		"Matches":       matches,
		"Standings":     standings,
		"Announcements": announcements,
		"CanManage":     app.CanManage(user, c),
		"LiveID":        c.ID, // makes the layout subscribe to this competition's live events
	}
	if user != nil {
		reg, err := s.app.MyRegistrationFor(user.ID, id)
		requests, err2 := s.app.MyPendingJoinRequests(user.ID)
		if err := firstError(err, err2); err != nil {
			s.pageError(w, r, err)
			return
		}
		var captainOf []app.Team
		onTeam := false
		for _, t := range teams {
			if t.CaptainID == user.ID {
				captainOf = append(captainOf, t)
			}
		}
		myTeams, _ := s.app.MyTeams(user.ID)
		for _, t := range myTeams {
			if t.CompetitionID == id {
				onTeam = true
			}
		}
		data["MyRegistration"] = reg
		data["CaptainOf"] = captainOf
		data["OnTeam"] = onTeam
		data["MyRequests"] = requests
	}
	s.render(w, r, http.StatusOK, "competition", c.Name, data)
}

func (s *Server) organizationPage(w http.ResponseWriter, r *http.Request) {
	org, err := s.app.OrganizationByID(pathID(r, "id"))
	if err != nil {
		s.pageError(w, r, err)
		return
	}
	upcoming, past, err := s.app.OrganizationCompetitions(org.ID)
	if err != nil {
		s.pageError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "organization", org.Name, map[string]any{
		"Org": org, "Upcoming": upcoming, "Past": past,
	})
}

func (s *Server) teamPage(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	team, err := s.app.TeamByID(pathID(r, "id"))
	if err != nil {
		s.pageError(w, r, err)
		return
	}
	isCaptain := user != nil && (team.CaptainID == user.ID || user.IsAdmin())
	data := map[string]any{
		"Team":      team,
		"IsCaptain": isCaptain,
		"IsMember":  user != nil && team.HasMember(user.ID),
	}
	if isCaptain {
		requests, err := s.app.PendingJoinRequests(user, team.ID)
		if err != nil {
			s.pageError(w, r, err)
			return
		}
		data["JoinRequests"] = requests
	}
	if user != nil {
		requests, _ := s.app.MyPendingJoinRequests(user.ID)
		data["MyRequestID"] = requests[team.ID]
	}
	s.render(w, r, http.StatusOK, "team", team.Name, data)
}

func (s *Server) findTeamPage(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	q := r.URL.Query()
	tab := q.Get("tab")
	if tab != "players" {
		tab = "teams"
	}
	compID, _ := strconv.ParseInt(q.Get("competitionId"), 10, 64)
	f := app.FindTeamFilter{CompetitionID: compID, Category: q.Get("category"), Role: q.Get("role")}

	// Competitions people can still form teams for, for the filter and post form.
	published, err := s.app.ListCompetitions(user, app.Filter{Status: "published", Limit: 100})
	if err != nil {
		s.pageError(w, r, err)
		return
	}
	data := map[string]any{"Tab": tab, "Filter": f, "Competitions": published}

	if tab == "teams" {
		teams, err := s.app.RecruitingTeams(f)
		if err != nil {
			s.pageError(w, r, err)
			return
		}
		data["Teams"] = teams
	} else {
		posts, err := s.app.OpenPosts(f)
		if err != nil {
			s.pageError(w, r, err)
			return
		}
		data["Posts"] = posts
	}
	if user != nil {
		requests, _ := s.app.MyPendingJoinRequests(user.ID)
		myTeams, _ := s.app.MyTeams(user.ID)
		var captainOf []app.Team
		for _, t := range myTeams {
			if t.CaptainID == user.ID {
				captainOf = append(captainOf, t)
			}
		}
		data["MyRequests"] = requests
		data["CaptainOf"] = captainOf
	}
	s.render(w, r, http.StatusOK, "find-team", "Find a Team", data)
}

func (s *Server) organizePage(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{}
	if user := currentUser(r); user != nil {
		orgs, err := s.app.OrganizationsOwnedBy(user.ID)
		if err != nil {
			s.pageError(w, r, err)
			return
		}
		data["Orgs"] = orgs
	}
	s.render(w, r, http.StatusOK, "organize", "Organize an Event", data)
}

// ---------- signed-in pages ----------

func (s *Server) profilePage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, "profile", "Profile", map[string]any{"DefaultCollege": s.app.Cfg.DefaultCollege})
}

func (s *Server) saveProfile(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	wasIncomplete := !user.ProfileComplete()
	err := s.app.UpdateProfile(user, formText(r, "college"), formText(r, "rollNumber"), formText(r, "branch"))
	to := "/profile"
	if wasIncomplete {
		to = "/dashboard" // first-time setup done, carry on to the dashboard
	}
	s.done(w, r, err, "Profile updated", to)
}

func (s *Server) dashboardPage(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	registrations, err1 := s.app.MyRegistrations(user.ID)
	teams, err2 := s.app.MyTeams(user.ID)
	matches, err3 := s.app.MyMatches(user.ID)
	invitations, err4 := s.app.MyInvitations(user.ID)
	if err := firstError(err1, err2, err3, err4); err != nil {
		s.pageError(w, r, err)
		return
	}

	var upcoming, results []app.Match
	for _, m := range matches {
		switch m.Status {
		case "scheduled", "live":
			upcoming = append(upcoming, m)
		case "completed":
			results = append([]app.Match{m}, results...) // newest first
		}
	}
	registered, approved := 0, 0
	for _, reg := range registrations {
		if reg.Status != "cancelled" {
			registered++
		}
		if reg.Status == "approved" {
			approved++
		}
	}
	s.render(w, r, http.StatusOK, "dashboard", "Dashboard", map[string]any{
		"Registrations": registrations,
		"Teams":         teams,
		"Upcoming":      upcoming,
		"Results":       results,
		"Invitations":   invitations,
		"Stats": map[string]int{
			"Registered": registered, "Teams": len(teams), "Upcoming": len(upcoming), "Approved": approved,
		},
	})
}

func (s *Server) notificationsPage(w http.ResponseWriter, r *http.Request) {
	list, err := s.app.NotificationsFor(currentUser(r).ID)
	if err != nil {
		s.pageError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "notifications", "Notifications", map[string]any{"Notifications": list})
}

func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
