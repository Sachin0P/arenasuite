package models

import (
	"database/sql"
	"errors"
	"time"
)

type User struct {
	ID             int64
	GoogleID       string
	Name           string
	Email          string
	ProfilePicture string
	Role           string // participant | organizer | admin
	College        string
	RollNumber     string
	Branch         string
	Status         string // active | suspended
	CreatedAt      time.Time
}

// IsAdmin and IsOrganizer are safe to call on a nil *User (a visitor who
// isn't signed in), which keeps the templates simple.
func (u *User) IsAdmin() bool     { return u != nil && u.Role == "admin" }
func (u *User) IsOrganizer() bool { return u != nil && (u.Role == "organizer" || u.Role == "admin") }

// ProfileComplete reports whether the user has filled in the roll number and
// branch that every account needs.
func (u *User) ProfileComplete() bool { return u.RollNumber != "" && u.Branch != "" }

type UserModel struct {
	DB *sql.DB
}

// Optional text columns are wrapped in COALESCE() so that a NULL is read as
// an empty string, and we can scan into plain string fields.
const userColumns = `id, google_id, name, email, COALESCE(profile_picture, ''), role,
	COALESCE(college, ''), COALESCE(roll_number, ''), COALESCE(branch, ''), status, created_at`

func scanUser(row scanner) (*User, error) {
	u := &User{}
	err := row.Scan(&u.ID, &u.GoogleID, &u.Name, &u.Email, &u.ProfilePicture, &u.Role,
		&u.College, &u.RollNumber, &u.Branch, &u.Status, &u.CreatedAt)
	return u, err
}

func (m *UserModel) Get(id int64) (*User, error) {
	stmt := `SELECT ` + userColumns + ` FROM users WHERE id = $1`

	u, err := scanUser(m.DB.QueryRow(stmt, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		}
		return nil, err
	}

	return u, nil
}

// GetByGoogleIDOrEmail finds the account for a Google sign-in. Matching on
// email too means an account created by the seed command is picked up the
// first time its owner signs in with Google.
func (m *UserModel) GetByGoogleIDOrEmail(googleID, email string) (*User, error) {
	stmt := `SELECT ` + userColumns + ` FROM users WHERE google_id = $1 OR email = $2`

	u, err := scanUser(m.DB.QueryRow(stmt, googleID, email))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		}
		return nil, err
	}

	return u, nil
}

func (m *UserModel) Insert(googleID, name, email, picture, role, college string) (int64, error) {
	stmt := `INSERT INTO users (google_id, name, email, profile_picture, role, college)
	VALUES ($1, $2, $3, NULLIF($4, ''), $5, NULLIF($6, ''))
	RETURNING id`

	var id int64
	err := m.DB.QueryRow(stmt, googleID, name, email, picture, role, college).Scan(&id)
	return id, err
}

// UpdateGoogleProfile refreshes the name and picture from Google on each sign-in.
func (m *UserModel) UpdateGoogleProfile(id int64, name, picture string) error {
	stmt := `UPDATE users SET name = $1, profile_picture = NULLIF($2, ''), updated_at = now() WHERE id = $3`

	_, err := m.DB.Exec(stmt, name, picture, id)
	return err
}

func (m *UserModel) UpdateProfile(id int64, college, rollNumber, branch string) error {
	stmt := `UPDATE users SET college = NULLIF($1, ''), roll_number = $2, branch = $3, updated_at = now() WHERE id = $4`

	_, err := m.DB.Exec(stmt, college, rollNumber, branch, id)
	return err
}

// All is used by the development-only login page.
func (m *UserModel) All() ([]User, error) {
	return m.list(`SELECT ` + userColumns + ` FROM users ORDER BY role, name`)
}

// Search powers the admin users page. Empty arguments are ignored.
func (m *UserModel) Search(role, status, q string) ([]User, error) {
	stmt := `SELECT ` + userColumns + ` FROM users
	WHERE ($1 = '' OR role = $1)
	AND ($2 = '' OR status = $2)
	AND ($3 = '' OR name ILIKE '%' || $3 || '%' OR email ILIKE '%' || $3 || '%')
	ORDER BY created_at DESC`

	return m.list(stmt, role, status, q)
}

func (m *UserModel) list(stmt string, args ...any) ([]User, error) {
	rows, err := m.DB.Query(stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User

	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, *u)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return users, nil
}

func (m *UserModel) SetRole(id int64, role string) error {
	stmt := `UPDATE users SET role = $1, updated_at = now() WHERE id = $2`
	return execOne(m.DB, stmt, role, id)
}

func (m *UserModel) SetStatus(id int64, status string) error {
	stmt := `UPDATE users SET status = $1, updated_at = now() WHERE id = $2`
	return execOne(m.DB, stmt, status, id)
}

// execOne runs an UPDATE or DELETE that should affect exactly one row, and
// returns ErrNoRecord if no row matched.
func execOne(q querier, stmt string, args ...any) error {
	result, err := q.Exec(stmt, args...)
	if err != nil {
		return err
	}

	n, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if n == 0 {
		return ErrNoRecord
	}

	return nil
}
