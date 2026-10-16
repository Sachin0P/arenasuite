package app

import (
	"strings"

	"arenasuite/internal/db"
)

func (a *App) AnnouncementsFor(competitionID int64) ([]Announcement, error) {
	return db.All[Announcement](a.DB, `
		SELECT a.*, u.name AS created_by_name FROM announcements a JOIN users u ON u.id = a.created_by
		WHERE a.competition_id = ? ORDER BY a.created_at DESC, a.id DESC`, competitionID)
}

// CreateAnnouncement posts an announcement, pushes it live to everyone on
// the competition page, and notifies every approved participant (for a team
// registration, that's every active member of the team).
func (a *App) CreateAnnouncement(user *User, competitionID int64, title, message string) error {
	c, err := a.competitionToManage(user, competitionID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(title) == "" || strings.TrimSpace(message) == "" {
		return fail(400, "Title and message are required")
	}
	res, err := a.DB.Exec(`INSERT INTO announcements (competition_id, title, message, created_by) VALUES (?, ?, ?, ?)`,
		competitionID, title, message, user.ID)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()

	if ann, _ := db.One[Announcement](a.DB, `SELECT * FROM announcements WHERE id = ?`, id); ann != nil {
		a.toCompetition(competitionID, "announcementCreated", map[string]any{"id": ann.ID, "title": ann.Title})
	}

	participants, err := db.All[TeamMember](a.DB, `
		SELECT r.user_id FROM registrations r
		WHERE r.competition_id = ? AND r.status = 'approved' AND r.team_id IS NULL
		UNION
		SELECT tm.user_id FROM registrations r
		JOIN team_members tm ON tm.team_id = r.team_id AND tm.status = 'active'
		WHERE r.competition_id = ? AND r.status = 'approved'`, competitionID, competitionID)
	if err != nil {
		return err
	}
	for _, p := range participants {
		a.Notify(p.UserID, "announcement", c.Name+": "+title, id)
	}
	return nil
}

func (a *App) DeleteAnnouncement(user *User, announcementID int64) (competitionID int64, err error) {
	ann, err := db.One[Announcement](a.DB, `SELECT * FROM announcements WHERE id = ?`, announcementID)
	if err != nil {
		return 0, err
	}
	if ann == nil {
		return 0, fail(404, "Announcement not found")
	}
	if _, err := a.competitionToManage(user, ann.CompetitionID); err != nil {
		return 0, err
	}
	_, err = a.DB.Exec(`DELETE FROM announcements WHERE id = ?`, announcementID)
	return ann.CompetitionID, err
}
