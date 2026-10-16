package app

import (
	"arenasuite/internal/db"
	"arenasuite/internal/tournament"
)

func (a *App) StandingsFor(competitionID int64) ([]Standing, error) {
	return db.All[Standing](a.DB, `
		SELECT s.*, t.name AS team_name FROM standings s JOIN teams t ON t.id = s.team_id
		WHERE s.competition_id = ? ORDER BY s.rank ASC`, competitionID)
}

// recomputeStandings rebuilds the table for Sports/Esports from scratch by
// replaying every completed match (see tournament.Standings for the rules).
// Standings are never edited by hand for these categories.
func recomputeStandings(q db.Querier, competitionID int64) error {
	teams, err := db.All[Team](q, `
		SELECT DISTINCT t.id FROM teams t
		JOIN registrations r ON r.team_id = t.id AND r.status = 'approved'
		WHERE t.competition_id = ?`, competitionID)
	if err != nil {
		return err
	}
	teamIDs := make([]int64, len(teams))
	for i, t := range teams {
		teamIDs[i] = t.ID
	}

	matches, err := db.All[Match](q, `
		SELECT * FROM matches
		WHERE competition_id = ? AND status = 'completed' AND team_a_id IS NOT NULL AND team_b_id IS NOT NULL`, competitionID)
	if err != nil {
		return err
	}
	results := make([]tournament.Result, len(matches))
	for i, m := range matches {
		results[i] = tournament.Result{TeamA: m.TeamAID, TeamB: m.TeamBID, Winner: m.WinnerID}
		if m.ScoreA != nil {
			results[i].ScoreA = *m.ScoreA
		}
		if m.ScoreB != nil {
			results[i].ScoreB = *m.ScoreB
		}
	}

	for _, row := range tournament.Standings(teamIDs, results) {
		_, err := q.Exec(`
			INSERT INTO standings (competition_id, team_id, played, won, lost, draw, points, score_difference, rank, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
			ON CONFLICT (competition_id, team_id) DO UPDATE SET
				played = excluded.played, won = excluded.won, lost = excluded.lost, draw = excluded.draw,
				points = excluded.points, score_difference = excluded.score_difference, rank = excluded.rank,
				updated_at = CURRENT_TIMESTAMP`,
			competitionID, row.TeamID, row.Played, row.Won, row.Lost, row.Draw, row.Points, row.ScoreDifference, row.Rank)
		if err != nil {
			return err
		}
	}
	return nil
}

// RecomputeStandings is the organizer's "recalculate" button.
func (a *App) RecomputeStandings(user *User, competitionID int64) error {
	if _, err := a.competitionToManage(user, competitionID); err != nil {
		return err
	}
	err := a.inTx(func(tx *db.Tx) error { return recomputeStandings(tx, competitionID) })
	if err == nil {
		a.toCompetition(competitionID, "standingsUpdated", map[string]int64{"competitionId": competitionID})
	}
	return err
}

// SetAcademicScore is for Academic competitions, which have no matches to
// derive a table from: the organizer enters a team's score directly, then
// every team is re-ranked by points.
func (a *App) SetAcademicScore(user *User, competitionID, teamID, points int64) error {
	c, err := a.competitionToManage(user, competitionID)
	if err != nil {
		return err
	}
	if c.Category != "ACADEMIC" {
		return fail(400, "Direct score entry is only for Academic competitions; other categories use match results")
	}
	if teamID == 0 {
		return fail(400, "Pick a team")
	}
	t, err := a.getTeam(a.DB, teamID)
	if err != nil {
		return err
	}
	if t.CompetitionID != competitionID {
		return fail(404, "Team not found for this competition")
	}

	err = a.inTx(func(tx *db.Tx) error {
		_, err := tx.Exec(`
			INSERT INTO standings (competition_id, team_id, points, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)
			ON CONFLICT (competition_id, team_id) DO UPDATE SET points = excluded.points, updated_at = CURRENT_TIMESTAMP`,
			competitionID, teamID, points)
		if err != nil {
			return err
		}
		// Re-rank: highest points first. ROW_NUMBER() numbers rows 1, 2, 3...
		_, err = tx.Exec(`
			UPDATE standings SET rank = (
				SELECT rn FROM (
					SELECT id, ROW_NUMBER() OVER (ORDER BY points DESC, id ASC) AS rn
					FROM standings WHERE competition_id = ?
				) ranked WHERE ranked.id = standings.id
			) WHERE competition_id = ?`, competitionID, competitionID)
		return err
	})
	if err == nil {
		a.toCompetition(competitionID, "standingsUpdated", map[string]int64{"competitionId": competitionID})
	}
	return err
}
