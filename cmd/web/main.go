// ArenaSuite: a college competition platform (Sports, Academic, Esports).
package main

import (
	"context"
	"database/sql"
	"flag"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // bundles timezone data, so -timezone works on any server

	"arenasuite/internal/auth"
	"arenasuite/internal/models"
	"arenasuite/internal/realtime"

	"github.com/alexedwards/scs/postgresstore"
	"github.com/alexedwards/scs/v2"
	"github.com/go-playground/form/v4"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver with database/sql
)

// config holds the settings, which come from command-line flags. Each flag
// defaults to an environment variable, because that's how the hosting
// platform (Render) passes settings in.
type config struct {
	port     int
	baseURL  string
	timezone string
	db       struct {
		dsn string
	}
	session struct {
		lifetime time.Duration
	}
	google struct {
		clientID     string
		clientSecret string
	}
	allowedEmailDomain string
	defaultCollege     string
	adminEmails        []string
	devLogin           bool
	limiter            struct {
		rps        float64
		burst      int
		enabled    bool
		trustProxy bool
	}
}

// application holds the dependencies for the HTTP handlers, helpers and
// middleware.
type application struct {
	config         config
	logger         *slog.Logger
	templateCache  map[string]*template.Template
	formDecoder    *form.Decoder
	sessionManager *scs.SessionManager
	google         *auth.Google
	hub            *realtime.Hub
	users          *models.UserModel
	organizations  *models.OrganizationModel
	competitions   *models.CompetitionModel
	teams          *models.TeamModel
	joinRequests   *models.JoinRequestModel
	invitations    *models.InvitationModel
	registrations  *models.RegistrationModel
	matches        *models.MatchModel
	standings      *models.StandingModel
	announcements  *models.AnnouncementModel
	notifications  *models.NotificationModel
	posts          *models.PostModel
	reports        *models.ReportModel
	stats          *models.StatsModel
}

func main() {
	var cfg config

	flag.IntVar(&cfg.port, "port", envInt("PORT", 4000), "HTTP network port")
	flag.StringVar(&cfg.baseURL, "base-url", os.Getenv("BASE_URL"), "Public URL of the site (default http://localhost:<port>)")
	flag.StringVar(&cfg.timezone, "timezone", envString("TIMEZONE", "Asia/Kolkata"), "Times are shown and entered in this zone")
	flag.StringVar(&cfg.db.dsn, "db-dsn", os.Getenv("DATABASE_URL"), "PostgreSQL data source name")
	flag.DurationVar(&cfg.session.lifetime, "session-lifetime", time.Duration(envInt("SESSION_DAYS", 7))*24*time.Hour, "How long a login lasts")
	flag.StringVar(&cfg.google.clientID, "google-client-id", os.Getenv("GOOGLE_CLIENT_ID"), "Google OAuth client ID")
	flag.StringVar(&cfg.google.clientSecret, "google-client-secret", os.Getenv("GOOGLE_CLIENT_SECRET"), "Google OAuth client secret")
	flag.StringVar(&cfg.allowedEmailDomain, "allowed-email-domain", envString("ALLOWED_EMAIL_DOMAIN", "thapar.edu"), "Only this Google domain may sign in")
	flag.StringVar(&cfg.defaultCollege, "default-college", envString("DEFAULT_COLLEGE", "Thapar University"), "College pre-filled for new users")
	flag.BoolVar(&cfg.devLogin, "dev-login", envBool("DEV_LOGIN", false), "Show a 'sign in as any user' list (never in production)")
	flag.Float64Var(&cfg.limiter.rps, "limiter-rps", 5, "Rate limiter maximum requests per second")
	flag.IntVar(&cfg.limiter.burst, "limiter-burst", 100, "Rate limiter maximum burst")
	flag.BoolVar(&cfg.limiter.enabled, "limiter-enabled", true, "Enable rate limiter")
	flag.BoolVar(&cfg.limiter.trustProxy, "trust-proxy", envBool("TRUST_PROXY", false), "Read the client IP from X-Forwarded-For (behind a hosting proxy)")

	// Use flag.Func() for the list of admin emails. ADMIN_EMAILS is comma
	// separated, so the same format is used for the flag.
	cfg.adminEmails = splitEmails(os.Getenv("ADMIN_EMAILS"))
	flag.Func("admin-emails", "Comma-separated emails promoted to admin on sign-in", func(val string) error {
		cfg.adminEmails = splitEmails(val)
		return nil
	})

	flag.Parse()

	if cfg.baseURL == "" {
		cfg.baseURL = "http://localhost:" + strconv.Itoa(cfg.port)
	}
	cfg.baseURL = strings.TrimRight(cfg.baseURL, "/")
	cfg.allowedEmailDomain = strings.ToLower(cfg.allowedEmailDomain)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Make every time on the site use the college's timezone, even when the
	// server itself runs in UTC (as most hosting does).
	loc, err := time.LoadLocation(cfg.timezone)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
	time.Local = loc

	if cfg.devLogin {
		logger.Warn("dev login is on: anyone can sign in as any user, never use this in production")
	}

	db, err := openDB(cfg.db.dsn)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
	defer db.Close()

	logger.Info("database connection pool established")

	templateCache, err := newTemplateCache()
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}

	formDecoder := form.NewDecoder()

	// Sessions are stored in PostgreSQL, so logins survive a restart or
	// redeploy. Only a random token goes in the cookie.
	sessionManager := scs.New()
	sessionManager.Store = postgresstore.New(db)
	sessionManager.Lifetime = cfg.session.lifetime
	sessionManager.Cookie.Secure = strings.HasPrefix(cfg.baseURL, "https://")

	app := &application{
		config:         cfg,
		logger:         logger,
		templateCache:  templateCache,
		formDecoder:    formDecoder,
		sessionManager: sessionManager,
		google:         auth.NewGoogle(cfg.google.clientID, cfg.google.clientSecret, cfg.baseURL+"/auth/google/callback"),
		hub:            realtime.NewHub(),
		users:          &models.UserModel{DB: db},
		organizations:  &models.OrganizationModel{DB: db},
		competitions:   &models.CompetitionModel{DB: db},
		teams:          &models.TeamModel{DB: db},
		joinRequests:   &models.JoinRequestModel{DB: db},
		invitations:    &models.InvitationModel{DB: db},
		registrations:  &models.RegistrationModel{DB: db},
		matches:        &models.MatchModel{DB: db},
		standings:      &models.StandingModel{DB: db},
		announcements:  &models.AnnouncementModel{DB: db},
		notifications:  &models.NotificationModel{DB: db},
		posts:          &models.PostModel{DB: db},
		reports:        &models.ReportModel{DB: db},
		stats:          &models.StatsModel{DB: db},
	}

	srv := &http.Server{
		Addr:     ":" + strconv.Itoa(cfg.port),
		Handler:  app.routes(),
		ErrorLog: slog.NewLogLogger(logger.Handler(), slog.LevelError),
		// The live-updates stream (/events) clears its own write deadline, so
		// the usual timeouts can stay in place for every other request.
		IdleTimeout:  time.Minute,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	logger.Info("starting server", "addr", srv.Addr, "url", cfg.baseURL)

	err = srv.ListenAndServe()
	logger.Error(err.Error())
	os.Exit(1)
}

// openDB returns a sql.DB connection pool for the given DSN, and creates any
// missing tables.
func openDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}

	// Free hosted databases allow only a few connections, so keep the pool small.
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxIdleTime(5 * time.Minute)

	// A hosted database that has been asleep can take a few seconds to wake
	// up, so allow a generous timeout for the first connection.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err = db.PingContext(ctx)
	if err != nil {
		db.Close()
		return nil, err
	}

	_, err = db.ExecContext(ctx, models.Schema)
	if err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

// envString, envInt and envBool read flag defaults from the environment.
func envString(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(key))
	if err != nil {
		return fallback
	}
	return value
}

func envBool(key string, fallback bool) bool {
	value, err := strconv.ParseBool(os.Getenv(key))
	if err != nil {
		return fallback
	}
	return value
}

func splitEmails(s string) []string {
	var emails []string
	for _, e := range strings.Split(s, ",") {
		if e = strings.ToLower(strings.TrimSpace(e)); e != "" {
			emails = append(emails, e)
		}
	}
	return emails
}
