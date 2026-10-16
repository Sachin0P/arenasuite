package app

import (
	"strings"

	"arenasuite/internal/db"
)

const teamSelect = `
	SELECT t.*, u.name AS captain_name, c.name AS competition_name, c.category,
		c.max_participants AS competition_max_team_size,
		(SELECT COUNT(*) FROM team_members m WHERE m.team_id = t.id AND m.status = 'active') AS member_count
	FROM teams t
	JOIN users u ON u.id = t.captain_id
	JOIN competitions c ON c.id = t.competition_id`

func (a *App) getTeam(q db.Querier, id int64) (*Team, error) {
	t, err := db.One[Team](q, teamSelect+` WHERE t.id = ?`, id)
	if err == nil && t == nil {
		return nil, fail(404, "Team not found")
	}
	return t, err
}

// TeamByID loads a team together with its active members.
func (a *App) TeamByID(id int64) (*Team, error) {
	t, err := a.getTeam(a.DB, id)
	if err != nil {
		return nil, err
	}
	t.Members, err = db.All[TeamMember](a.DB, `
		SELECT tm.*, u.name, u.email, u.profile_picture
		FROM team_members tm JOIN users u ON u.id = tm.user_id
		WHERE tm.team_id = ? AND tm.status = 'active'
		ORDER BY tm.role ASC, tm.joined_at ASC`, id) // "captain" sorts before "member"
	return t, err
}

func (a *App) TeamsForCompetition(competitionID int64) ([]Team, error) {
	return db.All[Team](a.DB, teamSelect+` WHERE t.competition_id = ? AND t.status = 'active' ORDER BY t.created_at DESC`, competitionID)
}

// MyTeams lists the active teams a user belongs to.
func (a *App) MyTeams(userID int64) ([]Team, error) {
	return db.All[Team](a.DB, teamSelect+`
		JOIN team_members tm ON tm.team_id = t.id AND tm.user_id = ? AND tm.status = 'active'
		WHERE t.status = 'active' ORDER BY t.created_at DESC`, userID)
}

// mustNotBeOnTeam enforces "at most one team per user per competition".
// It returns a 409 error with msg if the user is already on a team there.
func mustNotBeOnTeam(q db.Querier, competitionID, userID int64, msg string) error {
	n, err := db.Count(q, `
		SELECT COUNT(*) FROM team_members tm JOIN teams t ON t.id = tm.team_id
		WHERE t.competition_id = ? AND tm.user_id = ? AND tm.status = 'active'`, competitionID, userID)
	if err != nil {
		return err
	}
	if n > 0 {
		return fail(409, "%s", msg)
	}
	return nil
}

func mustBeCaptain(user *User, t *Team) error {
	if user.IsAdmin() || t.CaptainID == user.ID {
		return nil
	}
	return fail(403, "Only the team captain can do this")
}

type TeamInput struct {
	Name, Description, RequiredSkills, LookingForRole string
	MaxMembers                                        int64
	LookingForMembers                                 bool
}

// CreateTeam makes the user captain of a new team in a competition.
func (a *App) CreateTeam(user *User, competitionID int64, in TeamInput) (int64, error) {
	if strings.TrimSpace(in.Name) == "" {
		return 0, fail(400, "Team name is required")
	}
	c, err := a.getCompetition(a.DB, competitionID)
	if err != nil {
		return 0, err
	}
	if c.Status != "published" && c.Status != "ongoing" {
		return 0, fail(400, "Teams can only be created for published competitions")
	}
	// Default team size: the competition's limit if it has one, otherwise 4.
	if in.MaxMembers <= 0 {
		in.MaxMembers = c.MaxTeamSize
	}
	if in.MaxMembers <= 0 {
		in.MaxMembers = 4
	}
	if err := checkTeamSize(in.MaxMembers, c.MaxTeamSize, 1); err != nil {
		return 0, err
	}

	var id int64
	err = a.inTx(func(tx *db.Tx) error {
		if err := mustNotBeOnTeam(tx, competitionID, user.ID, "You are already on a team for this competition"); err != nil {
			return err
		}
		newID, err := db.Insert(tx, `
			INSERT INTO teams (competition_id, name, description, captain_id, max_members, required_skills, looking_for_members, looking_for_role)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			competitionID, in.Name, db.NullIfEmpty(in.Description), user.ID, in.MaxMembers,
			db.NullIfEmpty(in.RequiredSkills), in.LookingForMembers, db.NullIfEmpty(in.LookingForRole))
		if db.IsUniqueViolation(err) {
			return fail(409, "A team with this name already exists in this competition")
		} else if err != nil {
			return err
		}
		id = newID
		_, err = tx.Exec(`INSERT INTO team_members (team_id, user_id, role, status) VALUES (?, ?, 'captain', 'active')`, id, user.ID)
		return err
	})
	return id, err
}

func (a *App) UpdateTeam(user *User, teamID int64, in TeamInput) error {
	t, err := a.getTeam(a.DB, teamID)
	if err != nil {
		return err
	}
	if err := mustBeCaptain(user, t); err != nil {
		return err
	}
	if strings.TrimSpace(in.Name) == "" {
		return fail(400, "Team name is required")
	}
	if in.MaxMembers <= 0 {
		in.MaxMembers = t.MaxMembers
	}
	if err := checkTeamSize(in.MaxMembers, t.MaxTeamSize, t.MemberCount); err != nil {
		return err
	}
	_, err = a.DB.Exec(`
		UPDATE teams SET name = ?, description = ?, max_members = ?, required_skills = ?,
			looking_for_members = ?, looking_for_role = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`,
		in.Name, db.NullIfEmpty(in.Description), in.MaxMembers, db.NullIfEmpty(in.RequiredSkills),
		in.LookingForMembers, db.NullIfEmpty(in.LookingForRole), teamID)
	if db.IsUniqueViolation(err) {
		return fail(409, "A team with this name already exists in this competition")
	}
	return err
}

// checkTeamSize makes sure a team's size fits the competition's limit
// (0 = no limit) and isn't smaller than the members it already has.
func checkTeamSize(size, limit, currentMembers int64) error {
	if limit > 0 && size > limit {
		return fail(400, "Teams in this competition can have at most %d members", limit)
	}
	if size < currentMembers {
		return fail(400, "This team already has %d members", currentMembers)
	}
	return nil
}

// DeleteTeam disbands a team. Its registration goes too (ON DELETE CASCADE).
func (a *App) DeleteTeam(user *User, teamID int64) (competitionID int64, err error) {
	t, err := a.getTeam(a.DB, teamID)
	if err != nil {
		return 0, err
	}
	if err := mustBeCaptain(user, t); err != nil {
		return 0, err
	}
	_, err = a.DB.Exec(`DELETE FROM teams WHERE id = ?`, teamID)
	return t.CompetitionID, err
}

func (a *App) RemoveMember(user *User, teamID, memberID int64) error {
	t, err := a.getTeam(a.DB, teamID)
	if err != nil {
		return err
	}
	if err := mustBeCaptain(user, t); err != nil {
		return err
	}
	if memberID == t.CaptainID {
		return fail(400, "Transfer captaincy before removing the captain")
	}
	res, err := a.DB.Exec(`UPDATE team_members SET status = 'removed' WHERE team_id = ? AND user_id = ? AND status = 'active'`, teamID, memberID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fail(404, "Member not found on this team")
	}
	return nil
}

func (a *App) LeaveTeam(user *User, teamID int64) error {
	t, err := a.getTeam(a.DB, teamID)
	if err != nil {
		return err
	}
	if t.CaptainID == user.ID {
		return fail(400, "Transfer captaincy before leaving the team")
	}
	res, err := a.DB.Exec(`UPDATE team_members SET status = 'left' WHERE team_id = ? AND user_id = ? AND status = 'active'`, teamID, user.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fail(404, "You are not an active member of this team")
	}
	return nil
}

func (a *App) TransferCaptain(user *User, teamID, newCaptainID int64) error {
	t, err := a.getTeam(a.DB, teamID)
	if err != nil {
		return err
	}
	if err := mustBeCaptain(user, t); err != nil {
		return err
	}
	n, err := db.Count(a.DB, `SELECT COUNT(*) FROM team_members WHERE team_id = ? AND user_id = ? AND status = 'active'`, teamID, newCaptainID)
	if err != nil {
		return err
	}
	if n == 0 {
		return fail(400, "Target user is not an active member of this team")
	}
	return a.inTx(func(tx *db.Tx) error {
		if _, err := tx.Exec(`UPDATE team_members SET role = 'member' WHERE team_id = ? AND user_id = ?`, teamID, t.CaptainID); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE team_members SET role = 'captain' WHERE team_id = ? AND user_id = ?`, teamID, newCaptainID); err != nil {
			return err
		}
		_, err := tx.Exec(`UPDATE teams SET captain_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, newCaptainID, teamID)
		return err
	})
}

// addMember adds (or re-activates) a user on a team. Someone who left or was
// removed earlier already has a row, which we switch back to active.
func addMember(q db.Querier, teamID, userID int64) error {
	_, err := q.Exec(`
		INSERT INTO team_members (team_id, user_id, role, status) VALUES (?, ?, 'member', 'active')
		ON CONFLICT (team_id, user_id) DO UPDATE SET status = 'active', role = 'member', joined_at = CURRENT_TIMESTAMP`,
		teamID, userID)
	return err
}

// ---------- Join requests ----------

func (a *App) RequestToJoin(user *User, teamID int64, message string) error {
	t, err := a.getTeam(a.DB, teamID)
	if err != nil {
		return err
	}
	if !t.LookingForMembers {
		return fail(400, "This team is not looking for members")
	}
	if t.IsFull() {
		return fail(400, "This team is full")
	}
	if err := mustNotBeOnTeam(a.DB, t.CompetitionID, user.ID, "You are already on a team for this competition"); err != nil {
		return err
	}
	pending, err := db.Count(a.DB, `SELECT COUNT(*) FROM team_join_requests WHERE team_id = ? AND user_id = ? AND status = 'pending'`, teamID, user.ID)
	if err != nil {
		return err
	}
	if pending > 0 {
		return fail(409, "You already have a pending request for this team")
	}

	requestID, err := db.Insert(a.DB, `INSERT INTO team_join_requests (team_id, user_id, message) VALUES (?, ?, ?)`, teamID, user.ID, db.NullIfEmpty(message))
	if err != nil {
		return err
	}
	a.Notify(t.CaptainID, "teamJoinRequest", user.Name+" requested to join "+t.Name, teamID)
	a.toCompetition(t.CompetitionID, "teamJoinRequest", map[string]int64{"teamId": teamID, "requestId": requestID})
	return nil
}

// PendingJoinRequests is what the captain sees on the team page.
func (a *App) PendingJoinRequests(user *User, teamID int64) ([]JoinRequest, error) {
	t, err := a.getTeam(a.DB, teamID)
	if err != nil {
		return nil, err
	}
	if err := mustBeCaptain(user, t); err != nil {
		return nil, err
	}
	return db.All[JoinRequest](a.DB, `
		SELECT jr.*, u.name, u.email FROM team_join_requests jr JOIN users u ON u.id = jr.user_id
		WHERE jr.team_id = ? AND jr.status = 'pending' ORDER BY jr.created_at DESC`, teamID)
}

// MyPendingJoinRequests lists the teams a user has asked to join.
func (a *App) MyPendingJoinRequests(userID int64) (map[int64]int64, error) {
	rows, err := db.All[JoinRequest](a.DB, `SELECT * FROM team_join_requests WHERE user_id = ? AND status = 'pending'`, userID)
	byTeam := map[int64]int64{} // team id -> request id
	for _, r := range rows {
		byTeam[r.TeamID] = r.ID
	}
	return byTeam, err
}

func (a *App) RespondJoinRequest(user *User, requestID int64, accept bool) error {
	req, err := db.One[JoinRequest](a.DB, `SELECT * FROM team_join_requests WHERE id = ?`, requestID)
	if err != nil {
		return err
	}
	if req == nil {
		return fail(404, "Request not found")
	}
	t, err := a.getTeam(a.DB, req.TeamID)
	if err != nil {
		return err
	}
	if err := mustBeCaptain(user, t); err != nil {
		return err
	}
	if req.Status != "pending" {
		return fail(400, "This request has already been resolved")
	}

	status, verb := "rejected", "rejected"
	err = a.inTx(func(tx *db.Tx) error {
		if accept {
			status, verb = "accepted", "accepted"
			if t.IsFull() {
				return fail(400, "This team is full")
			}
			if err := mustNotBeOnTeam(tx, t.CompetitionID, req.UserID, "That user is already on a team for this competition"); err != nil {
				return err
			}
			if err := addMember(tx, t.ID, req.UserID); err != nil {
				return err
			}
		}
		_, err := tx.Exec(`UPDATE team_join_requests SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, status, requestID)
		return err
	})
	if err != nil {
		return err
	}
	a.Notify(req.UserID, "teamJoinRequest", "Your request to join "+t.Name+" was "+verb, t.ID)
	return nil
}

func (a *App) CancelJoinRequest(user *User, requestID int64) error {
	req, err := db.One[JoinRequest](a.DB, `SELECT * FROM team_join_requests WHERE id = ?`, requestID)
	if err != nil {
		return err
	}
	if req == nil {
		return fail(404, "Request not found")
	}
	if req.UserID != user.ID {
		return fail(403, "You can only cancel your own request")
	}
	if req.Status != "pending" {
		return fail(400, "This request has already been resolved")
	}
	_, err = a.DB.Exec(`UPDATE team_join_requests SET status = 'cancelled', updated_at = CURRENT_TIMESTAMP WHERE id = ?`, requestID)
	return err
}

// ---------- Invitations ----------

func (a *App) InviteMember(user *User, teamID, inviteeID int64) error {
	t, err := a.getTeam(a.DB, teamID)
	if err != nil {
		return err
	}
	if err := mustBeCaptain(user, t); err != nil {
		return err
	}
	if t.IsFull() {
		return fail(400, "This team is full")
	}
	if err := mustNotBeOnTeam(a.DB, t.CompetitionID, inviteeID, "That user is already on a team for this competition"); err != nil {
		return err
	}
	pending, err := db.Count(a.DB, `SELECT COUNT(*) FROM team_invitations WHERE team_id = ? AND user_id = ? AND status = 'pending'`, teamID, inviteeID)
	if err != nil {
		return err
	}
	if pending > 0 {
		return fail(409, "An invitation is already pending for this user")
	}
	if _, err := a.DB.Exec(`INSERT INTO team_invitations (team_id, user_id, invited_by) VALUES (?, ?, ?)`, teamID, inviteeID, user.ID); err != nil {
		return err
	}
	a.Notify(inviteeID, "teamInvitation", "You have been invited to join "+t.Name, teamID)
	return nil
}

func (a *App) MyInvitations(userID int64) ([]Invitation, error) {
	return db.All[Invitation](a.DB, `
		SELECT ti.*, t.name AS team_name, t.competition_id, u.name AS inviter_name
		FROM team_invitations ti
		JOIN teams t ON t.id = ti.team_id
		JOIN users u ON u.id = ti.invited_by
		WHERE ti.user_id = ? AND ti.status = 'pending'
		ORDER BY ti.created_at DESC`, userID)
}

func (a *App) RespondInvitation(user *User, invitationID int64, accept bool) error {
	inv, err := db.One[Invitation](a.DB, `SELECT * FROM team_invitations WHERE id = ?`, invitationID)
	if err != nil {
		return err
	}
	if inv == nil {
		return fail(404, "Invitation not found")
	}
	if inv.UserID != user.ID {
		return fail(403, "This invitation is not yours")
	}
	if inv.Status != "pending" {
		return fail(400, "This invitation has already been resolved")
	}
	t, err := a.getTeam(a.DB, inv.TeamID)
	if err != nil {
		return err
	}

	status, verb := "rejected", "declined"
	err = a.inTx(func(tx *db.Tx) error {
		if accept {
			status, verb = "accepted", "accepted"
			if t.IsFull() {
				return fail(400, "This team is full")
			}
			if err := mustNotBeOnTeam(tx, t.CompetitionID, user.ID, "You are already on a team for this competition"); err != nil {
				return err
			}
			if err := addMember(tx, t.ID, user.ID); err != nil {
				return err
			}
		}
		_, err := tx.Exec(`UPDATE team_invitations SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, status, invitationID)
		return err
	})
	if err != nil {
		return err
	}
	a.Notify(t.CaptainID, "teamInvitationResponse", user.Name+" "+verb+" your invitation to "+t.Name, t.ID)
	return nil
}

func (a *App) CancelInvitation(user *User, invitationID int64) error {
	inv, err := db.One[Invitation](a.DB, `SELECT * FROM team_invitations WHERE id = ?`, invitationID)
	if err != nil {
		return err
	}
	if inv == nil {
		return fail(404, "Invitation not found")
	}
	t, err := a.getTeam(a.DB, inv.TeamID)
	if err != nil {
		return err
	}
	if err := mustBeCaptain(user, t); err != nil {
		return err
	}
	_, err = a.DB.Exec(`UPDATE team_invitations SET status = 'cancelled', updated_at = CURRENT_TIMESTAMP WHERE id = ?`, invitationID)
	return err
}
