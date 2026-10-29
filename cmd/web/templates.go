package main

import (
	"fmt"
	"html/template"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"arenasuite/internal/models"
	"arenasuite/ui"
)

// templateData holds any dynamic data that we want to pass to the HTML
// templates. Each page only fills in the fields it needs.
type templateData struct {
	CurrentYear     int
	Flash           string
	FlashError      string
	IsAuthenticated bool
	User            *models.User // the signed-in user, or nil for visitors
	Unread          int          // unread notifications, for the badge
	Path            string       // the current URL path, to highlight the nav
	LiveID          int64        // a competition id makes the page subscribe to its live events
	Section         string       // the active entry of the organizer/admin side menu
	Heading         string
	Message         string
	Form            any

	Sections             []homeSection
	Competition          *models.Competition
	Competitions         []models.Competition
	UpcomingCompetitions []models.Competition
	PastCompetitions     []models.Competition
	Filter               models.CompetitionFilter
	LockedCategory       string
	CanManage            bool
	EditingID            int64

	Org    *models.Organization
	Orgs   []models.Organization
	Team   *models.Team
	Teams  []models.Team
	Posts  []models.Post
	Users  []models.User
	Counts map[string]int64

	Matches         []models.Match
	UpcomingMatches []models.Match
	RecentResults   []models.Match
	Standings       []models.Standing
	Announcements   []models.Announcement
	Registrations   []models.Registration
	MyRegistration  *models.Registration
	Invitations     []models.Invitation
	JoinRequests    []models.JoinRequest
	Notifications   []models.Notification
	Reports         []models.Report
	PlatformStats   *models.PlatformStats

	CaptainOf   []models.Team
	OnTeam      bool
	IsCaptain   bool
	IsMember    bool
	MyRequests  map[int64]int64 // team id -> pending join request id
	MyRequestID int64

	Tab        string
	FindFilter models.FindTeamFilter

	Role   string
	Status string
	Q      string

	GoogleReady    bool
	Domain         string
	DevLogin       bool
	DefaultCollege string
}

type homeSection struct {
	Title        string
	Competitions []models.Competition
}

// dateTimeLayout is the format an <input type="datetime-local"> uses.
const dateTimeLayout = "2006-01-02T15:04"

// humanDate returns a nicely formatted string representation of a time.Time
// object, in the site's timezone, e.g. "Oct 20, 2026".
func humanDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(time.Local).Format("Jan 2, 2006")
}

// humanDateTime is humanDate with the time of day: "Oct 20, 2026, 10:00 AM".
func humanDateTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(time.Local).Format("Jan 2, 2006, 3:04 PM")
}

// inputDateTime formats a time for the value of a datetime-local input.
func inputDateTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(time.Local).Format(dateTimeLayout)
}

// fee shows "Free" or a rupee amount.
func fee(f float64) string {
	if f <= 0 {
		return "Free"
	}
	return fmt.Sprintf("₹%.0f", f)
}

// label turns "round_robin" into "Round robin".
func label(s string) string {
	s = strings.ReplaceAll(s, "_", " ")
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// initial is the first letter of a name, for avatar circles.
func initial(s string) string {
	for _, r := range s {
		return strings.ToUpper(string(r))
	}
	return "?"
}

// plural picks "team" or "teams".
func plural(n int64, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// dict builds a map so a partial can receive several values:
// {{template "team-card" dict "Team" . "User" $.User}}
func dict(pairs ...any) map[string]any {
	m := map[string]any{}
	for i := 0; i+1 < len(pairs); i += 2 {
		m[fmt.Sprint(pairs[i])] = pairs[i+1]
	}
	return m
}

type matchRound struct {
	Name    string
	Matches []models.Match
}

// rounds groups matches by round, in order, for the bracket view.
func rounds(matches []models.Match) []matchRound {
	var out []matchRound
	for _, m := range matches {
		if len(out) == 0 || out[len(out)-1].Name != m.Round {
			out = append(out, matchRound{Name: m.Round})
		}
		out[len(out)-1].Matches = append(out[len(out)-1].Matches, m)
	}
	return out
}

// statusColor picks the pill colour for a status word.
func statusColor(status string) string {
	switch status {
	case "published", "approved", "accepted", "open", "active":
		return "green"
	case "ongoing", "live":
		return "blue"
	case "cancelled", "rejected", "suspended":
		return "red"
	case "pending", "postponed":
		return "amber"
	}
	return ""
}

// Initialize a template.FuncMap object and store it in a global variable.
// This is essentially a string-keyed map which acts as a lookup between the
// names of our custom template functions and the functions themselves.
var functions = template.FuncMap{
	"humanDate":     humanDate,
	"humanDateTime": humanDateTime,
	"inputDateTime": inputDateTime,
	"fee":           fee,
	"label":         label,
	"initial":       initial,
	"plural":        plural,
	"dict":          dict,
	"rounds":        rounds,
	"statusColor":   statusColor,
	"lower":         strings.ToLower,
	"join":          strings.Join,
	"categories":    func() []string { return models.Categories },
	"formats":       func() []string { return models.CompetitionFormats },
	"roles":         func() []string { return models.Roles },
}

func newTemplateCache() (map[string]*template.Template, error) {
	cache := map[string]*template.Template{}

	// Use fs.Glob() to get a slice of all filepaths in the ui.Files embedded
	// filesystem which match the pattern 'html/pages/*.tmpl'.
	pages, err := fs.Glob(ui.Files, "html/pages/*.tmpl")
	if err != nil {
		return nil, err
	}

	for _, page := range pages {
		name := filepath.Base(page)

		// Create a slice containing the filepath patterns for the templates we
		// want to parse.
		patterns := []string{
			"html/base.tmpl",
			"html/partials/*.tmpl",
			page,
		}

		// Use ParseFS() instead of ParseFiles() to parse the template files
		// from the ui.Files embedded filesystem.
		ts, err := template.New(name).Funcs(functions).ParseFS(ui.Files, patterns...)
		if err != nil {
			return nil, err
		}

		cache[name] = ts
	}

	return cache, nil
}
