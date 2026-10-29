package models

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"arenasuite/internal/tournament"
)

type Competition struct {
	ID                   int64
	OrganizationID       int64
	Name                 string
	Description          string
	Category             string // SPORTS | ACADEMIC | ESPORTS
	Type                 string
	Banner               string
	StartDate            time.Time
	EndDate              time.Time
	RegistrationDeadline time.Time
	Venue                string
	Mode                 string // online | offline
	MaxTeamSize          int64  // max people per team (0 = no limit)
	MaxTeams             int64  // 0 = no limit
	RegistrationFee      float64
	Rules                string
	Format               string
	StagesJSON           string
	Status               string
	CreatedBy            int64
	CreatedAt            time.Time

	// Filled from JOINs so a card can render without more queries.
	OrganizerName      string
	OrganizerCollege   string
	OrgOwnerID         int64
	RegistrationsCount int64
	TeamsCount         int64
}

func (c Competition) IsKnockout() bool { return tournament.IsKnockout(c.Format) }

// OrganizedBy reports whether u runs this competition (owns its organization).
// Organizers can't take part in their own competitions.
func (c Competition) OrganizedBy(u *User) bool { return u != nil && c.OrgOwnerID == u.ID }

// ManagedBy reports whether u may manage this competition: admins manage
// everything, organizers manage the competitions of their own organizations.
func (c Competition) ManagedBy(u *User) bool { return u.IsAdmin() || c.OrganizedBy(u) }

// RegistrationOpen reports whether the competition is published and its
// registration deadline hasn't passed yet.
func (c Competition) RegistrationOpen() bool {
	return c.Status == "published" && time.Now().Before(c.RegistrationDeadline)
}

// Stages returns the Academic stage names, which are stored as a JSON array.
func (c Competition) Stages() []string {
	var stages []string
	json.Unmarshal([]byte(c.StagesJSON), &stages)
	return stages
}

type CompetitionModel struct {
	DB *sql.DB
}

// competitionSelect adds the organizer's name and the approved registration
// and team counts to each competition, so cards render from one query.
const competitionSelect = `SELECT c.id, c.organization_id, c.name, COALESCE(c.description, ''), c.category,
	COALESCE(c.type, ''), COALESCE(c.banner, ''), c.start_date, c.end_date, c.registration_deadline,
	COALESCE(c.venue, ''), c.mode, COALESCE(c.max_participants, 0), COALESCE(c.max_teams, 0),
	c.registration_fee, COALESCE(c.rules, ''), c.format, COALESCE(c.stages_json, ''), c.status,
	c.created_by, c.created_at,
	o.name, COALESCE(o.college, ''), o.created_by,
	(SELECT COUNT(*) FROM registrations r WHERE r.competition_id = c.id AND r.status = 'approved') AS registrations_count,
	(SELECT COUNT(DISTINCT t.id) FROM teams t
		JOIN registrations r ON r.team_id = t.id AND r.status = 'approved'
		WHERE t.competition_id = c.id)
	FROM competitions c
	JOIN organizations o ON o.id = c.organization_id`

func scanCompetition(row scanner) (*Competition, error) {
	c := &Competition{}
	err := row.Scan(&c.ID, &c.OrganizationID, &c.Name, &c.Description, &c.Category,
		&c.Type, &c.Banner, &c.StartDate, &c.EndDate, &c.RegistrationDeadline,
		&c.Venue, &c.Mode, &c.MaxTeamSize, &c.MaxTeams,
		&c.RegistrationFee, &c.Rules, &c.Format, &c.StagesJSON, &c.Status,
		&c.CreatedBy, &c.CreatedAt,
		&c.OrganizerName, &c.OrganizerCollege, &c.OrgOwnerID,
		&c.RegistrationsCount, &c.TeamsCount)
	return c, err
}

// Get returns a competition regardless of its status. Deciding who may see a
// draft is the handler's job.
func (m *CompetitionModel) Get(id int64) (*Competition, error) {
	c, err := scanCompetition(m.DB.QueryRow(competitionSelect+` WHERE c.id = $1`, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		}
		return nil, err
	}

	return c, nil
}

// CompetitionFilter holds the search options for List. Zero values are ignored.
type CompetitionFilter struct {
	Q                string
	Category         string
	Mode             string
	Free             string // "true" = free only, "false" = paid only
	RegistrationOpen bool
	ClosingSoonDays  int
	Status           string
	IncludeDrafts    bool  // drafts are hidden unless this is set
	OwnerID          int64 // only competitions of this user's organizations
	Sort             string
	Limit            int
}

// SortSafelist holds the sort values List accepts. The ORDER BY clause can't
// be a placeholder parameter, so it's chosen from this fixed list instead of
// being built from user input.
var SortSafelist = map[string]string{
	"":            "c.start_date ASC",
	"recent":      "c.created_at DESC",
	"closingSoon": "c.registration_deadline ASC",
	"popular":     "registrations_count DESC",
}

func (m *CompetitionModel) List(f CompetitionFilter) ([]Competition, error) {
	orderBy, ok := SortSafelist[f.Sort]
	if !ok {
		orderBy = SortSafelist[""]
	}

	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 50
	}

	// Each condition is written so that an empty filter value switches it
	// off, which lets us use one fixed query with placeholders for everything.
	stmt := competitionSelect + `
	WHERE ($1 = '' OR c.name ILIKE '%' || $1 || '%')
	AND ($2 = '' OR c.category = $2)
	AND ($3 = '' OR c.mode = $3)
	AND ($4 = '' OR ($4 = 'true' AND c.registration_fee = 0) OR ($4 = 'false' AND c.registration_fee > 0))
	AND (NOT $5 OR (c.status = 'published' AND c.registration_deadline >= now()))
	AND ($6 = 0 OR (c.status = 'published' AND c.registration_deadline BETWEEN now() AND now() + make_interval(days => $6)))
	AND ($7 = '' OR c.status = $7)
	AND ($8 OR c.status <> 'draft')
	AND ($9 = 0 OR o.created_by = $9)
	ORDER BY ` + orderBy + `, c.id ASC
	LIMIT $10`

	args := []any{f.Q, f.Category, f.Mode, f.Free, f.RegistrationOpen, f.ClosingSoonDays,
		f.Status, f.IncludeDrafts || f.Status != "", f.OwnerID, f.Limit}

	return m.list(stmt, args...)
}

// All includes drafts, for the admin list.
func (m *CompetitionModel) All() ([]Competition, error) {
	return m.list(competitionSelect + ` ORDER BY c.created_at DESC`)
}

// ForOrganization returns an organization's upcoming and past competitions
// for its public profile page.
func (m *CompetitionModel) ForOrganization(orgID int64) (upcoming, past []Competition, err error) {
	upcoming, err = m.list(competitionSelect+
		` WHERE c.organization_id = $1 AND c.status IN ('published', 'ongoing') ORDER BY c.start_date ASC`, orgID)
	if err != nil {
		return nil, nil, err
	}

	past, err = m.list(competitionSelect+
		` WHERE c.organization_id = $1 AND c.status = 'completed' ORDER BY c.start_date DESC`, orgID)
	if err != nil {
		return nil, nil, err
	}

	return upcoming, past, nil
}

func (m *CompetitionModel) list(stmt string, args ...any) ([]Competition, error) {
	rows, err := m.DB.Query(stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var competitions []Competition

	for rows.Next() {
		c, err := scanCompetition(rows)
		if err != nil {
			return nil, err
		}
		competitions = append(competitions, *c)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return competitions, nil
}

// Insert creates a competition from the fields the create form fills in, and
// returns the new id.
func (m *CompetitionModel) Insert(c *Competition) (int64, error) {
	stmt := `INSERT INTO competitions (
		organization_id, name, description, category, type, banner,
		start_date, end_date, registration_deadline, venue, mode,
		max_participants, max_teams, registration_fee, rules, format, stages_json,
		status, created_by)
	VALUES ($1, $2, NULLIF($3, ''), $4, NULLIF($5, ''), NULLIF($6, ''),
		$7, $8, $9, NULLIF($10, ''), $11,
		NULLIF($12, 0), NULLIF($13, 0), $14, NULLIF($15, ''), $16, NULLIF($17, ''),
		$18, $19)
	RETURNING id`

	args := []any{
		c.OrganizationID, c.Name, c.Description, c.Category, c.Type, c.Banner,
		c.StartDate, c.EndDate, c.RegistrationDeadline, c.Venue, c.Mode,
		c.MaxTeamSize, c.MaxTeams, c.RegistrationFee, c.Rules, c.Format, c.StagesJSON,
		c.Status, c.CreatedBy,
	}

	var id int64
	err := m.DB.QueryRow(stmt, args...).Scan(&id)
	return id, err
}

// Update saves the fields the edit form can change. Moving a competition to
// another organization isn't supported.
func (m *CompetitionModel) Update(c *Competition) error {
	stmt := `UPDATE competitions SET
		name = $1, description = NULLIF($2, ''), category = $3, type = NULLIF($4, ''), banner = NULLIF($5, ''),
		start_date = $6, end_date = $7, registration_deadline = $8, venue = NULLIF($9, ''), mode = $10,
		max_participants = NULLIF($11, 0), max_teams = NULLIF($12, 0), registration_fee = $13,
		rules = NULLIF($14, ''), format = $15, stages_json = NULLIF($16, ''), updated_at = now()
	WHERE id = $17`

	args := []any{
		c.Name, c.Description, c.Category, c.Type, c.Banner,
		c.StartDate, c.EndDate, c.RegistrationDeadline, c.Venue, c.Mode,
		c.MaxTeamSize, c.MaxTeams, c.RegistrationFee,
		c.Rules, c.Format, c.StagesJSON, c.ID,
	}

	return execOne(m.DB, stmt, args...)
}

func (m *CompetitionModel) SetStatus(id int64, status string) error {
	stmt := `UPDATE competitions SET status = $1, updated_at = now() WHERE id = $2`
	return execOne(m.DB, stmt, status, id)
}

// Delete removes a competition. Its teams, matches and registrations go too
// (ON DELETE CASCADE).
func (m *CompetitionModel) Delete(id int64) error {
	return execOne(m.DB, `DELETE FROM competitions WHERE id = $1`, id)
}
