# ArenaSuite

A competition platform for Thapar University, with three sections: **Sports**,
**Academic** and **Esports**.

**Live site: <https://arenasuite.onrender.com>** (sign in with your
`@thapar.edu` Google account). It runs on a free plan, so if nobody has
visited for a while, the first page load takes about a minute.

- **Students** find competitions, form or join teams, register, and follow
  fixtures and live scores.
- **Organizers** create competitions, approve registrations, generate
  fixtures, enter scores and post announcements.
- **Admins** manage users, verify organizations, hide competitions and handle
  reports.

It's written in Go and stores its data in **PostgreSQL**. The code follows
the structure of Alex Edwards' book *Let's Go* (see [Project layout](#project-layout)).
Besides the standard library it uses [`pgx`](https://github.com/jackc/pgx)
(the Postgres driver), [`scs`](https://github.com/alexedwards/scs) (sessions),
[`alice`](https://github.com/justinas/alice) (middleware chains),
[`form`](https://github.com/go-playground/form) (decoding HTML forms) and
`golang.org/x/time/rate` (rate limiting). Pages are server-rendered with
`html/template`. The browser side is one CSS file, a small script for tabs,
toasts and live updates, and optional eye candy: a [three.js](https://threejs.org)
scene behind every page (a smashable 3D orb on the home page) plus page
animations. It's a rewrite of the earlier Node/Express + Angular version.

## Getting started

You need:

- **Go 1.27 or newer** (check with `go version`)
- **A PostgreSQL database**: either one on your own computer, or a free
  hosted one. Pick one of the options below.

### 1. Get a database

**Option A: PostgreSQL on your Mac (Homebrew)**

```sh
brew install postgresql@17
brew services start postgresql@17      # runs in the background, starts at login
createdb arenasuite
```

If `createdb` isn't found, run `brew link postgresql@17` and open a new
terminal.

Your connection URL is
`postgres://YOUR_MAC_USERNAME@localhost:5432/arenasuite?sslmode=disable`.
Run `whoami` to see your username.

**Option B: Docker (any OS)**

```sh
docker run -d --name arenasuite-db -p 5432:5432 \
  -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=arenasuite postgres:17
```

Your connection URL is
`postgres://postgres:postgres@localhost:5432/arenasuite?sslmode=disable`.

**Option C: A free hosted database (Neon or Supabase)**

Create a project and copy its connection string. This is also how a team
shares one database; see [Sharing a database](#sharing-a-database).

### 2. Configure and run

```sh
git clone https://github.com/Sachin0P/arenasuite.git
cd arenasuite
cp .env.example .env
```

Open `.env` and set:

```sh
DATABASE_URL=<your connection URL from step 1>
DEV_LOGIN=true                          # sign in without Google (local only)
```

Then:

```sh
make db/seed      # optional: sample users, an organization, 3 competitions (asks y/N first)
make run/web      # start the server
```

`make` loads `.env` for you. Without `make`, pass the settings as flags
instead, e.g. `go run ./cmd/web -db-dsn="<URL>" -dev-login`.

Open **http://localhost:4000** (use `localhost`, not `127.0.0.1`). Stop the
server with **Ctrl+C**. Tables are created automatically on the first start.

### Signing in locally

With `DEV_LOGIN=true`, the login page has a **Dev login** box that lets you
sign in as any user in the database without Google.

| Sample user (from `cmd/seed`) | Role |
|---|---|
| Admin User | admin |
| Olivia Organizer | organizer (owns the sample organization) |
| Priya, Raj, Sam, Tara | participants |

Tip: use a normal and a private browser window to be two users at once, for
example an organizer entering scores and a participant watching them update live.

> **Never set `DEV_LOGIN=true` on a public server.** Anyone could sign in as anyone.

## Configuration

Settings are command-line flags of `cmd/web` (run `go run ./cmd/web -help`).
Each flag defaults to an environment variable, which is how Render passes
settings in. Locally, put them in `.env` (copy from `.env.example`) and
start the app with `make run/web`, which exports the file's values.

| Environment variable | Flag | What it does | Default |
|---|---|---|---|
| `DATABASE_URL` | `-db-dsn` | PostgreSQL connection URL. **Required.** Contains the password, so keep it private | none |
| `PORT` | `-port` | Port the server listens on | `4000` |
| `BASE_URL` | `-base-url` | Public address of the site; Google redirects back to `BASE_URL/auth/google/callback` | `http://localhost:4000` |
| `TIMEZONE` | `-timezone` | All times on the site are shown and entered in this zone | `Asia/Kolkata` |
| `SESSION_DAYS` | `-session-lifetime` (e.g. `168h`) | How long a login lasts | `7` days |
| `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` | `-google-client-id` / `-google-client-secret` | Google OAuth client (see below) | none |
| `ALLOWED_EMAIL_DOMAIN` | `-allowed-email-domain` | Only Google accounts on this domain can sign in | `thapar.edu` |
| `DEFAULT_COLLEGE` | `-default-college` | College pre-filled for new users | `Thapar University` |
| `ADMIN_EMAILS` | `-admin-emails` | Comma-separated emails made admin when they sign in. Removing an email later does **not** remove admin; use the Admin Panel for that | empty |
| `DEV_LOGIN` | `-dev-login` | Sign in as any user without Google (local testing only) | `false` |
| `TRUST_PROXY` | `-trust-proxy` | Set to `true` when deployed behind a hosting proxy (Render, Fly, Caddy…) so rate limiting sees each visitor's real address | `false` |
| (none) | `-limiter-rps`, `-limiter-burst`, `-limiter-enabled` | Per-IP rate limit: steady requests per second, burst size, on/off | `5`, `100`, `true` |

### Google sign-in

1. In [Google Cloud Console → Credentials](https://console.cloud.google.com/apis/credentials),
   create an **OAuth client ID** of type **Web application**.
2. Under **Authorized redirect URIs**, add exactly
   `http://localhost:4000/auth/google/callback` (and later
   `https://your-domain/auth/google/callback` for the live site).
3. Put the client ID and client secret in `.env` and restart the server.

If Google shows **`Error 400: redirect_uri_mismatch`**, the address in step 2
doesn't exactly match `BASE_URL` + `/auth/google/callback`. Check http vs
https, the port, a trailing slash, or `127.0.0.1` vs `localhost`.

New users land on their **Profile** page first and must enter a roll number
and branch before using the rest of the site.

## Sharing a database

The database isn't stored in git. To have everyone on the team see the same
data, use one **hosted** Postgres (Neon or Supabase) and put its connection
URL in each person's `.env`. Changes show up for everyone immediately, with
nothing to commit or merge.

Be careful with the URL:

- **Never commit it.** It contains the password. `.env` is already ignored by
  git, so share the URL privately (e.g. a direct message).
- **Don't develop against the live site's database.** One bad test could wipe
  real student data. Give the team a separate test database instead: on
  Neon, create a `dev` **branch** (an instant copy) and share that branch's
  URL; on Supabase, create a second free project. Fill it with
  `make db/seed`.
- If you'd rather keep your experiments private, use a database on your own
  computer (Option A or B above).

## Common commands

| Task | Command |
|---|---|
| List the make targets | `make help` |
| Run the server | `make run/web` |
| Add sample data | `make db/seed` |
| Run the tests | `go test ./...` |
| Format, vet and test everything | `make tidy` then `make audit` |
| Open the local database in a terminal | `psql arenasuite` (for a hosted one: `psql "<connection URL>"`) |
| Start over with an empty local database | stop the server, then `dropdb arenasuite && createdb arenasuite` |
| Build a single program file | `make build/web` then `./bin/web` |
| Build a Docker image | `docker build -t arenasuite .` |

**`address already in use`** means another copy is still running. Stop it, or
change `PORT` in `.env`.
**`cannot reach the database`** means Postgres isn't running, or
`DATABASE_URL` is wrong. Check `brew services list` or `docker ps`.

## Features

- Google sign-in restricted to `@thapar.edu`; roles: participant, organizer, admin
- Home page with Upcoming, Closing Soon, Popular and Recently Added lists
- Sports / Academic / Esports sections and an Explore page with search and filters
- Competition page with tabs: Overview, Teams, Fixtures (bracket or list),
  Standings and Announcements, all updating live
- Teams: create, join requests, invitations, transfer captaincy, remove
  members, leave or disband
- Organizers can set a **maximum number of people per team**; team sizes and
  joins are capped by it
- Find a Team: teams looking for members, and players looking for a team
- Team registration with organizer approval. "Register Individually" creates a
  one-person team for the player, so solo players appear in team lists and fixtures
- Organizers can't take part in their own competitions (registering, teams,
  join requests and invites are all blocked)
- Fixture generation: seeded knockout brackets and round robin
- Live scores and automatic standings (Sports/Esports); direct score entry (Academic)
- Notifications (live badge plus a notifications page)
- Organizer dashboard with a 4-step create/edit competition wizard
- Admin panel: stats, users (role, suspend), organizations (verify),
  competitions (hide), reports

## Project layout

```
cmd/
  web/                   the web application
    main.go              flags, logger, database pool, the application struct
    routes.go            every URL and its middleware chain
    handlers*.go         one handler per page or form action
    helpers.go           serverError, clientError, render, decodePostForm…
    middleware.go        headers, logging, panics, rate limit, auth, CSRF
    templates.go         templateData, the template cache and template functions
    context.go           the signed-in user in the request context
  seed/                  sample data for local testing
internal/
  models/                one file per table: SQL queries, plus schema.sql
  validator/             form validation helpers
  tournament/            pure logic: bracket seeding, round robin, standings (+ tests)
  realtime/              live updates over Server-Sent Events
  auth/                  Google OAuth
  assert/                test helpers
ui/
  html/                  base.tmpl, partials/*.tmpl, pages/*.tmpl
  static/                app.css, app.js (tabs, toasts, live updates),
                         fx.js (three.js scenes), motion.js (animations),
                         vendor/ (three.js r170, MIT)
Makefile                 run, seed, audit and build commands
```

Where to make a change:

- **A rule** ("who can do X", "how many Y"): the handler in `cmd/web/handlers*.go`
- **A query**: the model in `internal/models/`
- **A table or column**: `internal/models/schema.sql`. It only creates missing
  tables, so a new column on an existing table needs an
  `ALTER TABLE ... ADD COLUMN IF NOT EXISTS ...` line there.
- **A page's look**: `ui/html/pages/`
- **Styling**: `ui/static/app.css` (flat colours only, no gradients)
- **The 3D scenes or animations**: `ui/static/fx.js` and `ui/static/motion.js`.
  Both skip themselves when the visitor has "reduce motion" switched on
- **A new URL**: `cmd/web/routes.go`

## How it works

- **Pages and actions.** Pages are plain GET requests rendered with Go
  templates. Every button is an HTML form that POSTs to an action URL. The
  handler checks permissions, calls a model, puts a one-time flash message
  ("Team created") in the session and redirects back, so refreshing never
  re-submits a form.
- **Database.** Each model (e.g. `TeamModel`) runs plain SQL with `$1, $2…`
  placeholders and scans rows into Go structs. Steps that must succeed
  together run in a transaction. Times are stored as exact moments
  (`TIMESTAMPTZ`) and shown in `TIMEZONE`.
- **Sign-in.** Google's OAuth "authorization code" flow, done entirely on the
  server. Sessions are handled by `scs` and stored in the `sessions` table;
  the cookie holds only a random token. The user is re-loaded on every
  request, so suspensions and role changes apply immediately. Cross-site form
  posts are rejected by Go's `http.CrossOriginProtection`.
- **Roles.** New users are participants. Creating an organization (on the
  Organize page) makes you an organizer. Emails in `ADMIN_EMAILS` become
  admins when they sign in.
- **Live updates.** Each browser tab opens one connection to `/events`
  (Server-Sent Events). Personal notifications go only to that user; scores,
  standings and announcements go to everyone viewing that competition.
- **Fixtures.** Knockout formats use a seeded bracket (8 slots seed as
  `[1,8,4,5,2,7,3,6]`), so the top seeds get the byes. Other formats use round
  robin (the circle method). Completing a match picks the winner from the
  scores, moves them into the next bracket match and recalculates the
  standings: 3 points for a win, 1 for a draw, ranked by points, then score
  difference, then wins.

## Deployment

The live site runs on two free services:

| Part | Service | Details |
|---|---|---|
| App | [Render](https://render.com) web service `arenasuite` | Free plan, Singapore region, built from the `Dockerfile` using `render.yaml` |
| Database | [Neon](https://neon.tech) Postgres | Free plan, Singapore region |

### Updating the live site

- **Code changes:** push to `main`. Render rebuilds and redeploys
  automatically in a few minutes. Watch progress under **Events** and
  **Logs** on the Render service page.
- **Database changes:** edit `internal/models/schema.sql`. It runs on every
  start, so new tables appear on the next deploy. A new column on an existing
  table needs an `ALTER TABLE ... ADD COLUMN IF NOT EXISTS ...` line.
- **Settings** (`ADMIN_EMAILS`, Google keys, …): change them on Render under
  **Environment**. Saving restarts the app. Don't put production values in
  git.
- **Check it's up:** <https://arenasuite.onrender.com/healthz> shows `ok`
  when the app is running and can reach the database.
- **Logs** are structured `key=value` lines (Go's `log/slog`), e.g.
  `level=ERROR msg="..." method=POST uri=/teams/3/update`. Search the Render
  logs for `level=ERROR` to find problems.

### Upgrading the live site to the Let's Go version (one time)

The October 2026 refactor (new `cmd/web` layout, database sessions) needs a
few things on the live site. Nothing else changes: the Dockerfile still
produces one binary, `/healthz` and the Google callback URL are the same, and
the existing data is untouched.

1. **Push the code to `main`.** Render builds `./cmd/web` from the
   Dockerfile automatically; no build settings change.
2. **Nothing to do in Neon.** On its first start the app creates the new
   `sessions` table (all other tables already exist).
3. **Everyone is signed out once.** Logins now live in the `sessions` table
   instead of a signed cookie, so old cookies are ignored and people just
   sign in again with Google.
4. **Optional cleanup on Render:** delete the `SESSION_SECRET` environment
   variable under **Environment**. The app no longer reads it, and removing
   it from `render.yaml` doesn't remove it from an existing service. Leaving
   it there is harmless.
5. **Keep `TRUST_PROXY=true`.** The rate limiter (5 requests a second per
   visitor, bursts up to 100) needs it to see each visitor's real address;
   without it every visitor shares Render's proxy address and one limit.
6. **Check it worked:** open `/healthz`, sign in, and look for
   `level=INFO msg="starting server"` (and no `level=ERROR`) in the Render
   logs.

If something goes wrong, use **Rollback** on the Render service page to go
back to the previous deploy. The extra `sessions` table doesn't affect the
old version.

### Looking after the data

- The live database's connection string is only stored in Render (as
  `DATABASE_URL`) and in Neon's dashboard. It contains the password, so
  never commit it or paste it in chats. If it leaks, reset the password in
  Neon (**Roles → neondb_owner → Reset password**) and update `DATABASE_URL`
  on Render.
- **Backups:** Neon can restore the database to an earlier point in time
  (limited history on the free plan). For your own copy:
  `pg_dump "<connection URL>" > backup.sql`.
- **Sessions** are rows in the `sessions` table. Expired ones are deleted
  every few minutes while the app runs. To sign everyone out at once, run
  `DELETE FROM sessions;` in the Neon SQL editor.
- Don't run `make db/seed` or experiments against the live database.
  Use a local database or a Neon `dev` branch instead (see
  [Sharing a database](#sharing-a-database)).

### Setting it up from scratch

Follow these steps if you ever need to recreate the deployment (for example,
in a new account).

**1. Database (Neon).** Create a project in the **Singapore** region. On the
dashboard click **Connect**, turn **Connection pooling off**, and copy the
connection string (`postgresql://...neon.tech/neondb?sslmode=require...`).

**2. App (Render).** Sign up with GitHub and allow access to this repository.
Click **New → Blueprint** and pick the repository. Render reads
`render.yaml` and asks for:

- `DATABASE_URL`: the Neon connection string
- `BASE_URL`: the site's address, e.g. `https://arenasuite.onrender.com`
  (no trailing slash; fix it under **Environment** if Render picks a
  different name)
- `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET`
- `ADMIN_EMAILS`: who should be admin after their first sign-in

Render sets `PORT` itself.
`render.yaml` also sets `TRUST_PROXY=true` and `DEV_LOGIN=false`.

**3. Google.** In Google Cloud Console → your OAuth client →
**Authorized redirect URIs**, add `<BASE_URL>/auth/google/callback`.

**4.** Open the site and sign in. Tables (including `sessions`) are created
on the first start.

### Other hosts

The app must run as **one always-on instance**, because live updates are kept
in its memory and use long-lived connections. (Logins are stored in the
database, so a restart doesn't sign anyone out.) Railway, Fly.io, Google Cloud
Run (max instances 1, 60-minute timeout) or any Linux VPS with Caddy for
HTTPS all work. Set `TRUST_PROXY=true` on them. **Not suitable:** Vercel,
Netlify and AWS Lambda, which cut the long connections live updates need.
