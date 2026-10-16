# ArenaSuite (Go)

A college competition platform for Sports, Academic and Esports events.
Students find competitions, form teams, register, and follow live scores.
Organizers run the events, and admins moderate the platform.

This is a Go rewrite of the original Node/Express + Angular app. It uses the
Go standard library, server-rendered HTML (`html/template`), one CSS file and
one small JavaScript file. The only dependency is
[`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite), a pure-Go SQLite
driver, so no C compiler is needed.

## Run it

```sh
cp .env.example .env          # then edit SESSION_SECRET (and Google keys if you have them)
go run ./cmd/seed             # optional: sample users, an organization, 3 competitions
DEV_LOGIN=true go run .       # http://localhost:4000
```

`DEV_LOGIN=true` adds a "sign in as any user" box to the login page so you can
try every role without Google. **Never enable it in production.**

```sh
go test ./...                 # fixture and standings tests
go build -o arenasuite .      # one binary; templates and CSS are embedded
docker build -t arenasuite .  # or as a container (data lives in /data)
```

### Google sign-in

Create an OAuth client of type "Web application" in Google Cloud Console.
Add `BASE_URL/auth/google/callback` as an authorized redirect URI, then put
the client ID and secret in `.env`. Only verified accounts on
`ALLOWED_EMAIL_DOMAIN` can sign in.

## Project layout

```
main.go                  starts the server
cmd/seed/                sample data for local testing
internal/
  config/                settings from env / .env
  db/                    opens SQLite, schema.sql, All/One query helpers
  app/                   all the rules (permissions, registration limits, notifications…)
  tournament/            pure logic: bracket seeding, round robin, standings (+ tests)
  realtime/              live updates over Server-Sent Events
  auth/                  Google OAuth and the signed login cookie
  server/                URLs → handlers → templates
web/
  templates/             layout.html, partials.html, pages/*.html
  static/                app.css, app.js
```

## How it works, briefly

- **Pages and actions.** Pages are plain GET requests rendered with Go
  templates. Every button is an HTML form that POSTs to an action URL. The
  action calls `internal/app`, sets a one-time flash message and redirects
  back (Post/Redirect/Get).
- **Sign-in.** Google OAuth "authorization code" flow, done on the server.
  The session is a cookie holding the user id plus an HMAC signature. The user
  is re-loaded on every request, so suspensions and role changes apply
  immediately.
- **Roles.** New users are participants. Creating an organization makes you
  an organizer. Emails in `ADMIN_EMAILS` become admins on login. Users must
  add a roll number and branch before using signed-in pages.
- **Live updates.** Each tab opens one `EventSource` to `/events`. Personal
  notifications go to a per-user room. Scores, standings and announcements go
  to everyone viewing that competition. The competition page re-fetches its
  live sections when an event arrives.
- **Fixtures.** Knockout formats use a seeded bracket (`[1,8,4,5,2,7,3,6]`
  for 8), so the top seeds get the byes. Other formats use round robin (the
  circle method). Completing a match picks the winner from the scores, moves
  them into the next bracket match and recalculates the standings (3 points
  for a win, 1 for a draw). Academic competitions use scores the organizer
  enters directly.
