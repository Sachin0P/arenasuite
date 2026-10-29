package models

import (
	"database/sql"

	"arenasuite/internal/tournament"
)

type Standing struct {
	ID              int64
	CompetitionID   int64
	TeamID          int64
	Played          int64
	Won             int64
	Lost            int64
	Draw            int64
	Points          int64
	ScoreDifference int64
	Rank            int64
	TeamName        string
}

type StandingModel struct {
	DB *sql.DB
}

func (m *StandingModel) ForCompetition(competitionID int64) ([]Standing, error) {
	stmt := `SELECT s.id, s.competition_id, s.team_id, s.played, s.won, s.lost, s.draw, s.points,
		s.score_difference, COALESCE(s.rank, 0), t.name
	FROM standings s
	JOIN teams t ON t.id = s.team_id
	WHERE s.competition_id = $1
	ORDER BY s.rank ASC`

	rows, err := m.DB.Query(stmt, competitionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var standings []Standing

	for rows.Next() {
		var s Standing
		err = rows.Scan(&s.ID, &s.CompetitionID, &s.TeamID, &s.Played, &s.Won, &s.Lost, &s.Draw, &s.Points,
			&s.ScoreDifference, &s.Rank, &s.TeamName)
		if err != nil {
			return nil, err
		}
		standings = append(standings, s)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return standings, nil
}

// Recompute is the organizer's "recalculate" button.
func (m *StandingModel) Recompute(competitionID int64) error {
	tx, err := m.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	err = recomputeStandings(tx, competitionID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// SetScore is for Academic competitions, which have no matches to derive a
// table from: the organizer enters a team's score directly, then every team
// is re-ranked by points.
func (m *StandingModel) SetScore(competitionID, teamID, points int64) error {
	tx, err := m.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt := `INSERT INTO standings (competition_id, team_id, points, updated_at) VALUES ($1, $2, $3, now())
	ON CONFLICT (competition_id, team_id) DO UPDATE SET points = excluded.points, updated_at = now()`

	_, err = tx.Exec(stmt, competitionID, teamID, points)
	if err != nil {
		return err
	}

	// Re-rank: highest points first. ROW_NUMBER() numbers the rows 1, 2, 3...
	stmt = `UPDATE standings SET rank = ranked.rn
	FROM (
		SELECT id, ROW_NUMBER() OVER (ORDER BY points DESC, id ASC) AS rn
		FROM standings WHERE competition_id = $1
	) ranked
	WHERE ranked.id = standings.id`

	_, err = tx.Exec(stmt, competitionID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// recomputeStandings rebuilds the table for Sports and Esports from scratch
// by replaying every completed match (see tournament.Standings for the
// rules). It runs inside the caller's transaction.
func recomputeStandings(tx *sql.Tx, competitionID int64) error {
	stmt := `SELECT DISTINCT t.id FROM teams t
	JOIN registrations r ON r.team_id = t.id AND r.status = 'approved'
	WHERE t.competition_id = $1`

	teamIDs, err := queryIDs(tx, stmt, competitionID)
	if err != nil {
		return err
	}

	stmt = `SELECT team_a_id, team_b_id, COALESCE(score_a, 0), COALESCE(score_b, 0), COALESCE(winner_id, 0)
	FROM matches
	WHERE competition_id = $1 AND status = 'completed' AND team_a_id IS NOT NULL AND team_b_id IS NOT NULL`

	rows, err := tx.Query(stmt, competitionID)
	if err != nil {
		return err
	}
	defer rows.Close()

	var results []tournament.Result

	for rows.Next() {
		var r tournament.Result
		err = rows.Scan(&r.TeamA, &r.TeamB, &r.ScoreA, &r.ScoreB, &r.Winner)
		if err != nil {
			return err
		}
		results = append(results, r)
	}

	if err = rows.Err(); err != nil {
		return err
	}

	stmt = `INSERT INTO standings (competition_id, team_id, played, won, lost, draw, points, score_difference, rank, updated_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
	ON CONFLICT (competition_id, team_id) DO UPDATE SET
		played = excluded.played, won = excluded.won, lost = excluded.lost, draw = excluded.draw,
		points = excluded.points, score_difference = excluded.score_difference, rank = excluded.rank,
		updated_at = now()`

	for _, row := range tournament.Standings(teamIDs, results) {
		_, err = tx.Exec(stmt, competitionID, row.TeamID, row.Played, row.Won, row.Lost, row.Draw,
			row.Points, row.ScoreDifference, row.Rank)
		if err != nil {
			return err
		}
	}

	return nil
}
