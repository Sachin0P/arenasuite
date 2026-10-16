package app

import (
	"encoding/json"
	"fmt"
	"strings"

	"arenasuite/internal/db"
)

// competitionSelect adds the organizer's name and the approved registration
// and team counts to each competition, so cards render from one query.
const competitionSelect = `
	SELECT c.*, o.name AS organizer_name, o.college AS organizer_college, o.created_by AS org_owner_id,
		(SELECT COUNT(*) FROM registrations r WHERE r.competition_id = c.id AND r.status = 'approved') AS registrations_count,
		(SELECT COUNT(DISTINCT t.id) FROM teams t
			JOIN registrations r ON r.team_id = t.id AND r.status = 'approved'
			WHERE t.competition_id = c.id) AS teams_count
	FROM competitions c
	JOIN organizations o ON o.id = c.organization_id`

// Filter holds the search options for ListCompetitions. Empty fields are ignored.
type Filter struct {
	Category, Type, College, Mode, Q string
	OrganizationID                   int64
	DateFrom, DateTo                 string
	Free                             string // "true" = free only, "false" = paid only
	RegistrationOpen                 bool
	ClosingSoonDays                  int
	Status                           string
	Sort                             string // "" (by start date) | recent | closingSoon | popular
	Mine                             bool   // only competitions the user's organizations own
	Limit                            int
}

func (a *App) ListCompetitions(user *User, f Filter) ([]Competition, error) {
	var where []string
	var args []any
	add := func(clause string, values ...any) {
		where = append(where, clause)
		args = append(args, values...)
	}

	if f.Category != "" {
		if err := mustBeOneOf(f.Category, Categories, "category"); err != nil {
			return nil, err
		}
		add("c.category = ?", f.Category)
	}
	if f.Type != "" {
		add("c.type ILIKE ?", "%"+f.Type+"%")
	}
	if f.College != "" {
		add("o.college ILIKE ?", "%"+f.College+"%")
	}
	if f.OrganizationID != 0 {
		add("c.organization_id = ?", f.OrganizationID)
	}
	if f.Mode != "" {
		add("c.mode = ?", f.Mode)
	}
	if f.DateFrom != "" {
		add("c.start_date >= ?", f.DateFrom)
	}
	if f.DateTo != "" {
		add("c.start_date <= ?", f.DateTo)
	}
	if f.Q != "" {
		add("c.name ILIKE ?", "%"+f.Q+"%")
	}
	switch f.Free {
	case "true":
		add("c.registration_fee = 0")
	case "false":
		add("c.registration_fee > 0")
	}
	if f.RegistrationOpen {
		add("c.status = 'published' AND c.registration_deadline >= now()")
	}
	if f.ClosingSoonDays > 0 {
		add("c.status = 'published' AND c.registration_deadline BETWEEN now() AND now() + make_interval(days => ?)", f.ClosingSoonDays)
	}

	// Who sees what: the public never sees drafts. Owners see their own
	// drafts in "mine" lists; admins see everything.
	if f.Status != "" {
		if err := mustBeOneOf(f.Status, CompetitionStatuses, "status"); err != nil {
			return nil, err
		}
		add("c.status = ?", f.Status)
	} else if !user.IsAdmin() && !(f.Mine && user != nil) {
		add("c.status != 'draft'")
	}
	if f.Mine && user != nil {
		add("o.created_by = ?", user.ID)
	}

	query := competitionSelect
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	switch f.Sort {
	case "recent":
		query += " ORDER BY c.created_at DESC"
	case "closingSoon":
		query += " ORDER BY c.registration_deadline ASC"
	case "popular":
		query += " ORDER BY registrations_count DESC"
	default:
		query += " ORDER BY c.start_date ASC"
	}
	limit := f.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	query += fmt.Sprintf(" LIMIT %d", limit)

	return db.All[Competition](a.DB, query, args...)
}

// getCompetition loads a competition without any visibility check.
func (a *App) getCompetition(q db.Querier, id int64) (*Competition, error) {
	c, err := db.One[Competition](q, competitionSelect+` WHERE c.id = ?`, id)
	if err == nil && c == nil {
		return nil, fail(404, "Competition not found")
	}
	return c, err
}

// CompetitionByID loads a competition for viewing. Drafts are hidden
// (reported as "not found") from everyone except the owner and admins.
func (a *App) CompetitionByID(user *User, id int64) (*Competition, error) {
	c, err := a.getCompetition(a.DB, id)
	if err != nil {
		return nil, err
	}
	if c.Status == "draft" && !CanManage(user, c) {
		return nil, fail(404, "Competition not found")
	}
	return c, nil
}

// CanManage: admins manage everything; organizers manage competitions that
// belong to an organization they created.
func CanManage(user *User, c *Competition) bool {
	return user != nil && (user.IsAdmin() || c.OrgOwnerID == user.ID)
}

// competitionToManage loads a competition and checks the user may manage it.
func (a *App) competitionToManage(user *User, id int64) (*Competition, error) {
	c, err := a.getCompetition(a.DB, id)
	if err != nil {
		return nil, err
	}
	if !CanManage(user, c) {
		return nil, fail(403, "You do not have permission to manage this competition")
	}
	return c, nil
}

// CompetitionInput is what the create/edit wizard submits.
type CompetitionInput struct {
	OrganizationID                            int64
	Name, Description, Category, Type, Banner string
	StartDate, EndDate, RegistrationDeadline  string
	Venue, Mode                               string
	MaxTeamSize, MaxTeams                     int64
	RegistrationFee                           float64
	Rules, Format                             string
	Stages                                    []string // Academic only
	Status                                    string   // "draft" or "published" (create only)

	// The three dates as real times, filled in by validate().
	start, end, deadline any
}

func (in *CompetitionInput) validate() error {
	if in.OrganizationID == 0 || strings.TrimSpace(in.Name) == "" || in.Category == "" ||
		in.StartDate == "" || in.EndDate == "" || in.RegistrationDeadline == "" {
		return fail(400, "Please fill in organization, name, category, start/end dates and registration deadline")
	}
	if err := mustBeOneOf(in.Category, Categories, "category"); err != nil {
		return err
	}
	if in.Format == "" {
		in.Format = "knockout"
		if in.Category == "ACADEMIC" {
			in.Format = "staged"
		}
	}
	if err := mustBeOneOf(in.Format, CompetitionFormats, "format"); err != nil {
		return err
	}
	if in.Mode != "online" {
		in.Mode = "offline"
	}
	if in.MaxTeamSize < 0 || in.MaxTeams < 0 || in.RegistrationFee < 0 {
		return fail(400, "Limits and fee can't be negative")
	}
	if in.EndDate < in.StartDate {
		return fail(400, "End date must be after the start date")
	}
	// Turn the form's "2026-10-20T10:00" text into times for the database.
	var errs [3]error
	in.start, errs[0] = db.LocalTime(in.StartDate)
	in.end, errs[1] = db.LocalTime(in.EndDate)
	in.deadline, errs[2] = db.LocalTime(in.RegistrationDeadline)
	for _, err := range errs {
		if err != nil {
			return fail(400, "Please enter valid dates")
		}
	}
	return nil
}

// stagesJSON stores Academic stage names as a JSON array (NULL otherwise).
func (in *CompetitionInput) stagesJSON() any {
	if in.Category != "ACADEMIC" || len(in.Stages) == 0 {
		return nil
	}
	b, _ := json.Marshal(in.Stages)
	return string(b)
}

func (a *App) CreateCompetition(user *User, in CompetitionInput) (int64, error) {
	if err := in.validate(); err != nil {
		return 0, err
	}
	org, err := a.OrganizationByID(in.OrganizationID)
	if err != nil {
		return 0, err
	}
	if !user.IsAdmin() && org.CreatedBy != user.ID {
		return 0, fail(403, "You do not own this organization")
	}
	status := "draft"
	if in.Status == "published" {
		status = "published"
	}

	id, err := db.Insert(a.DB, `
		INSERT INTO competitions (
			organization_id, name, description, category, type, banner,
			start_date, end_date, registration_deadline, venue, mode,
			max_participants, max_teams, registration_fee, rules, format, stages_json,
			status, created_by
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		in.OrganizationID, in.Name, db.NullIfEmpty(in.Description), in.Category, db.NullIfEmpty(in.Type), db.NullIfEmpty(in.Banner),
		in.start, in.end, in.deadline, db.NullIfEmpty(in.Venue), in.Mode,
		db.NullIfZero(in.MaxTeamSize), db.NullIfZero(in.MaxTeams), in.RegistrationFee, db.NullIfEmpty(in.Rules), in.Format, in.stagesJSON(),
		status, user.ID)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (a *App) UpdateCompetition(user *User, id int64, in CompetitionInput) error {
	c, err := a.competitionToManage(user, id)
	if err != nil {
		return err
	}
	in.OrganizationID = c.OrganizationID // moving between organizations isn't supported
	if err := in.validate(); err != nil {
		return err
	}
	_, err = a.DB.Exec(`
		UPDATE competitions SET
			name = ?, description = ?, category = ?, type = ?, banner = ?,
			start_date = ?, end_date = ?, registration_deadline = ?, venue = ?, mode = ?,
			max_participants = ?, max_teams = ?, registration_fee = ?, rules = ?, format = ?, stages_json = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`,
		in.Name, db.NullIfEmpty(in.Description), in.Category, db.NullIfEmpty(in.Type), db.NullIfEmpty(in.Banner),
		in.start, in.end, in.deadline, db.NullIfEmpty(in.Venue), in.Mode,
		db.NullIfZero(in.MaxTeamSize), db.NullIfZero(in.MaxTeams), in.RegistrationFee, db.NullIfEmpty(in.Rules), in.Format, in.stagesJSON(),
		id)
	return err
}

func (a *App) SetCompetitionStatus(user *User, id int64, status string) error {
	if _, err := a.competitionToManage(user, id); err != nil {
		return err
	}
	if err := mustBeOneOf(status, CompetitionStatuses, "status"); err != nil {
		return err
	}
	_, err := a.DB.Exec(`UPDATE competitions SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, status, id)
	return err
}

func (a *App) DeleteCompetition(user *User, id int64) error {
	if _, err := a.competitionToManage(user, id); err != nil {
		return err
	}
	_, err := a.DB.Exec(`DELETE FROM competitions WHERE id = ?`, id)
	return err
}
