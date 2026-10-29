package models

import (
	"database/sql"
	"errors"
	"time"
)

// Post is a "looking for a team" post on the Find a Team page.
type Post struct {
	ID              int64
	CompetitionID   int64
	UserID          int64
	Role            string
	Skills          string
	Experience      string
	Description     string
	Status          string
	CreatedAt       time.Time
	UserName        string
	ProfilePicture  string
	CompetitionName string
	Category        string
}

type PostModel struct {
	DB *sql.DB
}

const postSelect = `SELECT p.id, p.competition_id, p.user_id, p.role, COALESCE(p.skills, ''),
	COALESCE(p.experience, ''), COALESCE(p.description, ''), p.status, p.created_at,
	u.name, COALESCE(u.profile_picture, ''), c.name, c.category
	FROM looking_for_team_posts p
	JOIN users u ON u.id = p.user_id
	JOIN competitions c ON c.id = p.competition_id`

func scanPost(row scanner) (*Post, error) {
	p := &Post{}
	err := row.Scan(&p.ID, &p.CompetitionID, &p.UserID, &p.Role, &p.Skills,
		&p.Experience, &p.Description, &p.Status, &p.CreatedAt,
		&p.UserName, &p.ProfilePicture, &p.CompetitionName, &p.Category)
	return p, err
}

func (m *PostModel) Insert(p *Post) error {
	stmt := `INSERT INTO looking_for_team_posts (competition_id, user_id, role, skills, experience, description)
	VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''))`

	_, err := m.DB.Exec(stmt, p.CompetitionID, p.UserID, p.Role, p.Skills, p.Experience, p.Description)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrDuplicatePost
		}
		return err
	}

	return nil
}

func (m *PostModel) Get(id int64) (*Post, error) {
	p, err := scanPost(m.DB.QueryRow(postSelect+` WHERE p.id = $1`, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		}
		return nil, err
	}

	return p, nil
}

// Open lists "I need a team" posts.
func (m *PostModel) Open(f FindTeamFilter) ([]Post, error) {
	stmt := postSelect + `
	WHERE p.status = 'open'
	AND ($1 = 0 OR p.competition_id = $1)
	AND ($2 = '' OR c.category = $2)
	AND ($3 = '' OR p.role ILIKE '%' || $3 || '%' OR p.skills ILIKE '%' || $3 || '%')
	ORDER BY p.created_at DESC`

	rows, err := m.DB.Query(stmt, f.CompetitionID, f.Category, f.Role)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []Post

	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			return nil, err
		}
		posts = append(posts, *p)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return posts, nil
}

func (m *PostModel) Delete(id int64) error {
	return execOne(m.DB, `DELETE FROM looking_for_team_posts WHERE id = $1`, id)
}
