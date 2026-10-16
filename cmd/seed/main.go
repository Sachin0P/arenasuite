// Command seed fills the database with sample data so you can try the app
// without real Google accounts:
//
//	go run ./cmd/seed
//
// Then start the server with DEV_LOGIN=true and pick a user on the login page.
// Running it again is safe: users are upserted, and the sample organization
// and competitions are only created if they don't exist yet.
package main

import (
	"fmt"
	"log"
	"time"

	"arenasuite/internal/config"
	"arenasuite/internal/db"
)

func main() {
	cfg := config.Load()
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required. See .env.example.")
	}
	conn, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	domain := cfg.AllowedEmailDomain
	users := []struct{ googleID, name, email, role string }{
		{"seed-admin", "Admin User", "admin@" + domain, "admin"},
		{"seed-organizer", "Olivia Organizer", "organizer@" + domain, "organizer"},
		{"seed-participant-1", "Priya Participant", "priya@" + domain, "participant"},
		{"seed-participant-2", "Raj Participant", "raj@" + domain, "participant"},
		{"seed-participant-3", "Sam Student", "sam@" + domain, "participant"},
		{"seed-participant-4", "Tara Teammate", "tara@" + domain, "participant"},
	}
	ids := map[string]int64{}
	for i, u := range users {
		// Seed users get a roll number and branch so they skip onboarding.
		id, err := db.Insert(conn, `
			INSERT INTO users (google_id, name, email, role, college, roll_number, branch)
			VALUES (?, ?, ?, ?, ?, ?, 'Computer Engineering')
			ON CONFLICT (google_id) DO UPDATE SET name = excluded.name, role = excluded.role`,
			u.googleID, u.name, u.email, u.role, cfg.DefaultCollege, fmt.Sprintf("10230300%d", i+1))
		if err != nil {
			log.Fatal(err)
		}
		ids[u.googleID] = id
		fmt.Printf("%-12s %-30s id=%d\n", u.role, u.email, id)
	}

	// One organization owned by the organizer, with a competition per category.
	var orgID int64
	conn.QueryRow(`SELECT id FROM organizations WHERE name = 'Thapar Sports & Tech Club'`).Scan(&orgID)
	if orgID == 0 {
		newID, err := db.Insert(conn, `INSERT INTO organizations (name, description, college, verified, created_by)
			VALUES ('Thapar Sports & Tech Club', 'Runs the campus leagues, hackathons and LAN nights.', ?, TRUE, ?)`,
			cfg.DefaultCollege, ids["seed-organizer"])
		if err != nil {
			log.Fatal(err)
		}
		orgID = newID

		at := func(days int) time.Time { return time.Now().AddDate(0, 0, days).Truncate(time.Hour) }
		comps := []struct{ name, category, typ, format, venue, mode string }{
			{"Inter-Hostel Football Cup", "SPORTS", "Football", "knockout", "Main Ground", "offline"},
			{"HackTIET 2026", "ACADEMIC", "Hackathon", "staged", "Library Auditorium", "offline"},
			{"Valorant Campus Clash", "ESPORTS", "Valorant", "round_robin", "", "online"},
		}
		for _, c := range comps {
			stages := any(nil)
			if c.category == "ACADEMIC" {
				stages = `["Registration","Team Formation","Submission","Evaluation","Results"]`
			}
			_, err := conn.Exec(`
				INSERT INTO competitions (organization_id, name, description, category, type, start_date, end_date,
					registration_deadline, venue, mode, max_teams, format, stages_json, status, created_by)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 16, ?, ?, 'published', ?)`,
				orgID, c.name, "Sample competition created by the seed command.", c.category, c.typ,
				at(14), at(16), at(10), db.NullIfEmpty(c.venue), c.mode, c.format, stages, ids["seed-organizer"])
			if err != nil {
				log.Fatal(err)
			}
		}
		fmt.Println("created sample organization and 3 competitions")
	}
	fmt.Println("\nStart the server with DEV_LOGIN=true and sign in as any of these users.")
}
