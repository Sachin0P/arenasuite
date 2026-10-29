package models

import (
	"database/sql"
	"errors"
	"time"
)

type Invitation struct {
	ID            int64
	TeamID        int64
	UserID        int64
	InvitedBy     int64
	Status        string
	CreatedAt     time.Time
	TeamName      string
	CompetitionID int64
	InviterName   string
}

type InvitationModel struct {
	DB *sql.DB
}

const invitationSelect = `SELECT ti.id, ti.team_id, ti.user_id, ti.invited_by, ti.status, ti.created_at,
	t.name, t.competition_id, u.name
	FROM team_invitations ti
	JOIN teams t ON t.id = ti.team_id
	JOIN users u ON u.id = ti.invited_by`

func scanInvitation(row scanner) (*Invitation, error) {
	i := &Invitation{}
	err := row.Scan(&i.ID, &i.TeamID, &i.UserID, &i.InvitedBy, &i.Status, &i.CreatedAt,
		&i.TeamName, &i.CompetitionID, &i.InviterName)
	return i, err
}

func (m *InvitationModel) Insert(teamID, userID, invitedBy int64) error {
	stmt := `INSERT INTO team_invitations (team_id, user_id, invited_by) VALUES ($1, $2, $3)`

	_, err := m.DB.Exec(stmt, teamID, userID, invitedBy)
	return err
}

func (m *InvitationModel) Get(id int64) (*Invitation, error) {
	i, err := scanInvitation(m.DB.QueryRow(invitationSelect+` WHERE ti.id = $1`, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		}
		return nil, err
	}

	return i, nil
}

// HasPending reports whether a user already has a pending invitation to a team.
func (m *InvitationModel) HasPending(teamID, userID int64) (bool, error) {
	var exists bool

	stmt := `SELECT EXISTS(SELECT true FROM team_invitations WHERE team_id = $1 AND user_id = $2 AND status = 'pending')`

	err := m.DB.QueryRow(stmt, teamID, userID).Scan(&exists)
	return exists, err
}

// PendingForUser lists the invitations waiting for a user's answer.
func (m *InvitationModel) PendingForUser(userID int64) ([]Invitation, error) {
	stmt := invitationSelect + ` WHERE ti.user_id = $1 AND ti.status = 'pending' ORDER BY ti.created_at DESC`

	rows, err := m.DB.Query(stmt, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var invitations []Invitation

	for rows.Next() {
		i, err := scanInvitation(rows)
		if err != nil {
			return nil, err
		}
		invitations = append(invitations, *i)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return invitations, nil
}

// Accept adds the invited user to the team and marks the invitation
// accepted, both in one transaction.
func (m *InvitationModel) Accept(i *Invitation, t *Team) error {
	tx, err := m.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	err = addMember(tx, t, i.UserID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`UPDATE team_invitations SET status = 'accepted', updated_at = now() WHERE id = $1`, i.ID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// SetStatus resolves a pending invitation as "rejected" or "cancelled".
func (m *InvitationModel) SetStatus(id int64, status string) error {
	stmt := `UPDATE team_invitations SET status = $1, updated_at = now() WHERE id = $2`
	return execOne(m.DB, stmt, status, id)
}
