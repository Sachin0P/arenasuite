package app

import (
	"encoding/json"
	"time"

	"arenasuite/internal/tournament"
)

// The structs below mirror the database tables. The `db` tag is the column
// name; extra fields (like CaptainName) are filled from JOINs when a query
// selects them, and stay empty otherwise.

type User struct {
	ID             int64  `db:"id"`
	GoogleID       string `db:"google_id"`
	Name           string `db:"name"`
	Email          string `db:"email"`
	ProfilePicture string `db:"profile_picture"`
	Role           string `db:"role"` // participant | organizer | admin
	College        string `db:"college"`
	RollNumber     string `db:"roll_number"`
	Branch         string `db:"branch"`
	Status         string `db:"status"` // active | suspended
	CreatedAt      string `db:"created_at"`
}

func (u *User) IsAdmin() bool     { return u != nil && u.Role == "admin" }
func (u *User) IsOrganizer() bool { return u != nil && (u.Role == "organizer" || u.Role == "admin") }

// ProfileComplete: every user must fill in roll number and branch.
func (u *User) ProfileComplete() bool { return u.RollNumber != "" && u.Branch != "" }

type Organization struct {
	ID          int64  `db:"id"`
	Name        string `db:"name"`
	Description string `db:"description"`
	Logo        string `db:"logo"`
	College     string `db:"college"`
	Verified    bool   `db:"verified"`
	CreatedBy   int64  `db:"created_by"`
	CreatedAt   string `db:"created_at"`

	CreatedByName     string `db:"created_by_name"`
	CompetitionsCount int64  `db:"competitions_count"`
}

type Competition struct {
	ID                   int64   `db:"id"`
	OrganizationID       int64   `db:"organization_id"`
	Name                 string  `db:"name"`
	Description          string  `db:"description"`
	Category             string  `db:"category"` // SPORTS | ACADEMIC | ESPORTS
	Type                 string  `db:"type"`
	Banner               string  `db:"banner"`
	StartDate            string  `db:"start_date"`
	EndDate              string  `db:"end_date"`
	RegistrationDeadline string  `db:"registration_deadline"`
	Venue                string  `db:"venue"`
	Mode                 string  `db:"mode"`             // online | offline
	MaxTeamSize          int64   `db:"max_participants"` // max people per team (0 = no limit)
	MaxTeams             int64   `db:"max_teams"`
	RegistrationFee      float64 `db:"registration_fee"`
	Rules                string  `db:"rules"`
	Format               string  `db:"format"`
	StagesJSON           string  `db:"stages_json"`
	Status               string  `db:"status"`
	CreatedBy            int64   `db:"created_by"`
	CreatedAt            string  `db:"created_at"`

	// Computed in the list query so a card can render without more queries.
	OrganizerName      string `db:"organizer_name"`
	OrganizerCollege   string `db:"organizer_college"`
	OrgOwnerID         int64  `db:"org_owner_id"`
	RegistrationsCount int64  `db:"registrations_count"`
	TeamsCount         int64  `db:"teams_count"`
}

func (c Competition) IsKnockout() bool { return tournament.IsKnockout(c.Format) }

// RegistrationOpen: published and the deadline hasn't passed yet.
func (c Competition) RegistrationOpen() bool {
	return c.Status == "published" && c.RegistrationDeadline >= nowLocal()
}

// Stages returns the Academic stage names stored as a JSON array.
func (c Competition) Stages() []string {
	var stages []string
	json.Unmarshal([]byte(c.StagesJSON), &stages)
	return stages
}

type Team struct {
	ID                int64  `db:"id"`
	CompetitionID     int64  `db:"competition_id"`
	Name              string `db:"name"`
	Logo              string `db:"logo"`
	Description       string `db:"description"`
	CaptainID         int64  `db:"captain_id"`
	MaxMembers        int64  `db:"max_members"`
	RequiredSkills    string `db:"required_skills"`
	LookingForMembers bool   `db:"looking_for_members"`
	LookingForRole    string `db:"looking_for_role"`
	Status            string `db:"status"`
	CreatedAt         string `db:"created_at"`

	CaptainName     string `db:"captain_name"`
	MemberCount     int64  `db:"member_count"`
	CompetitionName string `db:"competition_name"`
	Category        string `db:"category"`
	MaxTeamSize     int64  `db:"competition_max_team_size"` // the competition's limit, 0 = none

	Members []TeamMember // only filled by TeamByID
}

// Capacity is how many people the team can hold: its own size, capped by the
// competition's limit (in case the organizer lowered it after the team formed).
func (t Team) Capacity() int64 {
	if t.MaxTeamSize > 0 && t.MaxTeamSize < t.MaxMembers {
		return t.MaxTeamSize
	}
	return t.MaxMembers
}

func (t Team) IsFull() bool { return t.MemberCount >= t.Capacity() }

// HasMember reports whether a user is an active member (needs Members loaded).
func (t Team) HasMember(userID int64) bool {
	for _, m := range t.Members {
		if m.UserID == userID {
			return true
		}
	}
	return false
}

type TeamMember struct {
	ID             int64  `db:"id"`
	TeamID         int64  `db:"team_id"`
	UserID         int64  `db:"user_id"`
	Role           string `db:"role"` // captain | member
	Status         string `db:"status"`
	JoinedAt       string `db:"joined_at"`
	Name           string `db:"name"`
	Email          string `db:"email"`
	ProfilePicture string `db:"profile_picture"`
}

type JoinRequest struct {
	ID        int64  `db:"id"`
	TeamID    int64  `db:"team_id"`
	UserID    int64  `db:"user_id"`
	Message   string `db:"message"`
	Status    string `db:"status"`
	CreatedAt string `db:"created_at"`
	Name      string `db:"name"`
	Email     string `db:"email"`
}

type Invitation struct {
	ID            int64  `db:"id"`
	TeamID        int64  `db:"team_id"`
	UserID        int64  `db:"user_id"`
	InvitedBy     int64  `db:"invited_by"`
	Status        string `db:"status"`
	CreatedAt     string `db:"created_at"`
	TeamName      string `db:"team_name"`
	CompetitionID int64  `db:"competition_id"`
	InviterName   string `db:"inviter_name"`
}

// Post is a "looking for a team" post on the Find a Team page.
type Post struct {
	ID              int64  `db:"id"`
	CompetitionID   int64  `db:"competition_id"`
	UserID          int64  `db:"user_id"`
	Role            string `db:"role"`
	Skills          string `db:"skills"`
	Experience      string `db:"experience"`
	Description     string `db:"description"`
	Status          string `db:"status"`
	CreatedAt       string `db:"created_at"`
	UserName        string `db:"user_name"`
	ProfilePicture  string `db:"profile_picture"`
	CompetitionName string `db:"competition_name"`
	Category        string `db:"category"`
}

type Registration struct {
	ID              int64  `db:"id"`
	CompetitionID   int64  `db:"competition_id"`
	UserID          int64  `db:"user_id"`
	TeamID          int64  `db:"team_id"` // 0 = individual registration
	Status          string `db:"status"`
	RegisteredAt    string `db:"registered_at"`
	RegistrantName  string `db:"registrant_name"`
	RegistrantEmail string `db:"registrant_email"`
	TeamName        string `db:"team_name"`
	CompetitionName string `db:"competition_name"`
}

type Match struct {
	ID            int64  `db:"id"`
	CompetitionID int64  `db:"competition_id"`
	Round         string `db:"round"`
	RoundOrder    int64  `db:"round_order"`
	TeamAID       int64  `db:"team_a_id"` // 0 = TBD
	TeamBID       int64  `db:"team_b_id"`
	ScoreA        *int64 `db:"score_a"` // nil = no score yet (different from 0)
	ScoreB        *int64 `db:"score_b"`
	WinnerID      int64  `db:"winner_id"`
	ScheduledAt   string `db:"scheduled_at"`
	Venue         string `db:"venue"`
	Status        string `db:"status"`
	NextMatchID   int64  `db:"next_match_id"`
	NextMatchSlot string `db:"next_match_slot"`

	TeamAName       string `db:"team_a_name"`
	TeamBName       string `db:"team_b_name"`
	WinnerName      string `db:"winner_name"`
	CompetitionName string `db:"competition_name"`
}

type Standing struct {
	ID              int64  `db:"id"`
	CompetitionID   int64  `db:"competition_id"`
	TeamID          int64  `db:"team_id"`
	Played          int64  `db:"played"`
	Won             int64  `db:"won"`
	Lost            int64  `db:"lost"`
	Draw            int64  `db:"draw"`
	Points          int64  `db:"points"`
	ScoreDifference int64  `db:"score_difference"`
	Rank            int64  `db:"rank"`
	StatsJSON       string `db:"stats_json"`
	TeamName        string `db:"team_name"`
}

type Announcement struct {
	ID            int64  `db:"id"`
	CompetitionID int64  `db:"competition_id"`
	Title         string `db:"title"`
	Message       string `db:"message"`
	CreatedBy     int64  `db:"created_by"`
	CreatedAt     string `db:"created_at"`
	CreatedByName string `db:"created_by_name"`
}

type Notification struct {
	ID          int64  `db:"id" json:"id"`
	UserID      int64  `db:"user_id" json:"userId"`
	Type        string `db:"type" json:"type"`
	Message     string `db:"message" json:"message"`
	ReferenceID int64  `db:"reference_id" json:"referenceId"`
	IsRead      bool   `db:"is_read" json:"isRead"`
	CreatedAt   string `db:"created_at" json:"createdAt"`
}

type Report struct {
	ID           int64  `db:"id"`
	ReporterID   int64  `db:"reporter_id"`
	TargetType   string `db:"target_type"`
	TargetID     int64  `db:"target_id"`
	Reason       string `db:"reason"`
	Status       string `db:"status"`
	CreatedAt    string `db:"created_at"`
	ResolvedAt   string `db:"resolved_at"`
	ReporterName string `db:"reporter_name"`
}

// Competition dates are stored the way an <input type="datetime-local">
// sends them ("2026-10-20T10:00"), in the server's local time. Keeping one
// format means plain string comparison sorts them correctly.
const DateTimeLayout = "2006-01-02T15:04"

func nowLocal() string { return time.Now().Format(DateTimeLayout) }

// sqlNow is the same "now" for use inside SQL queries.
const sqlNow = "strftime('%Y-%m-%dT%H:%M', 'now', 'localtime')"
