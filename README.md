# ArenaSuite

A competition platform for Thapar University, with three sections: **Sports**,
**Academic** and **Esports**.

- **Students** find competitions, form or join teams, register, and follow
  fixtures and live scores.
- **Organizers** create competitions, approve registrations, generate
  fixtures, enter scores and post announcements.
- **Admins** manage users, verify organizations, hide competitions and handle
  reports.

It's written in Go using only the standard library plus one package,
[`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite) (a pure-Go
SQLite driver, so no C compiler is needed). Pages are server-rendered with
`html/template`, with one CSS file and one small JavaScript file. It's a
rewrite of the earlier Node/Express + Angular version.

## Getting started

You need **Go 1.27 or newer** (`go version` to check). The `sqlite3` command
line tool is optional but handy.

```sh
git clone https://github.com/Sachin0P/arenasuite.git
cd arenasuite
cp .env.example .env
```

Open `.env` and change two values:

```sh
SESSION_SECRET=<any long random text>   # e.g. the output of: openssl rand -hex 32
DEV_LOGIN=true                          # lets you sign in without Google (local only)
```

Then start the server:

```sh
go run .
```

Open **http://localhost:4000** (use `localhost`, not `127.0.0.1`). Stop the
server with **Ctrl+C**.

### Signing in locally

With `DEV_LOGIN=true`, the login page has a **Dev login** box that lets you
sign in as any user in the database without Google. To add sample users, an
organization and three competitions (one per category), run:

```sh
go run ./cmd/seed
```

| Sample user | Role |
|---|---|
| Admin User | admin |
| Olivia Organizer | organizer (owns the sample organization) |
| Priya, Raj, Sam, Tara | participants |

Tip: use a normal and a private browser window to be two users at once, for
example an organizer entering scores and a participant watching them update live.

> **Never set `DEV_LOGIN=true` on a public server.** Anyone could sign in as anyone.

## Configuration

All settings live in `.env` (copy from `.env.example`). Real environment
variables override the file.

| Setting | What it does | Default |
|---|---|---|
| `PORT` | Port the server listens on | `4000` |
| `BASE_URL` | Public address of the site; Google redirects back to `BASE_URL/auth/google/callback` | `http://localhost:4000` |
| `DB_PATH` | SQLite database file | `./data/arenasuite.db` |
| `SESSION_SECRET` | Signs the login cookie. **Required.** Use a different value per server | none |
| `SESSION_DAYS` | How long a login lasts | `7` |
| `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` | Google OAuth client (see below) | none |
| `ALLOWED_EMAIL_DOMAIN` | Only Google accounts on this domain can sign in | `thapar.edu` |
| `DEFAULT_COLLEGE` | College pre-filled for new users | `Thapar University` |
| `ADMIN_EMAILS` | Comma-separated emails made admin when they sign in. Removing an email later does **not** remove admin; use the Admin Panel for that | empty |
| `DEV_LOGIN` | Sign in as any user without Google (local testing only) | `false` |

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

## The shared database

The database file `data/arenasuite.db` **is committed to git**, so everyone
who pulls gets the same data. Keep these in mind:

- **Before committing it**, stop the server and run:
  ```sh
  sqlite3 data/arenasuite.db "PRAGMA wal_checkpoint(TRUNCATE);"
  ```
  While the server runs, SQLite keeps recent changes in `arenasuite.db-wal`
  (which git ignores). This command moves them into the main file so they
  actually get committed.
- **Git can't merge database changes.** If two people both change data and
  commit, one overwrites the other. Pulling replaces your local data with the
  version on GitHub. Agree on who commits the database, or commit it rarely.
- **It contains real student data**: names, emails, roll numbers and branches of
  everyone who has signed in. Think about this before making the repository
  public.
- To start over with an empty database, stop the server and delete the
  `data/` folder. A fresh database is created on the next start.

## Common commands

| Task | Command |
|---|---|
| Run the server | `go run .` |
| Add sample data | `go run ./cmd/seed` |
| Run the tests | `go test ./...` |
| Build a single program file | `go build -o arenasuite .` then `./arenasuite` |
| Build a Docker image | `docker build -t arenasuite .` (database lives in `/data`) |

**`address already in use`** means another copy is still running. Stop it, or
change `PORT` in `.env`.

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
- Individual and team registration with organizer approval
- Fixture generation: seeded knockout brackets and round robin
- Live scores and automatic standings (Sports/Esports); direct score entry (Academic)
- Notifications (live badge plus a notifications page)
- Organizer dashboard with a 4-step create/edit competition wizard
- Admin panel: stats, users (role, suspend), organizations (verify),
  competitions (hide), reports

## Project layout

```
main.go                  starts the server
cmd/seed/                sample data for local testing
data/arenasuite.db       the shared database (committed)
internal/
  config/                settings from env / .env
  db/                    opens SQLite, schema.sql, All/One query helpers
  app/                   all the rules: permissions, limits, notifications…
  tournament/            pure logic: bracket seeding, round robin, standings (+ tests)
  realtime/              live updates over Server-Sent Events
  auth/                  Google OAuth and the signed login cookie
  server/                URLs → handlers → templates
web/
  templates/             layout.html, partials.html, pages/*.html
  static/                app.css, app.js
```

Where to make a change:

- **A rule** ("who can do X", "how many Y"): `internal/app/`
- **A page's look**: `web/templates/pages/`
- **Styling**: `web/static/app.css`
- **A new URL**: `internal/server/server.go` (`routes`)

## How it works

- **Pages and actions.** Pages are plain GET requests rendered with Go
  templates. Every button is an HTML form that POSTs to an action URL. The
  action calls `internal/app`, sets a one-time message ("Team created") and
  redirects back, so refreshing never re-submits a form.
- **Sign-in.** Google's OAuth "authorization code" flow, done entirely on the
  server. The login cookie holds the user id plus an HMAC signature, and the
  user is re-loaded on every request, so suspensions and role changes apply
  immediately.
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

## Deploying

The server must run as **one always-on instance with a disk that persists**,
because the database is a file and live updates are kept in memory.

- **Good fits:** Fly.io (with a volume), Railway or Render (with a persistent
  disk mounted at `/data`), or any Linux VPS with Caddy in front for HTTPS.
- **Not suitable:** Vercel, Netlify, AWS Lambda, Google Cloud Run and Heroku.
  They don't keep files between restarts, and they cut the long connections
  live updates need.

Before going live:

- Set `BASE_URL=https://your-domain`, a new `SESSION_SECRET`, and `DEV_LOGIN=false`.
- Add `https://your-domain/auth/google/callback` to the Google OAuth client.
- Back up the database file regularly.
- **Known issue:** the rate limiter (300 requests/minute per visitor) uses the
  connection's address. Behind a hosting proxy every visitor shares the
  proxy's address, so it needs to read `X-Forwarded-For` first. See
  `clientIP` in `internal/server/ratelimit.go`.
