package server

import "net/http"

// All /admin routes are wrapped in adminOnly (see routes), so these handlers
// can assume the user is an admin.

func (s *Server) adminOverviewPage(w http.ResponseWriter, r *http.Request) {
	stats, err := s.app.Stats()
	if err != nil {
		s.pageError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "admin-overview", "Admin Panel", map[string]any{"Stats": stats, "Section": "overview"})
}

func (s *Server) adminUsersPage(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	users, err := s.app.SearchUsers(q.Get("role"), q.Get("status"), q.Get("q"))
	if err != nil {
		s.pageError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "admin-users", "Users", map[string]any{
		"Users": users, "Role": q.Get("role"), "Status": q.Get("status"), "Q": q.Get("q"), "Section": "users",
	})
}

func (s *Server) adminOrganizationsPage(w http.ResponseWriter, r *http.Request) {
	orgs, err := s.app.ListOrganizations()
	if err != nil {
		s.pageError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "admin-organizations", "Organizations", map[string]any{"Orgs": orgs, "Section": "organizations"})
}

func (s *Server) adminCompetitionsPage(w http.ResponseWriter, r *http.Request) {
	comps, err := s.app.AllCompetitions()
	if err != nil {
		s.pageError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "admin-competitions", "All Competitions", map[string]any{"Competitions": comps, "Section": "competitions"})
}

func (s *Server) adminReportsPage(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	reports, err := s.app.Reports(status)
	if err != nil {
		s.pageError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "admin-reports", "Reports", map[string]any{"Reports": reports, "Status": status, "Section": "reports"})
}

func (s *Server) adminSetUserStatus(w http.ResponseWriter, r *http.Request) {
	err := s.app.SetUserStatus(currentUser(r), pathID(r, "id"), formText(r, "status"))
	s.done(w, r, err, "User status updated", "")
}

func (s *Server) adminSetUserRole(w http.ResponseWriter, r *http.Request) {
	err := s.app.SetUserRole(pathID(r, "id"), formText(r, "role"))
	s.done(w, r, err, "User role updated", "")
}

func (s *Server) adminVerifyOrganization(w http.ResponseWriter, r *http.Request) {
	s.done(w, r, s.app.VerifyOrganization(pathID(r, "id")), "Organization verified", "")
}

func (s *Server) adminHideCompetition(w http.ResponseWriter, r *http.Request) {
	s.done(w, r, s.app.HideCompetition(pathID(r, "id")), "Competition hidden (cancelled)", "")
}

func (s *Server) adminResolveReport(w http.ResponseWriter, r *http.Request) {
	s.done(w, r, s.app.ResolveReport(pathID(r, "id"), formText(r, "status")), "Report updated", "")
}
