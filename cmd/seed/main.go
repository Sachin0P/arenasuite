// Command seed fills the database with sample data so you can try the app
// without real Google accounts:
//
//	make db/seed
//
// Then start the server with -dev-login (DEV_LOGIN=true) and pick a user on
// the login page. Running it again is safe: users are upserted, and the
// sample organization and competitions are only created if they don't exist
// yet.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"arenasuite/internal/models"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	var (
		dsn            string
		domain         string
		defaultCollege string
	)

	flag.StringVar(&dsn, "db-dsn", os.Getenv("DATABASE_URL"), "PostgreSQL data source name")
	flag.StringVar(&domain, "allowed-email-domain", envString("ALLOWED_EMAIL_DOMAIN", "thapar.edu"), "Email domain for the sample users")
	flag.StringVar(&defaultCollege, "default-college", envString("DEFAULT_COLLEGE", "Thapar University"), "College of the sample users")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	err := run(dsn, domain, defaultCollege)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
}

func run(dsn, domain, defaultCollege string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err = db.ExecContext(ctx, models.Schema)
	if err != nil {
		return err
	}

	users := []struct{ googleID, name, email, role string }{
		{"seed-admin", "Admin User", "admin@" + domain, "admin"},
		{"seed-organizer", "Olivia Organizer", "organizer@" + domain, "organizer"},
		{"seed-participant-1", "Priya Participant", "priya@" + domain, "participant"},
		{"seed-participant-2", "Raj Participant", "raj@" + domain, "participant"},
		{"seed-participant-3", "Sam Student", "sam@" + domain, "participant"},
		{"seed-participant-4", "Tara Teammate", "tara@" + domain, "participant"},
	}

	// Seed users get a roll number and branch so they skip onboarding.
	stmt := `INSERT INTO users (google_id, name, email, role, college, roll_number, branch)
	VALUES ($1, $2, $3, $4, $5, $6, 'Computer Engineering')
	ON CONFLICT (google_id) DO UPDATE SET name = excluded.name, role = excluded.role
	RETURNING id`

	ids := map[string]int64{}
	for i, u := range users {
		var id int64
		err = db.QueryRow(stmt, u.googleID, u.name, u.email, u.role, defaultCollege, fmt.Sprintf("10230300%d", i+1)).Scan(&id)
		if err != nil {
			return err
		}
		ids[u.googleID] = id
		fmt.Printf("%-12s %-30s id=%d\n", u.role, u.email, id)
	}

	// One organization owned by the organizer, with a competition per category.
	var orgID int64
	err = db.QueryRow(`SELECT id FROM organizations WHERE name = 'Thapar Sports & Tech Club'`).Scan(&orgID)
	if err == nil {
		fmt.Println("\nsample organization already exists")
		fmt.Println("Start the server with DEV_LOGIN=true and sign in as any of these users.")
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	stmt = `INSERT INTO organizations (name, description, college, verified, created_by)
	VALUES ('Thapar Sports & Tech Club', 'Runs the campus leagues, hackathons and LAN nights.', $1, TRUE, $2)
	RETURNING id`

	err = db.QueryRow(stmt, defaultCollege, ids["seed-organizer"]).Scan(&orgID)
	if err != nil {
		return err
	}

	at := func(days int) time.Time { return time.Now().AddDate(0, 0, days).Truncate(time.Hour) }

	competitions := []struct{ name, category, typ, format, venue, mode string }{
		{"Inter-Hostel Football Cup", "SPORTS", "Football", "knockout", "Main Ground", "offline"},
		{"HackTIET 2026", "ACADEMIC", "Hackathon", "staged", "Library Auditorium", "offline"},
		{"Valorant Campus Clash", "ESPORTS", "Valorant", "round_robin", "", "online"},
	}

	stmt = `INSERT INTO competitions (organization_id, name, description, category, type, start_date, end_date,
		registration_deadline, venue, mode, max_teams, format, stages_json, status, created_by)
	VALUES ($1, $2, 'Sample competition created by the seed command.', $3, $4, $5, $6,
		$7, NULLIF($8, ''), $9, 16, $10, NULLIF($11, ''), 'published', $12)`

	for _, c := range competitions {
		stages := ""
		if c.category == "ACADEMIC" {
			stages = `["Registration","Team Formation","Submission","Evaluation","Results"]`
		}

		_, err = db.Exec(stmt, orgID, c.name, c.category, c.typ, at(14), at(16),
			at(10), c.venue, c.mode, c.format, stages, ids["seed-organizer"])
		if err != nil {
			return err
		}
	}

	fmt.Println("\ncreated sample organization and 3 competitions")
	fmt.Println("Start the server with DEV_LOGIN=true and sign in as any of these users.")
	return nil
}

func envString(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
