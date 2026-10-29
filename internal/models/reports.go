package models

import (
	"database/sql"
	"time"
)

type Report struct {
	ID           int64
	ReporterID   int64
	TargetType   string
	TargetID     int64
	Reason       string
	Status       string
	CreatedAt    time.Time
	ResolvedAt   *time.Time
	ReporterName string
}

type ReportModel struct {
	DB *sql.DB
}

func (m *ReportModel) Insert(reporterID int64, targetType string, targetID int64, reason string) error {
	stmt := `INSERT INTO reports (reporter_id, target_type, target_id, reason) VALUES ($1, $2, $3, $4)`

	_, err := m.DB.Exec(stmt, reporterID, targetType, targetID, reason)
	return err
}

// All lists reports, newest first. An empty status means every status.
func (m *ReportModel) All(status string) ([]Report, error) {
	stmt := `SELECT r.id, r.reporter_id, r.target_type, r.target_id, r.reason, r.status, r.created_at, r.resolved_at, u.name
	FROM reports r
	JOIN users u ON u.id = r.reporter_id
	WHERE ($1 = '' OR r.status = $1)
	ORDER BY r.created_at DESC`

	rows, err := m.DB.Query(stmt, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reports []Report

	for rows.Next() {
		var r Report
		err = rows.Scan(&r.ID, &r.ReporterID, &r.TargetType, &r.TargetID, &r.Reason, &r.Status,
			&r.CreatedAt, &r.ResolvedAt, &r.ReporterName)
		if err != nil {
			return nil, err
		}
		reports = append(reports, r)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return reports, nil
}

// Resolve closes a report as "resolved" or "dismissed".
func (m *ReportModel) Resolve(id int64, status string) error {
	stmt := `UPDATE reports SET status = $1, resolved_at = now() WHERE id = $2`
	return execOne(m.DB, stmt, status, id)
}
