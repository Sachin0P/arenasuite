// ArenaSuite: a college competition platform (Sports, Academic, Esports).
//
// Run it with:  go run .
// Settings come from environment variables or a .env file (see .env.example).
package main

import (
	"log"
	"net/http"
	"time"
	_ "time/tzdata" // bundles timezone data, so TIMEZONE works on any server

	"arenasuite/internal/app"
	"arenasuite/internal/config"
	"arenasuite/internal/db"
	"arenasuite/internal/realtime"
	"arenasuite/internal/server"
)

func main() {
	cfg := config.Load()
	if cfg.SessionSecret == "" {
		log.Fatal("SESSION_SECRET is required (any long random string). See .env.example.")
	}
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required, e.g. postgres://user:password@localhost:5432/arenasuite. See .env.example.")
	}
	setTimezone(cfg.Timezone)
	if cfg.DevLogin {
		log.Println("WARNING: DEV_LOGIN is on — anyone can sign in as any user. Never use this in production.")
	}

	database, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	a := &app.App{DB: database, Hub: realtime.NewHub(), Cfg: cfg}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           server.New(a),
		ReadHeaderTimeout: 10 * time.Second,
		// No WriteTimeout: the live-update stream (/events) stays open for a long time.
	}
	log.Printf("ArenaSuite running at %s", cfg.BaseURL)
	log.Fatal(srv.ListenAndServe())
}

// setTimezone makes every time on the site use the college's timezone,
// even when the server itself runs in UTC (as most hosting does).
func setTimezone(name string) {
	loc, err := time.LoadLocation(name)
	if err != nil {
		log.Fatalf("unknown TIMEZONE %q: %v", name, err)
	}
	time.Local = loc
}
