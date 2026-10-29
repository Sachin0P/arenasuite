package models

import (
	"database/sql"
	"errors"
	"time"
)

type Registration struct {
	ID              int64
	CompetitionID   int64
	UserID          int64
	TeamID          int64 // 0 = no team
	Status          string
	RegisteredAt    time.Time
	RegistrantName  string
	RegistrantEmail string
	TeamName        string
	CompetitionName string
}

type RegistrationModel struct {
	DB *sql.DB
}

const registrationSelect = `SELECT r.id, r.competition_id, r.user_id, COALESCE(r.team_id, 0), r.status, r.registered_at,
	u.name, u.email, COALESCE(t.name, ''), c.name
	FROM registrations r
	JOIN users u ON u.id = r.user_id
	JOIN competitions c ON c.id = r.competition_id
	LEFT JOIN teams t ON t.id = r.team_id`

func scanRegistration(row scanner) (*Registration, error) {
	r := &Registration{}
	err := row.Scan(&r.ID, &r.CompetitionID, &r.UserID, &r.TeamID, &r.Status, &r.RegisteredAt,
		&r.RegistrantName, &r.RegistrantEmail, &r.TeamName, &r.CompetitionName)
	return r, err
}

func (m *RegistrationModel) Get(id int64) (*Registration, error) {
	r, err := scanRegistration(m.DB.QueryRow(registrationSelect+` WHERE r.id = $1`, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		}
		return nil, err
	}

	return r, nil
}

// ForCompetition lists a competition's registrations for its organizer.
func (m *RegistrationModel) ForCompetition(competitionID int64) ([]Registration, error) {
	return m.list(registrationSelect+` WHERE r.competition_id = $1 ORDER BY r.registered_at DESC`, competitionID)
}

// ForUser includes team registrations made by a captain for a team the user
// is on, so every member sees their competitions.
func (m *RegistrationModel) ForUser(userID int64) ([]Registration, error) {
	stmt := registrationSelect + `
	WHERE r.user_id = $1
	OR r.team_id IN (SELECT team_id FROM team_members WHERE user_id = $1 AND status = 'active')
	ORDER BY r.registered_at DESC`

	return m.list(stmt, userID)
}

// GetActiveForUser returns the user's live (not cancelled) registration in a
// competition, either their own or their team's.
func (m *RegistrationModel) GetActiveForUser(userID, competitionID int64) (*Registration, error) {
	stmt := registrationSelect + `
	WHERE r.competition_id = $1 AND r.status <> 'cancelled'
	AND (r.user_id = $2 OR r.team_id IN (SELECT team_id FROM team_members WHERE user_id = $2 AND status = 'active'))`

	r, err := scanRegistration(m.DB.QueryRow(stmt, competitionID, userID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		}
		return nil, err
	}

	return r, nil
}

func (m *RegistrationModel) list(stmt string, args ...any) ([]Registration, error) {
	rows, err := m.DB.Query(stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var registrations []Registration

	for rows.Next() {
		r, err := scanRegistration(rows)
		if err != nil {
			return nil, err
		}
		registrations = append(registrations, *r)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return registrations, nil
}

// Insert registers an existing team for a competition. The registration
// starts as "pending" until the organizer reviews it.
func (m *RegistrationModel) Insert(c *Competition, userID, teamID int64) error {
	tx, err := m.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	err = insertRegistration(tx, c, userID, teamID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// InsertSolo is "Register Individually": the user gets a one-person team
// (named after them, with them as captain), which is then registered like
// any other team. That way solo players show up in team lists and fixtures.
func (m *RegistrationModel) InsertSolo(c *Competition, user *User) error {
	tx, err := m.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	onTeam, err := onTeamIn(tx, c.ID, user.ID)
	if err != nil {
		return err
	}
	if onTeam {
		return ErrAlreadyOnTeam
	}

	teamID, err := createSoloTeam(tx, c.ID, user)
	if err != nil {
		return err
	}

	err = insertRegistration(tx, c, user.ID, teamID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func insertRegistration(tx *sql.Tx, c *Competition, userID, teamID int64) error {
	// A cancelled registration shouldn't block signing up again, so clear it
	// out first (the table allows one row per user and one per team).
	stmt := `DELETE FROM registrations WHERE competition_id = $1 AND status = 'cancelled' AND (user_id = $2 OR team_id = $3)`

	_, err := tx.Exec(stmt, c.ID, userID, teamID)
	if err != nil {
		return err
	}

	if c.MaxTeams > 0 {
		var n int64
		stmt = `SELECT COUNT(*) FROM registrations WHERE competition_id = $1 AND team_id IS NOT NULL AND status <> 'cancelled'`

		err = tx.QueryRow(stmt, c.ID).Scan(&n)
		if err != nil {
			return err
		}
		if n >= c.MaxTeams {
			return ErrMaxTeamsReached
		}
	}

	// If any member is already registered (alone or with another team), the
	// whole team is blocked.
	var memberRegistered bool
	stmt = `SELECT EXISTS(
		SELECT true FROM registrations r
		JOIN team_members tm ON tm.user_id = r.user_id AND tm.team_id = $1 AND tm.status = 'active'
		WHERE r.competition_id = $2 AND r.status <> 'cancelled')`

	err = tx.QueryRow(stmt, teamID, c.ID).Scan(&memberRegistered)
	if err != nil {
		return err
	}
	if memberRegistered {
		return ErrAlreadyRegistered
	}

	stmt = `INSERT INTO registrations (competition_id, user_id, team_id) VALUES ($1, $2, $3)`

	_, err = tx.Exec(stmt, c.ID, userID, teamID)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrAlreadyRegistered
		}
		return err
	}

	return nil
}

// SetStatus records the organizer's review ("approved" or "rejected") or a
// cancellation.
func (m *RegistrationModel) SetStatus(id int64, status string) error {
	stmt := `UPDATE registrations SET status = $1, updated_at = now() WHERE id = $2`
	return execOne(m.DB, stmt, status, id)
}

// ApprovedParticipantIDs lists everyone taking part in a competition: approved
// individual registrants, plus every active member of an approved team.
func (m *RegistrationModel) ApprovedParticipantIDs(competitionID int64) ([]int64, error) {
	stmt := `SELECT r.user_id FROM registrations r
	WHERE r.competition_id = $1 AND r.status = 'approved' AND r.team_id IS NULL
	UNION
	SELECT tm.user_id FROM registrations r
	JOIN team_members tm ON tm.team_id = r.team_id AND tm.status = 'active'
	WHERE r.competition_id = $1 AND r.status = 'approved'`

	return queryIDs(m.DB, stmt, competitionID)
}
