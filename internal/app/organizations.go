package app

import (
	"database/sql"
	"strings"

	"arenasuite/internal/db"
)

const orgSelect = `
	SELECT o.*, u.name AS created_by_name,
		(SELECT COUNT(*) FROM competitions c WHERE c.organization_id = o.id) AS competitions_count
	FROM organizations o
	JOIN users u ON u.id = o.created_by`

func (a *App) ListOrganizations() ([]Organization, error) {
	return db.All[Organization](a.DB, orgSelect+` ORDER BY o.created_at DESC`)
}

func (a *App) OrganizationsOwnedBy(userID int64) ([]Organization, error) {
	return db.All[Organization](a.DB, orgSelect+` WHERE o.created_by = ? ORDER BY o.name`, userID)
}

func (a *App) OrganizationByID(id int64) (*Organization, error) {
	org, err := db.One[Organization](a.DB, orgSelect+` WHERE o.id = ?`, id)
	if err == nil && org == nil {
		return nil, fail(404, "Organization not found")
	}
	return org, err
}

type OrganizationInput struct {
	Name, Description, Logo, College string
}

// CreateOrganization is also how someone becomes an organizer: a
// participant who creates an organization is promoted in the same step.
func (a *App) CreateOrganization(user *User, in OrganizationInput) (int64, error) {
	if strings.TrimSpace(in.Name) == "" {
		return 0, fail(400, "Organization name is required")
	}
	var id int64
	err := a.inTx(func(tx *sql.Tx) error {
		res, err := tx.Exec(`INSERT INTO organizations (name, description, logo, college, created_by) VALUES (?, ?, ?, ?, ?)`,
			in.Name, db.NullIfEmpty(in.Description), db.NullIfEmpty(in.Logo), db.NullIfEmpty(in.College), user.ID)
		if err != nil {
			return err
		}
		id, _ = res.LastInsertId()
		_, err = tx.Exec(`UPDATE users SET role = 'organizer', updated_at = CURRENT_TIMESTAMP WHERE id = ? AND role = 'participant'`, user.ID)
		return err
	})
	return id, err
}

func (a *App) UpdateOrganization(user *User, id int64, in OrganizationInput) error {
	org, err := a.OrganizationByID(id)
	if err != nil {
		return err
	}
	if !user.IsAdmin() && org.CreatedBy != user.ID {
		return fail(403, "You do not own this organization")
	}
	if strings.TrimSpace(in.Name) == "" {
		return fail(400, "Organization name is required")
	}
	_, err = a.DB.Exec(`UPDATE organizations SET name = ?, description = ?, logo = ?, college = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		in.Name, db.NullIfEmpty(in.Description), db.NullIfEmpty(in.Logo), db.NullIfEmpty(in.College), id)
	return err
}

// OrganizationCompetitions returns an org's upcoming and past competitions
// for its public profile page.
func (a *App) OrganizationCompetitions(orgID int64) (upcoming, past []Competition, err error) {
	upcoming, err = db.All[Competition](a.DB, competitionSelect+
		` WHERE c.organization_id = ? AND c.status IN ('published', 'ongoing') ORDER BY c.start_date ASC`, orgID)
	if err != nil {
		return
	}
	past, err = db.All[Competition](a.DB, competitionSelect+
		` WHERE c.organization_id = ? AND c.status = 'completed' ORDER BY c.start_date DESC`, orgID)
	return
}
