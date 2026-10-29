package models

import (
	"database/sql"
)

// PlatformStats are the totals on the admin overview page.
type PlatformStats struct {
	TotalUsers         int64
	TotalOrganizations int64
	TotalCompetitions  int64
	ActiveCompetitions int64
	TotalTeams         int64
	TotalRegistrations int64
	OpenReports        int64
	ByCategory         map[string]int64
}

type StatsModel struct {
	DB *sql.DB
}

func (m *StatsModel) Get() (*PlatformStats, error) {
	s := &PlatformStats{ByCategory: map[string]int64{}}

	stmt := `SELECT
		(SELECT COUNT(*) FROM users),
		(SELECT COUNT(*) FROM organizations),
		(SELECT COUNT(*) FROM competitions),
		(SELECT COUNT(*) FROM competitions WHERE status IN ('published', 'ongoing')),
		(SELECT COUNT(*) FROM teams),
		(SELECT COUNT(*) FROM registrations),
		(SELECT COUNT(*) FROM reports WHERE status = 'open')`

	err := m.DB.QueryRow(stmt).Scan(&s.TotalUsers, &s.TotalOrganizations, &s.TotalCompetitions,
		&s.ActiveCompetitions, &s.TotalTeams, &s.TotalRegistrations, &s.OpenReports)
	if err != nil {
		return nil, err
	}

	rows, err := m.DB.Query(`SELECT category, COUNT(*) FROM competitions GROUP BY category`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var category string
		var n int64
		if err = rows.Scan(&category, &n); err != nil {
			return nil, err
		}
		s.ByCategory[category] = n
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return s, nil
}
