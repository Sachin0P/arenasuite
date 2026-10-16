// Package config reads the app's settings from environment variables.
// A local ".env" file is loaded first (if present) so you don't have to
// export everything by hand during development.
package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port    string // HTTP port to listen on
	BaseURL string // public URL of the site, used to build the Google redirect URL
	DBPath  string // SQLite file location

	SessionSecret string        // signs the login cookie
	SessionTTL    time.Duration // how long a login lasts

	GoogleClientID     string
	GoogleClientSecret string
	AllowedEmailDomain string   // only this Google domain may sign in, e.g. "thapar.edu"
	AdminEmails        []string // emails promoted to admin on login (one-way bootstrap)

	// DevLogin shows a "sign in as any user" list on the login page.
	// It exists so you can try the app without Google. Never enable in production.
	DevLogin bool
}

func Load() Config {
	loadDotEnv(".env")

	days, err := strconv.Atoi(get("SESSION_DAYS", "7"))
	if err != nil || days <= 0 {
		days = 7
	}

	port := get("PORT", "4000")
	cfg := Config{
		Port:               port,
		BaseURL:            strings.TrimRight(get("BASE_URL", "http://localhost:"+port), "/"),
		DBPath:             get("DB_PATH", "./data/arenasuite.db"),
		SessionSecret:      get("SESSION_SECRET", ""),
		SessionTTL:         time.Duration(days) * 24 * time.Hour,
		GoogleClientID:     get("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: get("GOOGLE_CLIENT_SECRET", ""),
		AllowedEmailDomain: strings.ToLower(get("ALLOWED_EMAIL_DOMAIN", "thapar.edu")),
		DevLogin:           get("DEV_LOGIN", "false") == "true",
	}
	for _, e := range strings.Split(get("ADMIN_EMAILS", ""), ",") {
		if e = strings.ToLower(strings.TrimSpace(e)); e != "" {
			cfg.AdminEmails = append(cfg.AdminEmails, e)
		}
	}
	return cfg
}

func get(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

// loadDotEnv reads KEY=VALUE lines from a file into the environment.
// Variables that are already set win, so real env vars override the file.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return // no .env file is fine
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, value)
		}
	}
}
