package models

import (
	"database/sql"
	"errors"
	"time"
)

type JoinRequest struct {
	ID        int64
	TeamID    int64
	UserID    int64
	Message   string
	Status    string
	CreatedAt time.Time
	Name      string
	Email     string
}

type JoinRequestModel struct {
	DB *sql.DB
}

func (m *JoinRequestModel) Insert(teamID, userID int64, message string) (int64, error) {
	stmt := `INSERT INTO team_join_requests (team_id, user_id, message) VALUES ($1, $2, NULLIF($3, '')) RETURNING id`

	var id int64
	err := m.DB.QueryRow(stmt, teamID, userID, message).Scan(&id)
	return id, err
}

func (m *JoinRequestModel) Get(id int64) (*JoinRequest, error) {
	stmt := `SELECT jr.id, jr.team_id, jr.user_id, COALESCE(jr.message, ''), jr.status, jr.created_at, u.name, u.email
	FROM team_join_requests jr
	JOIN users u ON u.id = jr.user_id
	WHERE jr.id = $1`

	r := &JoinRequest{}
	err := m.DB.QueryRow(stmt, id).Scan(&r.ID, &r.TeamID, &r.UserID, &r.Message, &r.Status, &r.CreatedAt, &r.Name, &r.Email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		}
		return nil, err
	}

	return r, nil
}

// HasPending reports whether a user already has a pending request for a team.
func (m *JoinRequestModel) HasPending(teamID, userID int64) (bool, error) {
	var exists bool

	stmt := `SELECT EXISTS(SELECT true FROM team_join_requests WHERE team_id = $1 AND user_id = $2 AND status = 'pending')`

	err := m.DB.QueryRow(stmt, teamID, userID).Scan(&exists)
	return exists, err
}

// PendingForTeam is what the captain sees on the team page.
func (m *JoinRequestModel) PendingForTeam(teamID int64) ([]JoinRequest, error) {
	stmt := `SELECT jr.id, jr.team_id, jr.user_id, COALESCE(jr.message, ''), jr.status, jr.created_at, u.name, u.email
	FROM team_join_requests jr
	JOIN users u ON u.id = jr.user_id
	WHERE jr.team_id = $1 AND jr.status = 'pending'
	ORDER BY jr.created_at DESC`

	rows, err := m.DB.Query(stmt, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var requests []JoinRequest

	for rows.Next() {
		var r JoinRequest
		err = rows.Scan(&r.ID, &r.TeamID, &r.UserID, &r.Message, &r.Status, &r.CreatedAt, &r.Name, &r.Email)
		if err != nil {
			return nil, err
		}
		requests = append(requests, r)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return requests, nil
}

// PendingForUser maps team id -> request id for every team a user has asked
// to join, so a page can show "Request sent · Withdraw" on those teams.
func (m *JoinRequestModel) PendingForUser(userID int64) (map[int64]int64, error) {
	stmt := `SELECT team_id, id FROM team_join_requests WHERE user_id = $1 AND status = 'pending'`

	rows, err := m.DB.Query(stmt, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byTeam := map[int64]int64{}

	for rows.Next() {
		var teamID, requestID int64
		if err = rows.Scan(&teamID, &requestID); err != nil {
			return nil, err
		}
		byTeam[teamID] = requestID
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return byTeam, nil
}

// Accept adds the requester to the team and marks the request accepted, both
// in one transaction.
func (m *JoinRequestModel) Accept(r *JoinRequest, t *Team) error {
	tx, err := m.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	err = addMember(tx, t, r.UserID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`UPDATE team_join_requests SET status = 'accepted', updated_at = now() WHERE id = $1`, r.ID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// SetStatus resolves a pending request as "rejected" or "cancelled".
func (m *JoinRequestModel) SetStatus(id int64, status string) error {
	stmt := `UPDATE team_join_requests SET status = $1, updated_at = now() WHERE id = $2`
	return execOne(m.DB, stmt, status, id)
}
