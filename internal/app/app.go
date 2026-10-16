// Package app contains all of ArenaSuite's rules: who may do what, and what
// happens in the database when they do it. The HTTP layer (package server)
// only reads forms, calls these methods and renders pages.
package app

import (
	"database/sql"
	"fmt"
	"slices"

	"arenasuite/internal/config"
	"arenasuite/internal/db"
	"arenasuite/internal/realtime"
)

type App struct {
	DB  *sql.DB
	Hub *realtime.Hub
	Cfg config.Config
}

// Error is a "normal" failure the user should see, like "This team is full".
// Status is the matching HTTP status code.
type Error struct {
	Status int
	Msg    string
}

func (e *Error) Error() string { return e.Msg }

func fail(status int, format string, args ...any) error {
	return &Error{Status: status, Msg: fmt.Sprintf(format, args...)}
}

var (
	Categories          = []string{"SPORTS", "ACADEMIC", "ESPORTS"}
	CompetitionStatuses = []string{"draft", "published", "ongoing", "completed", "cancelled"}
	CompetitionFormats  = []string{"knockout", "round_robin", "league", "group_knockout", "single_elimination", "double_elimination", "staged"}
	MatchStatuses       = []string{"scheduled", "live", "completed", "cancelled", "postponed"}
	Roles               = []string{"participant", "organizer", "admin"}
)

// mustBeOneOf returns a 400 error if value isn't in allowed.
func mustBeOneOf(value string, allowed []string, field string) error {
	if !slices.Contains(allowed, value) {
		return fail(400, "Invalid %s. Must be one of: %v", field, allowed)
	}
	return nil
}

// inTx runs fn inside a transaction: everything is saved if fn returns nil,
// and nothing is saved if it returns an error.
func (a *App) inTx(fn func(tx *sql.Tx) error) error {
	tx, err := a.DB.Begin()
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// Notify saves a notification for a user and pushes it live to any open tab,
// so it shows up now and is still there if they weren't online.
func (a *App) Notify(userID int64, kind, message string, referenceID int64) {
	res, err := a.DB.Exec(
		`INSERT INTO notifications (user_id, type, message, reference_id) VALUES (?, ?, ?, ?)`,
		userID, kind, message, db.NullIfZero(referenceID))
	if err != nil {
		return
	}
	id, _ := res.LastInsertId()
	if n, _ := db.One[Notification](a.DB, `SELECT * FROM notifications WHERE id = ?`, id); n != nil {
		a.Hub.Send(realtime.UserRoom(userID), "notification", n)
	}
}

// toCompetition pushes a live event to everyone viewing a competition page.
func (a *App) toCompetition(competitionID int64, event string, payload any) {
	a.Hub.Send(realtime.CompetitionRoom(competitionID), event, payload)
}

// activeMemberIDs lists the user ids of a team's active members.
func activeMemberIDs(q db.Querier, teamID int64) ([]int64, error) {
	rows, err := db.All[TeamMember](q, `SELECT * FROM team_members WHERE team_id = ? AND status = 'active'`, teamID)
	ids := make([]int64, len(rows))
	for i, m := range rows {
		ids[i] = m.UserID
	}
	return ids, err
}
