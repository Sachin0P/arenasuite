package app

import (
	"strings"

	"arenasuite/internal/db"
)

// The admin methods assume the caller already checked user.IsAdmin()
// (the server does this for every /admin route), except CreateReport which
// any signed-in user may use.

type PlatformStats struct {
	TotalUsers, TotalOrganizations, TotalCompetitions, ActiveCompetitions int64
	TotalTeams, TotalRegistrations, OpenReports                           int64
	ByCategory                                                            map[string]int64
}

func (a *App) Stats() (PlatformStats, error) {
	var s PlatformStats
	counts := []struct {
		dest  *int64
		query string
	}{
		{&s.TotalUsers, `SELECT COUNT(*) FROM users`},
		{&s.TotalOrganizations, `SELECT COUNT(*) FROM organizations`},
		{&s.TotalCompetitions, `SELECT COUNT(*) FROM competitions`},
		{&s.ActiveCompetitions, `SELECT COUNT(*) FROM competitions WHERE status IN ('published', 'ongoing')`},
		{&s.TotalTeams, `SELECT COUNT(*) FROM teams`},
		{&s.TotalRegistrations, `SELECT COUNT(*) FROM registrations`},
		{&s.OpenReports, `SELECT COUNT(*) FROM reports WHERE status = 'open'`},
	}
	for _, c := range counts {
		n, err := db.Count(a.DB, c.query)
		if err != nil {
			return s, err
		}
		*c.dest = n
	}
	s.ByCategory = map[string]int64{}
	for _, cat := range Categories {
		n, err := db.Count(a.DB, `SELECT COUNT(*) FROM competitions WHERE category = ?`, cat)
		if err != nil {
			return s, err
		}
		s.ByCategory[cat] = n
	}
	return s, nil
}

func (a *App) SearchUsers(role, status, q string) ([]User, error) {
	where := []string{"1 = 1"}
	var args []any
	if role != "" {
		where = append(where, "role = ?")
		args = append(args, role)
	}
	if status != "" {
		where = append(where, "status = ?")
		args = append(args, status)
	}
	if q != "" {
		where = append(where, "(name LIKE ? OR email LIKE ?)")
		args = append(args, "%"+q+"%", "%"+q+"%")
	}
	return db.All[User](a.DB, `SELECT * FROM users WHERE `+strings.Join(where, " AND ")+` ORDER BY created_at DESC`, args...)
}

func (a *App) SetUserStatus(admin *User, userID int64, status string) error {
	if err := mustBeOneOf(status, []string{"active", "suspended"}, "status"); err != nil {
		return err
	}
	if userID == admin.ID {
		return fail(400, "You cannot change your own status")
	}
	return a.updateUser(userID, `UPDATE users SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, status)
}

func (a *App) SetUserRole(userID int64, role string) error {
	if err := mustBeOneOf(role, Roles, "role"); err != nil {
		return err
	}
	return a.updateUser(userID, `UPDATE users SET role = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, role)
}

func (a *App) updateUser(userID int64, query, value string) error {
	res, err := a.DB.Exec(query, value, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fail(404, "User not found")
	}
	return nil
}

func (a *App) VerifyOrganization(orgID int64) error {
	res, err := a.DB.Exec(`UPDATE organizations SET verified = 1, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, orgID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fail(404, "Organization not found")
	}
	return nil
}

// AllCompetitions includes drafts, for the admin list.
func (a *App) AllCompetitions() ([]Competition, error) {
	return db.All[Competition](a.DB, competitionSelect+` ORDER BY c.created_at DESC`)
}

// HideCompetition force-cancels a competition.
func (a *App) HideCompetition(competitionID int64) error {
	res, err := a.DB.Exec(`UPDATE competitions SET status = 'cancelled', updated_at = CURRENT_TIMESTAMP WHERE id = ?`, competitionID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fail(404, "Competition not found")
	}
	return nil
}

func (a *App) CreateReport(user *User, targetType string, targetID int64, reason string) error {
	if err := mustBeOneOf(targetType, []string{"competition", "team", "user", "announcement"}, "target type"); err != nil {
		return err
	}
	if targetID == 0 || strings.TrimSpace(reason) == "" {
		return fail(400, "Please describe the problem")
	}
	_, err := a.DB.Exec(`INSERT INTO reports (reporter_id, target_type, target_id, reason) VALUES (?, ?, ?, ?)`,
		user.ID, targetType, targetID, reason)
	return err
}

func (a *App) Reports(status string) ([]Report, error) {
	query := `SELECT r.*, u.name AS reporter_name FROM reports r JOIN users u ON u.id = r.reporter_id`
	if status != "" {
		return db.All[Report](a.DB, query+` WHERE r.status = ? ORDER BY r.created_at DESC`, status)
	}
	return db.All[Report](a.DB, query+` ORDER BY r.created_at DESC`)
}

func (a *App) ResolveReport(reportID int64, status string) error {
	if err := mustBeOneOf(status, []string{"resolved", "dismissed"}, "status"); err != nil {
		return err
	}
	res, err := a.DB.Exec(`UPDATE reports SET status = ?, resolved_at = CURRENT_TIMESTAMP WHERE id = ?`, status, reportID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fail(404, "Report not found")
	}
	return nil
}
