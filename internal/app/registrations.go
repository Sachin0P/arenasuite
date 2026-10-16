package app

import (
	"arenasuite/internal/db"
)

const registrationSelect = `
	SELECT r.*, u.name AS registrant_name, u.email AS registrant_email,
		t.name AS team_name, c.name AS competition_name
	FROM registrations r
	JOIN users u ON u.id = r.user_id
	JOIN competitions c ON c.id = r.competition_id
	LEFT JOIN teams t ON t.id = r.team_id`

func (a *App) getRegistration(id int64) (*Registration, error) {
	r, err := db.One[Registration](a.DB, registrationSelect+` WHERE r.id = ?`, id)
	if err == nil && r == nil {
		return nil, fail(404, "Registration not found")
	}
	return r, err
}

// Register signs the user up for a competition: individually when teamID is
// 0, otherwise on behalf of a team they captain. It starts as "pending" until
// the organizer approves it, so the organizer is notified.
func (a *App) Register(user *User, competitionID, teamID int64) error {
	c, err := a.getCompetition(a.DB, competitionID)
	if err != nil {
		return err
	}
	if !c.RegistrationOpen() {
		return fail(400, "Registration is closed for this competition")
	}

	who := user.Name
	err = a.inTx(func(tx *db.Tx) error {
		// A cancelled registration shouldn't block signing up again, so clear
		// it out first (the table allows one row per user / team).
		if _, err := tx.Exec(`DELETE FROM registrations WHERE competition_id = ? AND status = 'cancelled' AND (user_id = ? OR team_id = ?)`,
			competitionID, user.ID, db.NullIfZero(teamID)); err != nil {
			return err
		}

		if teamID == 0 {
			// Blocked if they registered before, or a team they're on did.
			n, err := db.Count(tx, `
				SELECT COUNT(*) FROM registrations
				WHERE competition_id = ? AND status != 'cancelled'
				  AND (user_id = ? OR team_id IN (SELECT team_id FROM team_members WHERE user_id = ? AND status = 'active'))`,
				competitionID, user.ID, user.ID)
			if err != nil {
				return err
			}
			if n > 0 {
				return fail(409, "You are already registered for this competition")
			}
			_, err = tx.Exec(`INSERT INTO registrations (competition_id, user_id) VALUES (?, ?)`, competitionID, user.ID)
			return err
		}

		t, err := a.getTeam(tx, teamID)
		if err != nil {
			return err
		}
		if t.CompetitionID != competitionID {
			return fail(404, "Team not found for this competition")
		}
		if t.CaptainID != user.ID && !user.IsAdmin() {
			return fail(403, "Only the team captain can register this team")
		}
		if c.MaxTeams > 0 {
			n, err := db.Count(tx, `SELECT COUNT(*) FROM registrations WHERE competition_id = ? AND team_id IS NOT NULL AND status != 'cancelled'`, competitionID)
			if err != nil {
				return err
			}
			if n >= c.MaxTeams {
				return fail(400, "This competition has reached its maximum number of teams")
			}
		}

		// If any member is already registered (alone or with another team),
		// the whole team is blocked.
		n, err := db.Count(tx, `
			SELECT COUNT(*) FROM registrations r
			JOIN team_members tm ON tm.user_id = r.user_id AND tm.team_id = ? AND tm.status = 'active'
			WHERE r.competition_id = ? AND r.status != 'cancelled'`, teamID, competitionID)
		if err != nil {
			return err
		}
		if n > 0 {
			return fail(409, "One or more team members are already registered for this competition")
		}

		_, err = tx.Exec(`INSERT INTO registrations (competition_id, user_id, team_id) VALUES (?, ?, ?)`, competitionID, user.ID, teamID)
		if db.IsUniqueViolation(err) {
			return fail(409, "This team is already registered")
		}
		who = "Team " + t.Name
		return err
	})
	if err != nil {
		return err
	}
	a.Notify(c.OrgOwnerID, "registrationSubmitted", who+" registered for "+c.Name+" and is awaiting your approval", competitionID)
	a.toCompetition(competitionID, "registrationSubmitted", map[string]int64{"competitionId": competitionID})
	return nil
}

// RegistrationsFor lists a competition's registrations for its organizer.
func (a *App) RegistrationsFor(user *User, competitionID int64, status string) ([]Registration, error) {
	if _, err := a.competitionToManage(user, competitionID); err != nil {
		return nil, err
	}
	if status != "" {
		return db.All[Registration](a.DB, registrationSelect+` WHERE r.competition_id = ? AND r.status = ? ORDER BY r.registered_at DESC`, competitionID, status)
	}
	return db.All[Registration](a.DB, registrationSelect+` WHERE r.competition_id = ? ORDER BY r.registered_at DESC`, competitionID)
}

// MyRegistrations includes team registrations made by a captain for a team
// the user is on, so every member sees their competitions.
func (a *App) MyRegistrations(userID int64) ([]Registration, error) {
	return db.All[Registration](a.DB, registrationSelect+`
		WHERE r.user_id = ?
		   OR r.team_id IN (SELECT team_id FROM team_members WHERE user_id = ? AND status = 'active')
		ORDER BY r.registered_at DESC`, userID, userID)
}

// MyRegistrationFor returns the user's live (not cancelled) registration in a
// competition, either their own or their team's, or nil.
func (a *App) MyRegistrationFor(userID, competitionID int64) (*Registration, error) {
	return db.One[Registration](a.DB, registrationSelect+`
		WHERE r.competition_id = ? AND r.status != 'cancelled'
		  AND (r.user_id = ? OR r.team_id IN (SELECT team_id FROM team_members WHERE user_id = ? AND status = 'active'))`,
		competitionID, userID, userID)
}

// ReviewRegistration approves or rejects a pending registration and tells
// everyone affected (the individual, or every member of the team).
func (a *App) ReviewRegistration(user *User, registrationID int64, approve bool) (competitionID int64, err error) {
	reg, err := a.getRegistration(registrationID)
	if err != nil {
		return 0, err
	}
	if _, err := a.competitionToManage(user, reg.CompetitionID); err != nil {
		return 0, err
	}
	if reg.Status != "pending" {
		return 0, fail(400, "This registration has already been reviewed")
	}

	status, event, kind := "rejected", "registrationRejected", "registrationRejected"
	if approve {
		status, event, kind = "approved", "registrationApproved", "registrationApproved"
	}
	if _, err := a.DB.Exec(`UPDATE registrations SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, status, reg.ID); err != nil {
		return 0, err
	}

	affected := []int64{reg.UserID}
	if reg.TeamID != 0 {
		if affected, err = activeMemberIDs(a.DB, reg.TeamID); err != nil {
			return 0, err
		}
	}
	for _, id := range affected {
		a.Notify(id, kind, "Your registration for "+reg.CompetitionName+" was "+status, reg.CompetitionID)
	}
	a.toCompetition(reg.CompetitionID, event, map[string]int64{"registrationId": reg.ID})
	return reg.CompetitionID, nil
}

// CancelRegistration can be done by whoever registered (the captain for a
// team) or an admin.
func (a *App) CancelRegistration(user *User, registrationID int64) (competitionID int64, err error) {
	reg, err := a.getRegistration(registrationID)
	if err != nil {
		return 0, err
	}
	if reg.UserID != user.ID && !user.IsAdmin() {
		return 0, fail(403, "You cannot cancel this registration")
	}
	if reg.Status == "cancelled" {
		return 0, fail(400, "This registration is already cancelled")
	}
	_, err = a.DB.Exec(`UPDATE registrations SET status = 'cancelled', updated_at = CURRENT_TIMESTAMP WHERE id = ?`, reg.ID)
	return reg.CompetitionID, err
}
