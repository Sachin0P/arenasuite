package models

import (
	"database/sql"
	"errors"
	"time"
)

type Organization struct {
	ID          int64
	Name        string
	Description string
	Logo        string
	College     string
	Verified    bool
	CreatedBy   int64
	CreatedAt   time.Time

	CreatedByName     string
	CompetitionsCount int64
}

type OrganizationModel struct {
	DB *sql.DB
}

// organizationSelect adds the creator's name and the number of competitions,
// so a list renders from a single query.
const organizationSelect = `SELECT o.id, o.name, COALESCE(o.description, ''), COALESCE(o.logo, ''),
	COALESCE(o.college, ''), o.verified, o.created_by, o.created_at, u.name,
	(SELECT COUNT(*) FROM competitions c WHERE c.organization_id = o.id)
	FROM organizations o
	JOIN users u ON u.id = o.created_by`

func scanOrganization(row scanner) (*Organization, error) {
	o := &Organization{}
	err := row.Scan(&o.ID, &o.Name, &o.Description, &o.Logo, &o.College, &o.Verified,
		&o.CreatedBy, &o.CreatedAt, &o.CreatedByName, &o.CompetitionsCount)
	return o, err
}

func (m *OrganizationModel) Get(id int64) (*Organization, error) {
	o, err := scanOrganization(m.DB.QueryRow(organizationSelect+` WHERE o.id = $1`, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		}
		return nil, err
	}

	return o, nil
}

func (m *OrganizationModel) All() ([]Organization, error) {
	return m.list(organizationSelect + ` ORDER BY o.created_at DESC`)
}

func (m *OrganizationModel) OwnedBy(userID int64) ([]Organization, error) {
	return m.list(organizationSelect+` WHERE o.created_by = $1 ORDER BY o.name`, userID)
}

func (m *OrganizationModel) list(stmt string, args ...any) ([]Organization, error) {
	rows, err := m.DB.Query(stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orgs []Organization

	for rows.Next() {
		o, err := scanOrganization(rows)
		if err != nil {
			return nil, err
		}
		orgs = append(orgs, *o)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return orgs, nil
}

// Insert creates an organization. Creating one is also how someone becomes an
// organizer, so a participant is promoted in the same transaction.
func (m *OrganizationModel) Insert(userID int64, name, description, logo, college string) (int64, error) {
	tx, err := m.DB.Begin()
	if err != nil {
		return 0, err
	}
	// Rollback() is a no-op once Commit() has succeeded, so deferring it is a
	// simple way to make sure we never leave a transaction open on error.
	defer tx.Rollback()

	stmt := `INSERT INTO organizations (name, description, logo, college, created_by)
	VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), NULLIF($4, ''), $5)
	RETURNING id`

	var id int64
	err = tx.QueryRow(stmt, name, description, logo, college, userID).Scan(&id)
	if err != nil {
		return 0, err
	}

	stmt = `UPDATE users SET role = 'organizer', updated_at = now() WHERE id = $1 AND role = 'participant'`

	_, err = tx.Exec(stmt, userID)
	if err != nil {
		return 0, err
	}

	err = tx.Commit()
	return id, err
}

func (m *OrganizationModel) Update(id int64, name, description, logo, college string) error {
	stmt := `UPDATE organizations SET name = $1, description = NULLIF($2, ''), logo = NULLIF($3, ''),
	college = NULLIF($4, ''), updated_at = now()
	WHERE id = $5`

	return execOne(m.DB, stmt, name, description, logo, college, id)
}

func (m *OrganizationModel) Verify(id int64) error {
	stmt := `UPDATE organizations SET verified = TRUE, updated_at = now() WHERE id = $1`
	return execOne(m.DB, stmt, id)
}
