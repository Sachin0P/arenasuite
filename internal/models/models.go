package models

import (
	"database/sql"
	_ "embed"
)

// Schema creates any missing tables. It's applied when the application starts,
// and every statement uses "IF NOT EXISTS", so running it again is safe.
//
//go:embed schema.sql
var Schema string

// querier is satisfied by both *sql.DB and *sql.Tx. Helpers that accept a
// querier can run either on their own or as one step inside a transaction.
type querier interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// scanner is satisfied by both *sql.Row and *sql.Rows, so one scan function
// can be shared by a model's Get() and its list methods.
type scanner interface {
	Scan(dest ...any) error
}

// Statuses and other fixed values. The database enforces the same lists with
// CHECK constraints; these are for validating forms and filling <select>s.
var (
	Categories          = []string{"SPORTS", "ACADEMIC", "ESPORTS"}
	CompetitionStatuses = []string{"draft", "published", "ongoing", "completed", "cancelled"}
	CompetitionFormats  = []string{"knockout", "round_robin", "league", "group_knockout", "single_elimination", "double_elimination", "staged"}
	MatchStatuses       = []string{"scheduled", "live", "completed", "cancelled", "postponed"}
	Roles               = []string{"participant", "organizer", "admin"}
	UserStatuses        = []string{"active", "suspended"}
	ExperienceLevels    = []string{"Beginner", "Intermediate", "Advanced"}
	ReportTargetTypes   = []string{"competition", "team", "user", "announcement"}
	ReportResolutions   = []string{"resolved", "dismissed"}
)
