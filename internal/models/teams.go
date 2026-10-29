package models

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type Team struct {
	ID                int64
	CompetitionID     int64
	Name              string
	Description       string
	CaptainID         int64
	MaxMembers        int64
	RequiredSkills    string
	LookingForMembers bool
	LookingForRole    string
	Status            string
	CreatedAt         time.Time

	CaptainName     string
	MemberCount     int64
	CompetitionName string
	Category        string
	MaxTeamSize     int64 // the competition's limit, 0 = none
	OrganizerID     int64 // who runs the team's competition

	Members []TeamMember // only filled in when the handler loads them
}

// The methods on Team (and Competition) use value receivers so templates can
// call them on a copy, e.g. a Team passed to a partial through dict.

// Capacity is how many people the team can hold: its own size, capped by the
// competition's limit (in case the organizer lowered it after the team formed).
func (t Team) Capacity() int64 {
	if t.MaxTeamSize > 0 && t.MaxTeamSize < t.MaxMembers {
		return t.MaxTeamSize
	}
	return t.MaxMembers
}

func (t Team) IsFull() bool { return t.MemberCount >= t.Capacity() }

// OrganizedBy reports whether u runs the competition this team is in.
func (t Team) OrganizedBy(u *User) bool { return u != nil && t.OrganizerID == u.ID }

// CaptainedBy reports whether u may act as captain: the captain or an admin.
func (t Team) CaptainedBy(u *User) bool { return u.IsAdmin() || (u != nil && t.CaptainID == u.ID) }

// HasMember reports whether a user is an active member. Members must be loaded.
func (t Team) HasMember(userID int64) bool {
	for _, m := range t.Members {
		if m.UserID == userID {
			return true
		}
	}
	return false
}

type TeamMember struct {
	TeamID         int64
	UserID         int64
	Role           string // captain | member
	JoinedAt       time.Time
	Name           string
	Email          string
	ProfilePicture string
}

type TeamModel struct {
	DB *sql.DB
}

const teamSelect = `SELECT t.id, t.competition_id, t.name, COALESCE(t.description, ''), t.captain_id,
	t.max_members, COALESCE(t.required_skills, ''), t.looking_for_members, COALESCE(t.looking_for_role, ''),
	t.status, t.created_at,
	u.name, c.name, c.category, COALESCE(c.max_participants, 0), o.created_by,
	(SELECT COUNT(*) FROM team_members m WHERE m.team_id = t.id AND m.status = 'active')
	FROM teams t
	JOIN users u ON u.id = t.captain_id
	JOIN competitions c ON c.id = t.competition_id
	JOIN organizations o ON o.id = c.organization_id`

func scanTeam(row scanner) (*Team, error) {
	t := &Team{}
	err := row.Scan(&t.ID, &t.CompetitionID, &t.Name, &t.Description, &t.CaptainID,
		&t.MaxMembers, &t.RequiredSkills, &t.LookingForMembers, &t.LookingForRole,
		&t.Status, &t.CreatedAt,
		&t.CaptainName, &t.CompetitionName, &t.Category, &t.MaxTeamSize, &t.OrganizerID,
		&t.MemberCount)
	return t, err
}

func getTeam(q querier, id int64) (*Team, error) {
	t, err := scanTeam(q.QueryRow(teamSelect+` WHERE t.id = $1`, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		}
		return nil, err
	}

	return t, nil
}

func (m *TeamModel) Get(id int64) (*Team, error) {
	return getTeam(m.DB, id)
}

// Members returns a team's active members, captain first.
func (m *TeamModel) Members(teamID int64) ([]TeamMember, error) {
	stmt := `SELECT tm.team_id, tm.user_id, tm.role, tm.joined_at, u.name, u.email, COALESCE(u.profile_picture, '')
	FROM team_members tm
	JOIN users u ON u.id = tm.user_id
	WHERE tm.team_id = $1 AND tm.status = 'active'
	ORDER BY tm.role ASC, tm.joined_at ASC`

	rows, err := m.DB.Query(stmt, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []TeamMember

	for rows.Next() {
		var tm TeamMember
		err = rows.Scan(&tm.TeamID, &tm.UserID, &tm.Role, &tm.JoinedAt, &tm.Name, &tm.Email, &tm.ProfilePicture)
		if err != nil {
			return nil, err
		}
		members = append(members, tm)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return members, nil
}

func (m *TeamModel) ForCompetition(competitionID int64) ([]Team, error) {
	return m.list(teamSelect+` WHERE t.competition_id = $1 AND t.status = 'active' ORDER BY t.created_at DESC`, competitionID)
}

// ForUser lists the active teams a user belongs to.
func (m *TeamModel) ForUser(userID int64) ([]Team, error) {
	stmt := teamSelect + `
	JOIN team_members tm ON tm.team_id = t.id AND tm.user_id = $1 AND tm.status = 'active'
	WHERE t.status = 'active'
	ORDER BY t.created_at DESC`

	return m.list(stmt, userID)
}

// FindTeamFilter narrows the Find a Team lists. Zero values are ignored.
type FindTeamFilter struct {
	CompetitionID int64
	Category      string
	Role          string // also matched against skills
}

// Recruiting lists teams that are looking for members.
func (m *TeamModel) Recruiting(f FindTeamFilter) ([]Team, error) {
	stmt := teamSelect + `
	WHERE t.status = 'active' AND t.looking_for_members
	AND ($1 = 0 OR t.competition_id = $1)
	AND ($2 = '' OR c.category = $2)
	AND ($3 = '' OR t.looking_for_role ILIKE '%' || $3 || '%' OR t.required_skills ILIKE '%' || $3 || '%')
	ORDER BY t.created_at DESC`

	return m.list(stmt, f.CompetitionID, f.Category, f.Role)
}

func (m *TeamModel) list(stmt string, args ...any) ([]Team, error) {
	rows, err := m.DB.Query(stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var teams []Team

	for rows.Next() {
		t, err := scanTeam(rows)
		if err != nil {
			return nil, err
		}
		teams = append(teams, *t)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return teams, nil
}

// Insert creates a team with the given user as its captain, and returns the
// new team's id.
func (m *TeamModel) Insert(t *Team) (int64, error) {
	tx, err := m.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// The check runs inside the transaction so that two quick submissions
	// can't both get through.
	onTeam, err := onTeamIn(tx, t.CompetitionID, t.CaptainID)
	if err != nil {
		return 0, err
	}
	if onTeam {
		return 0, ErrAlreadyOnTeam
	}

	stmt := `INSERT INTO teams (competition_id, name, description, captain_id, max_members,
		required_skills, looking_for_members, looking_for_role)
	VALUES ($1, $2, NULLIF($3, ''), $4, $5, NULLIF($6, ''), $7, NULLIF($8, ''))
	RETURNING id`

	var id int64
	err = tx.QueryRow(stmt, t.CompetitionID, t.Name, t.Description, t.CaptainID, t.MaxMembers,
		t.RequiredSkills, t.LookingForMembers, t.LookingForRole).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return 0, ErrDuplicateName
		}
		return 0, err
	}

	stmt = `INSERT INTO team_members (team_id, user_id, role, status) VALUES ($1, $2, 'captain', 'active')`

	_, err = tx.Exec(stmt, id, t.CaptainID)
	if err != nil {
		return 0, err
	}

	err = tx.Commit()
	return id, err
}

func (m *TeamModel) Update(t *Team) error {
	stmt := `UPDATE teams SET name = $1, description = NULLIF($2, ''), max_members = $3,
		required_skills = NULLIF($4, ''), looking_for_members = $5, looking_for_role = NULLIF($6, ''),
		updated_at = now()
	WHERE id = $7`

	err := execOne(m.DB, stmt, t.Name, t.Description, t.MaxMembers,
		t.RequiredSkills, t.LookingForMembers, t.LookingForRole, t.ID)
	if isUniqueViolation(err) {
		return ErrDuplicateName
	}

	return err
}

// Delete disbands a team. Its registration goes too (ON DELETE CASCADE).
func (m *TeamModel) Delete(id int64) error {
	return execOne(m.DB, `DELETE FROM teams WHERE id = $1`, id)
}

// SetMemberStatus takes an active member off the team, with status "removed"
// (by the captain) or "left" (by themselves).
func (m *TeamModel) SetMemberStatus(teamID, userID int64, status string) error {
	stmt := `UPDATE team_members SET status = $1 WHERE team_id = $2 AND user_id = $3 AND status = 'active'`
	return execOne(m.DB, stmt, status, teamID, userID)
}

func (m *TeamModel) IsActiveMember(teamID, userID int64) (bool, error) {
	var exists bool

	stmt := `SELECT EXISTS(SELECT true FROM team_members WHERE team_id = $1 AND user_id = $2 AND status = 'active')`

	err := m.DB.QueryRow(stmt, teamID, userID).Scan(&exists)
	return exists, err
}

// IsOnTeamIn reports whether a user is an active member of any team in a
// competition.
func (m *TeamModel) IsOnTeamIn(competitionID, userID int64) (bool, error) {
	return onTeamIn(m.DB, competitionID, userID)
}

func onTeamIn(q querier, competitionID, userID int64) (bool, error) {
	var exists bool

	stmt := `SELECT EXISTS(
		SELECT true FROM team_members tm JOIN teams t ON t.id = tm.team_id
		WHERE t.competition_id = $1 AND tm.user_id = $2 AND tm.status = 'active')`

	err := q.QueryRow(stmt, competitionID, userID).Scan(&exists)
	return exists, err
}

func (m *TeamModel) TransferCaptain(teamID, oldCaptainID, newCaptainID int64) error {
	tx, err := m.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`UPDATE team_members SET role = 'member' WHERE team_id = $1 AND user_id = $2`, teamID, oldCaptainID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`UPDATE team_members SET role = 'captain' WHERE team_id = $1 AND user_id = $2`, teamID, newCaptainID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`UPDATE teams SET captain_id = $1, updated_at = now() WHERE id = $2`, newCaptainID, teamID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// ActiveMemberIDs lists the user ids of a team's active members.
func (m *TeamModel) ActiveMemberIDs(teamID int64) ([]int64, error) {
	return queryIDs(m.DB, `SELECT user_id FROM team_members WHERE team_id = $1 AND status = 'active'`, teamID)
}

// addMember adds a user to a team inside a transaction, after checking they
// aren't on another team in the competition and that the team has room.
// Someone who left or was removed earlier already has a row, which is
// switched back to active.
func addMember(tx *sql.Tx, t *Team, userID int64) error {
	onTeam, err := onTeamIn(tx, t.CompetitionID, userID)
	if err != nil {
		return err
	}
	if onTeam {
		return ErrAlreadyOnTeam
	}

	var count int64
	err = tx.QueryRow(`SELECT COUNT(*) FROM team_members WHERE team_id = $1 AND status = 'active'`, t.ID).Scan(&count)
	if err != nil {
		return err
	}
	if count >= t.Capacity() {
		return ErrTeamFull
	}

	stmt := `INSERT INTO team_members (team_id, user_id, role, status) VALUES ($1, $2, 'member', 'active')
	ON CONFLICT (team_id, user_id) DO UPDATE SET status = 'active', role = 'member', joined_at = now()`

	_, err = tx.Exec(stmt, t.ID, userID)
	return err
}

// createSoloTeam makes a one-person team named after the user, with them as
// captain, and returns its id. If the name is taken in this competition a
// number is added: "Priya Sharma (2)".
func createSoloTeam(tx *sql.Tx, competitionID int64, user *User) (int64, error) {
	name := user.Name

	for i := 2; ; i++ {
		var taken bool
		err := tx.QueryRow(`SELECT EXISTS(SELECT true FROM teams WHERE competition_id = $1 AND name = $2)`,
			competitionID, name).Scan(&taken)
		if err != nil {
			return 0, err
		}
		if !taken {
			break
		}
		name = fmt.Sprintf("%s (%d)", user.Name, i)
	}

	stmt := `INSERT INTO teams (competition_id, name, captain_id, max_members) VALUES ($1, $2, $3, 1) RETURNING id`

	var id int64
	err := tx.QueryRow(stmt, competitionID, name, user.ID).Scan(&id)
	if err != nil {
		return 0, err
	}

	_, err = tx.Exec(`INSERT INTO team_members (team_id, user_id, role, status) VALUES ($1, $2, 'captain', 'active')`, id, user.ID)
	return id, err
}

// queryIDs runs a query that returns a single column of ids.
func queryIDs(q querier, stmt string, args ...any) ([]int64, error) {
	rows, err := q.Query(stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64

	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return ids, nil
}
