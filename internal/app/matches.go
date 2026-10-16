package app

import (
	"strings"

	"arenasuite/internal/db"
	"arenasuite/internal/tournament"
)

const matchSelect = `
	SELECT m.*, ta.name AS team_a_name, tb.name AS team_b_name, tw.name AS winner_name, c.name AS competition_name
	FROM matches m
	JOIN competitions c ON c.id = m.competition_id
	LEFT JOIN teams ta ON ta.id = m.team_a_id
	LEFT JOIN teams tb ON tb.id = m.team_b_id
	LEFT JOIN teams tw ON tw.id = m.winner_id`

func (a *App) MatchesFor(competitionID int64) ([]Match, error) {
	return db.All[Match](a.DB, matchSelect+` WHERE m.competition_id = ? ORDER BY m.round_order ASC, m.id ASC`, competitionID)
}

// MyMatches lists matches involving any team the user is an active member of.
func (a *App) MyMatches(userID int64) ([]Match, error) {
	return db.All[Match](a.DB, matchSelect+`
		WHERE EXISTS (
			SELECT 1 FROM team_members tm
			WHERE tm.user_id = ? AND tm.status = 'active' AND tm.team_id IN (m.team_a_id, m.team_b_id))
		ORDER BY m.scheduled_at ASC, m.id ASC`, userID)
}

func (a *App) getMatch(id int64) (*Match, error) {
	m, err := db.One[Match](a.DB, matchSelect+` WHERE m.id = ?`, id)
	if err == nil && m == nil {
		return nil, fail(404, "Match not found")
	}
	return m, err
}

type MatchInput struct {
	Round              string
	TeamAID, TeamBID   int64
	ScheduledAt, Venue string
}

// CreateMatch adds a single match by hand (outside the fixture generator).
func (a *App) CreateMatch(user *User, competitionID int64, in MatchInput) error {
	if _, err := a.competitionToManage(user, competitionID); err != nil {
		return err
	}
	if strings.TrimSpace(in.Round) == "" {
		return fail(400, "Round is required")
	}
	scheduledAt, err := db.LocalTime(in.ScheduledAt)
	if err != nil {
		return fail(400, "Please enter a valid date and time")
	}
	_, err = a.DB.Exec(`
		INSERT INTO matches (competition_id, round, round_order, team_a_id, team_b_id, scheduled_at, venue)
		VALUES (?, ?, 0, ?, ?, ?, ?)`,
		competitionID, in.Round, db.NullIfZero(in.TeamAID), db.NullIfZero(in.TeamBID), scheduledAt, db.NullIfEmpty(in.Venue))
	if err == nil {
		a.toCompetition(competitionID, "matchStatusUpdated", map[string]int64{"competitionId": competitionID})
	}
	return err
}

// GenerateFixtures builds the whole schedule from the approved teams:
// a seeded bracket for knockout formats, round robin otherwise.
// Existing fixtures are only replaced if regenerate is true and no match has
// started, so a tournament in progress is never wiped by accident.
func (a *App) GenerateFixtures(user *User, competitionID int64, regenerate bool) error {
	c, err := a.competitionToManage(user, competitionID)
	if err != nil {
		return err
	}

	err = a.inTx(func(tx *db.Tx) error {
		existing, err := db.Count(tx, `SELECT COUNT(*) FROM matches WHERE competition_id = ?`, competitionID)
		if err != nil {
			return err
		}
		if existing > 0 && !regenerate {
			return fail(409, "Fixtures already exist for this competition")
		}
		if existing > 0 {
			started, err := db.Count(tx, `SELECT COUNT(*) FROM matches WHERE competition_id = ? AND status IN ('live', 'completed')`, competitionID)
			if err != nil {
				return err
			}
			if started > 0 {
				return fail(400, "Cannot regenerate fixtures once matches are live or completed")
			}
			if _, err := tx.Exec(`DELETE FROM matches WHERE competition_id = ?`, competitionID); err != nil {
				return err
			}
		}

		teams, err := db.All[Team](tx, `
			SELECT t.id FROM teams t
			JOIN registrations r ON r.team_id = t.id AND r.status = 'approved'
			WHERE t.competition_id = ? ORDER BY r.registered_at ASC`, competitionID)
		if err != nil {
			return err
		}
		if len(teams) < 2 {
			return fail(400, "At least 2 approved teams are required to generate fixtures")
		}
		teamIDs := make([]int64, len(teams))
		for i, t := range teams {
			teamIDs[i] = t.ID
		}

		var rounds [][]tournament.Fixture
		if c.IsKnockout() {
			rounds = tournament.Knockout(teamIDs)
		} else {
			rounds = tournament.RoundRobin(teamIDs)
		}

		// Insert every match, remembering the new ids by [round][index]...
		ids := make([][]int64, len(rounds))
		for r, round := range rounds {
			ids[r] = make([]int64, len(round))
			for i, f := range round {
				newID, err := db.Insert(tx, `
					INSERT INTO matches (competition_id, round, round_order, team_a_id, team_b_id, winner_id)
					VALUES (?, ?, ?, ?, ?, ?)`,
					competitionID, f.Round, f.RoundOrder, db.NullIfZero(f.TeamA), db.NullIfZero(f.TeamB), db.NullIfZero(f.Winner))
				if err != nil {
					return err
				}
				ids[r][i] = newID
			}
		}
		// ...then link each knockout match to the next-round slot it feeds.
		for r, round := range rounds {
			for i, f := range round {
				if !f.HasNext {
					continue
				}
				if _, err := tx.Exec(`UPDATE matches SET next_match_id = ?, next_match_slot = ? WHERE id = ?`,
					ids[f.NextRound][f.NextIndex], f.NextSlot, ids[r][i]); err != nil {
					return err
				}
			}
		}
		return recomputeStandings(tx, competitionID)
	})
	if err != nil {
		return err
	}
	a.toCompetition(competitionID, "standingsUpdated", map[string]int64{"competitionId": competitionID})
	a.toCompetition(competitionID, "matchStatusUpdated", map[string]int64{"competitionId": competitionID})
	return nil
}

// MatchUpdate is what the organizer submits from the live-scores screen.
// A nil score means "no score yet".
type MatchUpdate struct {
	ScoreA, ScoreB     *int64
	Status             string
	ScheduledAt, Venue string
}

// UpdateMatch saves scores and status. When a match is completed the winner
// is worked out from the scores (equal scores = draw, no winner), moved into
// its next bracket match, and the standings are recalculated.
func (a *App) UpdateMatch(user *User, matchID int64, in MatchUpdate) (competitionID int64, err error) {
	m, err := a.getMatch(matchID)
	if err != nil {
		return 0, err
	}
	if _, err := a.competitionToManage(user, m.CompetitionID); err != nil {
		return 0, err
	}
	if in.Status == "" {
		in.Status = m.Status
	}
	if err := mustBeOneOf(in.Status, MatchStatuses, "status"); err != nil {
		return 0, err
	}
	scheduledAt, err := db.LocalTime(in.ScheduledAt)
	if err != nil {
		return 0, fail(400, "Please enter a valid date and time")
	}

	winner := m.WinnerID
	if in.Status == "completed" && in.ScoreA != nil && in.ScoreB != nil {
		switch {
		case *in.ScoreA > *in.ScoreB:
			winner = m.TeamAID
		case *in.ScoreB > *in.ScoreA:
			winner = m.TeamBID
		default:
			winner = 0 // draw
		}
	}

	err = a.inTx(func(tx *db.Tx) error {
		_, err := tx.Exec(`
			UPDATE matches SET score_a = ?, score_b = ?, status = ?, winner_id = ?, scheduled_at = ?, venue = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ?`,
			in.ScoreA, in.ScoreB, in.Status, db.NullIfZero(winner), scheduledAt, db.NullIfEmpty(in.Venue), matchID)
		if err != nil {
			return err
		}
		if in.Status != "completed" {
			return nil
		}
		// The winner moves into their slot of the next bracket match.
		if winner != 0 && m.NextMatchID != 0 {
			column := "team_b_id"
			if m.NextMatchSlot == "A" {
				column = "team_a_id"
			}
			if _, err := tx.Exec(`UPDATE matches SET `+column+` = ? WHERE id = ?`, winner, m.NextMatchID); err != nil {
				return err
			}
		}
		return recomputeStandings(tx, m.CompetitionID)
	})
	if err != nil {
		return 0, err
	}

	updated, err := a.getMatch(matchID)
	if err != nil {
		return 0, err
	}
	if in.Status == "completed" {
		a.toCompetition(m.CompetitionID, "standingsUpdated", map[string]int64{"competitionId": m.CompetitionID})
	}
	if in.Status != m.Status {
		a.toCompetition(m.CompetitionID, "matchStatusUpdated", updated)
	}
	if !sameScore(in.ScoreA, m.ScoreA) || !sameScore(in.ScoreB, m.ScoreB) {
		a.toCompetition(m.CompetitionID, "scoreUpdated", updated)
	}

	// Tell both teams' members.
	title := orTBD(updated.TeamAName) + " vs " + orTBD(updated.TeamBName)
	for _, teamID := range []int64{m.TeamAID, m.TeamBID} {
		if teamID == 0 {
			continue
		}
		members, _ := activeMemberIDs(a.DB, teamID)
		for _, id := range members {
			a.Notify(id, "matchUpdated", title+" was updated", matchID)
		}
	}
	return m.CompetitionID, nil
}

func (a *App) DeleteMatch(user *User, matchID int64) (competitionID int64, err error) {
	m, err := a.getMatch(matchID)
	if err != nil {
		return 0, err
	}
	if _, err := a.competitionToManage(user, m.CompetitionID); err != nil {
		return 0, err
	}
	_, err = a.DB.Exec(`DELETE FROM matches WHERE id = ?`, matchID)
	return m.CompetitionID, err
}

func sameScore(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func orTBD(name string) string {
	if name == "" {
		return "TBD"
	}
	return name
}
