package main

import (
	"database/sql"
	"fmt"
)

var (
	fakeFirstNames = []string{
		"Aarav", "Ananya", "Arjun", "Diya", "Farhan", "Gurleen", "Harsh", "Ishita",
		"Jasleen", "Karan", "Kavya", "Manav", "Meera", "Nikhil", "Pooja", "Rohan",
		"Saanvi", "Simran", "Tanish", "Uday", "Vanya", "Varun", "Yash", "Zoya",
	}
	fakeLastNames = []string{
		"Agarwal", "Bansal", "Bhatia", "Chauhan", "Dhillon", "Gill", "Grewal", "Gupta",
		"Iyer", "Jain", "Kapoor", "Khanna", "Malhotra", "Menon", "Nair", "Rao",
		"Sandhu", "Sharma", "Singh", "Verma",
	}
	fakeBranches = []string{"COE", "COPC", "COBS", "DSAI", "ENC", "ECE", "EEC", "RAI", "ELE", "MEE", "CIE", "CHE"}
)

// fakeUser holds one generated test account. An empty rollNumber and branch
// mean the profile is incomplete, so the user is sent through onboarding.
type fakeUser struct {
	googleID, name, email, role, rollNumber, branch, status string
}

// newFakeUser builds the i-th test account (counting from 1). The same i
// always gives the same user, so running the command again updates the
// accounts instead of adding duplicates. The "fake.user" email addresses can
// never belong to a real student, and they make the accounts easy to find.
func newFakeUser(i int, domain string) fakeUser {
	u := fakeUser{
		googleID:   fmt.Sprintf("fake-user-%03d", i),
		name:       fakeFirstNames[(i-1)%len(fakeFirstNames)] + " " + fakeLastNames[(i-1)*7%len(fakeLastNames)],
		email:      fmt.Sprintf("fake.user%03d@%s", i, domain),
		role:       "participant",
		rollNumber: fmt.Sprintf("10239%04d", i),
		branch:     fakeBranches[(i-1)%len(fakeBranches)],
		status:     "active",
	}

	// Most accounts are ordinary participants. A few cover the other cases
	// worth testing. No fake user is ever an admin.
	if i%10 == 0 {
		u.role = "organizer"
	}
	if i%12 == 0 {
		u.status = "suspended"
	}
	if i%9 == 0 {
		u.rollNumber, u.branch = "", ""
	}

	return u
}

// seedFakeUsers upserts n test accounts in a single transaction, so a failure
// part way through leaves the database unchanged.
func seedFakeUsers(db *sql.DB, n int, domain, college string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt := `INSERT INTO users (google_id, name, email, role, college, roll_number, branch, status)
	VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), NULLIF($7, ''), $8)
	ON CONFLICT (google_id) DO UPDATE SET name = excluded.name, email = excluded.email, role = excluded.role,
		college = excluded.college, roll_number = excluded.roll_number, branch = excluded.branch,
		status = excluded.status, updated_at = now()`

	for i := 1; i <= n; i++ {
		u := newFakeUser(i, domain)

		_, err = tx.Exec(stmt, u.googleID, u.name, u.email, u.role, college, u.rollNumber, u.branch, u.status)
		if err != nil {
			return err
		}
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	fmt.Printf("%d fake users are ready (fake.user001@%s onwards)\n", n, domain)
	fmt.Println("Start the server with DEV_LOGIN=true and sign in as any of these users.")
	return nil
}
