package app

import (
	"slices"
	"strings"

	"arenasuite/internal/auth"
	"arenasuite/internal/db"
)

func (a *App) UserByID(id int64) (*User, error) {
	return db.One[User](a.DB, `SELECT * FROM users WHERE id = ?`, id)
}

// LoginWithGoogle checks the Google account is allowed in, then finds or
// creates the matching user.
func (a *App) LoginWithGoogle(g *auth.GoogleUser) (*User, error) {
	if !g.EmailVerified {
		return nil, fail(403, "Google email is not verified")
	}
	email := strings.ToLower(g.Email)
	_, domain, _ := strings.Cut(email, "@")
	if domain != a.Cfg.AllowedEmailDomain {
		return nil, fail(403, "Only @%s accounts may sign in", a.Cfg.AllowedEmailDomain)
	}

	// Emails on ADMIN_EMAILS become admin when they sign in, on the first
	// login or any later one. It never demotes: removing an email from the
	// list later doesn't take admin away.
	isBootstrapAdmin := slices.Contains(a.Cfg.AdminEmails, email)

	existing, err := db.One[User](a.DB, `SELECT * FROM users WHERE google_id = ? OR email = ?`, g.Sub, email)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		_, err = a.DB.Exec(`UPDATE users SET name = ?, profile_picture = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
			g.Name, db.NullIfEmpty(g.Picture), existing.ID)
		if err == nil && isBootstrapAdmin && existing.Role != "admin" {
			_, err = a.DB.Exec(`UPDATE users SET role = 'admin', updated_at = CURRENT_TIMESTAMP WHERE id = ?`, existing.ID)
		}
		if err != nil {
			return nil, err
		}
		return a.UserByID(existing.ID)
	}

	// Brand-new users always start as participants (unless bootstrap admin).
	role := "participant"
	if isBootstrapAdmin {
		role = "admin"
	}
	// Everyone signing in is from the same college, so fill it in for them.
	id, err := db.Insert(a.DB, `INSERT INTO users (google_id, name, email, profile_picture, role, college) VALUES (?, ?, ?, ?, ?, ?)`,
		g.Sub, g.Name, email, db.NullIfEmpty(g.Picture), role, db.NullIfEmpty(a.Cfg.DefaultCollege))
	if err != nil {
		return nil, err
	}
	return a.UserByID(id)
}

func (a *App) UpdateProfile(user *User, college, rollNumber, branch string) error {
	rollNumber, branch = strings.TrimSpace(rollNumber), strings.TrimSpace(branch)
	if rollNumber == "" || branch == "" {
		return fail(400, "Roll number and branch are required")
	}
	_, err := a.DB.Exec(`UPDATE users SET college = ?, roll_number = ?, branch = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		db.NullIfEmpty(college), rollNumber, branch, user.ID)
	return err
}

// AllUsers is used by the dev-only login page.
func (a *App) AllUsers() ([]User, error) {
	return db.All[User](a.DB, `SELECT * FROM users ORDER BY role, name`)
}
