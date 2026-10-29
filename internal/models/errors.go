package models

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrNoRecord = errors.New("models: no matching record found")

	// ErrDuplicateName is returned when a team name is already taken in a
	// competition.
	ErrDuplicateName = errors.New("models: duplicate name")

	// ErrDuplicatePost is returned when a user already has a "looking for a
	// team" post for a competition.
	ErrDuplicatePost = errors.New("models: duplicate post")

	// ErrAlreadyOnTeam is returned when a user is already an active member of
	// a team in the same competition (one team per user per competition).
	ErrAlreadyOnTeam = errors.New("models: already on a team in this competition")

	// ErrTeamFull is returned when a team has no free places left.
	ErrTeamFull = errors.New("models: team is full")

	// ErrAlreadyRegistered is returned when the team, or one of its members,
	// already has a live registration for the competition.
	ErrAlreadyRegistered = errors.New("models: already registered")

	// ErrMaxTeamsReached is returned when a competition has no places left.
	ErrMaxTeamsReached = errors.New("models: maximum number of teams reached")

	// ErrFixturesExist, ErrMatchesStarted and ErrNotEnoughTeams are returned
	// by MatchModel.Generate.
	ErrFixturesExist  = errors.New("models: fixtures already exist")
	ErrMatchesStarted = errors.New("models: matches have already started")
	ErrNotEnoughTeams = errors.New("models: not enough approved teams")
)

// isUniqueViolation reports whether err came from a UNIQUE constraint.
// PostgreSQL uses the error code 23505 for this.
func isUniqueViolation(err error) bool {
	var pgError *pgconn.PgError
	return errors.As(err, &pgError) && pgError.Code == "23505"
}
