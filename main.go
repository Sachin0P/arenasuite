// ArenaSuite: a college competition platform (Sports, Academic, Esports).
//
// Run it with:  go run .
// Settings come from environment variables or a .env file (see .env.example).
package main

import (
	"log"
	"net/http"
	"time"

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
	if cfg.DevLogin {
		log.Println("WARNING: DEV_LOGIN is on — anyone can sign in as any user. Never use this in production.")
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
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
