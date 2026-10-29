package models

import (
	"database/sql"
	"errors"
	"time"
)

type Announcement struct {
	ID            int64
	CompetitionID int64
	Title         string
	Message       string
	CreatedBy     int64
	CreatedAt     time.Time
	CreatedByName string
}

type AnnouncementModel struct {
	DB *sql.DB
}

const announcementSelect = `SELECT a.id, a.competition_id, a.title, a.message, a.created_by, a.created_at, u.name
	FROM announcements a
	JOIN users u ON u.id = a.created_by`

func (m *AnnouncementModel) Insert(competitionID int64, title, message string, createdBy int64) (int64, error) {
	stmt := `INSERT INTO announcements (competition_id, title, message, created_by) VALUES ($1, $2, $3, $4) RETURNING id`

	var id int64
	err := m.DB.QueryRow(stmt, competitionID, title, message, createdBy).Scan(&id)
	return id, err
}

func (m *AnnouncementModel) Get(id int64) (*Announcement, error) {
	a := &Announcement{}

	err := m.DB.QueryRow(announcementSelect+` WHERE a.id = $1`, id).
		Scan(&a.ID, &a.CompetitionID, &a.Title, &a.Message, &a.CreatedBy, &a.CreatedAt, &a.CreatedByName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		}
		return nil, err
	}

	return a, nil
}

func (m *AnnouncementModel) ForCompetition(competitionID int64) ([]Announcement, error) {
	stmt := announcementSelect + ` WHERE a.competition_id = $1 ORDER BY a.created_at DESC, a.id DESC`

	rows, err := m.DB.Query(stmt, competitionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var announcements []Announcement

	for rows.Next() {
		var a Announcement
		err = rows.Scan(&a.ID, &a.CompetitionID, &a.Title, &a.Message, &a.CreatedBy, &a.CreatedAt, &a.CreatedByName)
		if err != nil {
			return nil, err
		}
		announcements = append(announcements, a)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return announcements, nil
}

func (m *AnnouncementModel) Delete(id int64) error {
	return execOne(m.DB, `DELETE FROM announcements WHERE id = $1`, id)
}
