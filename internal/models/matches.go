package models

import (
	"database/sql"
	"errors"
	"time"

	"arenasuite/internal/tournament"
)

type Match struct {
	ID            int64
	CompetitionID int64
	Round         string
	RoundOrder    int64
	TeamAID       int64  // 0 = TBD
	TeamBID       int64  // 0 = TBD
	ScoreA        *int64 // nil = no score yet (different from 0)
	ScoreB        *int64
	WinnerID      int64      // 0 = none (not played, or a draw)
	ScheduledAt   *time.Time // nil = not scheduled
	Venue         string
	Status        string
	NextMatchID   int64
	NextMatchSlot string

	TeamAName       string
	TeamBName       string
	WinnerName      string
	CompetitionName string
}

type MatchModel struct {
	DB *sql.DB
}

// Nullable team ids are read as 0 and nullable team names as "". The score
// and schedule columns are scanned into pointers, which database/sql sets to
// nil for a NULL.
const matchSelect = `SELECT m.id, m.competition_id, m.round, m.round_order,
	COALESCE(m.team_a_id, 0), COALESCE(m.team_b_id, 0), m.score_a, m.score_b, COALESCE(m.winner_id, 0),
	m.scheduled_at, COALESCE(m.venue, ''), m.status, COALESCE(m.next_match_id, 0), COALESCE(m.next_match_slot, ''),
	COALESCE(ta.name, ''), COALESCE(tb.name, ''), COALESCE(tw.name, ''), c.name
	FROM matches m
	JOIN competitions c ON c.id = m.competition_id
	LEFT JOIN teams ta ON ta.id = m.team_a_id
	LEFT JOIN teams tb ON tb.id = m.team_b_id
	LEFT JOIN teams tw ON tw.id = m.winner_id`

func scanMatch(row scanner) (*Match, error) {
	m := &Match{}
	err := row.Scan(&m.ID, &m.CompetitionID, &m.Round, &m.RoundOrder,
		&m.TeamAID, &m.TeamBID, &m.ScoreA, &m.ScoreB, &m.WinnerID,
		&m.ScheduledAt, &m.Venue, &m.Status, &m.NextMatchID, &m.NextMatchSlot,
		&m.TeamAName, &m.TeamBName, &m.WinnerName, &m.CompetitionName)
	return m, err
}

func (m *MatchModel) Get(id int64) (*Match, error) {
	match, err := scanMatch(m.DB.QueryRow(matchSelect+` WHERE m.id = $1`, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		}
		return nil, err
	}

	return match, nil
}

func (m *MatchModel) ForCompetition(competitionID int64) ([]Match, error) {
	return m.list(matchSelect+` WHERE m.competition_id = $1 ORDER BY m.round_order ASC, m.id ASC`, competitionID)
}

// ForUser lists matches involving any team the user is an active member of.
func (m *MatchModel) ForUser(userID int64) ([]Match, error) {
	stmt := matchSelect + `
	WHERE EXISTS (
		SELECT true FROM team_members tm
		WHERE tm.user_id = $1 AND tm.status = 'active' AND tm.team_id IN (m.team_a_id, m.team_b_id))
	ORDER BY m.scheduled_at ASC, m.id ASC`

	return m.list(stmt, userID)
}

func (m *MatchModel) list(stmt string, args ...any) ([]Match, error) {
	rows, err := m.DB.Query(stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []Match

	for rows.Next() {
		match, err := scanMatch(rows)
		if err != nil {
			return nil, err
		}
		matches = append(matches, *match)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return matches, nil
}

// Insert adds a single match by hand (outside the fixture generator).
func (m *MatchModel) Insert(competitionID int64, round string, teamAID, teamBID int64, scheduledAt *time.Time, venue string) error {
	stmt := `INSERT INTO matches (competition_id, round, round_order, team_a_id, team_b_id, scheduled_at, venue)
	VALUES ($1, $2, 0, NULLIF($3, 0), NULLIF($4, 0), $5, NULLIF($6, ''))`

	_, err := m.DB.Exec(stmt, competitionID, round, teamAID, teamBID, scheduledAt, venue)
	return err
}

// Generate builds the whole schedule from the approved teams: a seeded
// bracket for knockout formats, round robin otherwise. Existing fixtures are
// only replaced if regenerate is true and no match has started, so a
// tournament in progress is never wiped by accident.
func (m *MatchModel) Generate(competitionID int64, knockout, regenerate bool) error {
	tx, err := m.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var existing, started int64
	stmt := `SELECT COUNT(*), COUNT(*) FILTER (WHERE status IN ('live', 'completed')) FROM matches WHERE competition_id = $1`

	err = tx.QueryRow(stmt, competitionID).Scan(&existing, &started)
	if err != nil {
		return err
	}

	switch {
	case existing > 0 && !regenerate:
		return ErrFixturesExist
	case started > 0:
		return ErrMatchesStarted
	}

	_, err = tx.Exec(`DELETE FROM matches WHERE competition_id = $1`, competitionID)
	if err != nil {
		return err
	}

	stmt = `SELECT t.id FROM teams t
	JOIN registrations r ON r.team_id = t.id AND r.status = 'approved'
	WHERE t.competition_id = $1
	ORDER BY r.registered_at ASC`

	teamIDs, err := queryIDs(tx, stmt, competitionID)
	if err != nil {
		return err
	}
	if len(teamIDs) < 2 {
		return ErrNotEnoughTeams
	}

	var rounds [][]tournament.Fixture
	if knockout {
		rounds = tournament.Knockout(teamIDs)
	} else {
		rounds = tournament.RoundRobin(teamIDs)
	}

	// Insert every match, remembering the new ids by [round][index]...
	stmt = `INSERT INTO matches (competition_id, round, round_order, team_a_id, team_b_id, winner_id)
	VALUES ($1, $2, $3, NULLIF($4, 0), NULLIF($5, 0), NULLIF($6, 0))
	RETURNING id`

	ids := make([][]int64, len(rounds))
	for r, round := range rounds {
		ids[r] = make([]int64, len(round))
		for i, f := range round {
			err = tx.QueryRow(stmt, competitionID, f.Round, f.RoundOrder, f.TeamA, f.TeamB, f.Winner).Scan(&ids[r][i])
			if err != nil {
				return err
			}
		}
	}

	// ...then link each knockout match to the next-round slot it feeds.
	stmt = `UPDATE matches SET next_match_id = $1, next_match_slot = $2 WHERE id = $3`

	for r, round := range rounds {
		for i, f := range round {
			if !f.HasNext {
				continue
			}
			_, err = tx.Exec(stmt, ids[f.NextRound][f.NextIndex], f.NextSlot, ids[r][i])
			if err != nil {
				return err
			}
		}
	}

	err = recomputeStandings(tx, competitionID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// Update saves the scores, status and schedule of a match. When the match is
// completed, the winner moves into their slot of the next bracket match and
// the standings are recalculated, all in one transaction.
func (m *MatchModel) Update(match *Match) error {
	tx, err := m.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt := `UPDATE matches SET score_a = $1, score_b = $2, status = $3, winner_id = NULLIF($4, 0),
		scheduled_at = $5, venue = NULLIF($6, ''), updated_at = now()
	WHERE id = $7`

	_, err = tx.Exec(stmt, match.ScoreA, match.ScoreB, match.Status, match.WinnerID,
		match.ScheduledAt, match.Venue, match.ID)
	if err != nil {
		return err
	}

	if match.Status == "completed" {
		if match.WinnerID != 0 && match.NextMatchID != 0 {
			// A column name can't be a placeholder, so pick between two
			// fixed statements instead.
			stmt = `UPDATE matches SET team_b_id = $1 WHERE id = $2`
			if match.NextMatchSlot == "A" {
				stmt = `UPDATE matches SET team_a_id = $1 WHERE id = $2`
			}

			_, err = tx.Exec(stmt, match.WinnerID, match.NextMatchID)
			if err != nil {
				return err
			}
		}

		err = recomputeStandings(tx, match.CompetitionID)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (m *MatchModel) Delete(id int64) error {
	return execOne(m.DB, `DELETE FROM matches WHERE id = $1`, id)
}
