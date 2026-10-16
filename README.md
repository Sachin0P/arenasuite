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

It's written in Go and stores its data in **PostgreSQL**. It uses the Go
standard library plus one package: [`pgx`](https://github.com/jackc/pgx), the
Postgres driver. Pages are server-rendered with `html/template`, with one CSS
file and one small JavaScript file. It's a rewrite of the earlier
Node/Express + Angular version.

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
SESSION_SECRET=<any long random text>   # e.g. the output of: openssl rand -hex 32
DEV_LOGIN=true                          # sign in without Google (local only)
```

Then:

```sh
go run ./cmd/seed     # optional: sample users, an organization, 3 competitions
go run .              # start the server
```

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

All settings live in `.env` (copy from `.env.example`). Real environment
variables override the file.

| Setting | What it does | Default |
|---|---|---|
| `DATABASE_URL` | PostgreSQL connection URL. **Required.** Contains the password, so keep it private | none |
| `SESSION_SECRET` | Signs the login cookie. **Required.** Use a different value per server | none |
| `PORT` | Port the server listens on | `4000` |
| `BASE_URL` | Public address of the site; Google redirects back to `BASE_URL/auth/google/callback` | `http://localhost:4000` |
| `TIMEZONE` | All times on the site are shown and entered in this zone | `Asia/Kolkata` |
| `SESSION_DAYS` | How long a login lasts | `7` |
| `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` | Google OAuth client (see below) | none |
| `ALLOWED_EMAIL_DOMAIN` | Only Google accounts on this domain can sign in | `thapar.edu` |
| `DEFAULT_COLLEGE` | College pre-filled for new users | `Thapar University` |
| `ADMIN_EMAILS` | Comma-separated emails made admin when they sign in. Removing an email later does **not** remove admin; use the Admin Panel for that | empty |
| `DEV_LOGIN` | Sign in as any user without Google (local testing only) | `false` |
| `TRUST_PROXY` | Set to `true` when deployed behind a hosting proxy (Render, Fly, Caddy…) so rate limiting sees each visitor's real address | `false` |

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
  `go run ./cmd/seed`.
- If you'd rather keep your experiments private, use a database on your own
  computer (Option A or B above).

## Common commands

| Task | Command |
|---|---|
| Run the server | `go run .` |
| Add sample data | `go run ./cmd/seed` |
| Run the tests | `go test ./...` |
| Open the local database in a terminal | `psql arenasuite` (for a hosted one: `psql "<connection URL>"`) |
| Start over with an empty local database | stop the server, then `dropdb arenasuite && createdb arenasuite` |
| Build a single program file | `go build -o arenasuite .` then `./arenasuite` |
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
main.go                  starts the server
cmd/seed/                sample data for local testing
internal/
  config/                settings from env / .env
  db/                    connects to Postgres, schema.sql, query helpers
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
- **A table or column**: `internal/db/schema.sql`. It only creates missing
  tables, so a new column on an existing table needs an
  `ALTER TABLE ... ADD COLUMN IF NOT EXISTS ...` line there.
- **A page's look**: `web/templates/pages/`
- **Styling**: `web/static/app.css`
- **A new URL**: `internal/server/server.go` (`routes`)

## How it works

- **Pages and actions.** Pages are plain GET requests rendered with Go
  templates. Every button is an HTML form that POSTs to an action URL. The
  action calls `internal/app`, sets a one-time message ("Team created") and
  redirects back, so refreshing never re-submits a form.
- **Database.** Queries are written with `?` placeholders; `internal/db`
  converts them to Postgres's `$1, $2…` and copies rows into Go structs by
  their `db:"column"` tags. Times are stored as exact moments (`TIMESTAMPTZ`)
  and shown in `TIMEZONE`.
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
- **Database changes:** edit `internal/db/schema.sql`. It runs on every
  start, so new tables appear on the next deploy. A new column on an existing
  table needs an `ALTER TABLE ... ADD COLUMN IF NOT EXISTS ...` line.
- **Settings** (`ADMIN_EMAILS`, Google keys, …): change them on Render under
  **Environment**. Saving restarts the app. Don't put production values in
  git.
- **Check it's up:** <https://arenasuite.onrender.com/healthz> shows `ok`
  when the app is running and can reach the database.

### Looking after the data

- The live database's connection string is only stored in Render (as
  `DATABASE_URL`) and in Neon's dashboard. It contains the password, so
  never commit it or paste it in chats. If it leaks, reset the password in
  Neon (**Roles → neondb_owner → Reset password**) and update `DATABASE_URL`
  on Render.
- **Backups:** Neon can restore the database to an earlier point in time
  (limited history on the free plan). For your own copy:
  `pg_dump "<connection URL>" > backup.sql`.
- Don't run `go run ./cmd/seed` or experiments against the live database.
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

Render generates `SESSION_SECRET` itself and sets `PORT`.
`render.yaml` also sets `TRUST_PROXY=true` and `DEV_LOGIN=false`.

**3. Google.** In Google Cloud Console → your OAuth client →
**Authorized redirect URIs**, add `<BASE_URL>/auth/google/callback`.

**4.** Open the site and sign in. Tables are created on the first start.

### Other hosts

The app must run as **one always-on instance**, because live updates are kept
in its memory and use long-lived connections. Railway, Fly.io, Google Cloud
Run (max instances 1, 60-minute timeout) or any Linux VPS with Caddy for
HTTPS all work. Set `TRUST_PROXY=true` on them. **Not suitable:** Vercel,
Netlify and AWS Lambda, which cut the long connections live updates need.
