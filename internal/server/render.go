package server

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"arenasuite/internal/app"
	"arenasuite/web"
)

// Page is what every template receives. Shared bits (the signed-in user,
// the unread badge, flash message) live here; Data holds the page's own values.
type Page struct {
	Title  string
	User   *app.User
	Unread int64
	Flash  *Flash
	Path   string
	Data   map[string]any
}

// Flash is a one-time message shown at the top of the next page (a "toast").
type Flash struct {
	Kind string // "success" or "error"
	Msg  string
}

// loadTemplates parses each page together with the shared layout and
// partials. Every page gets its own template set so their "content" blocks
// don't overwrite each other.
func loadTemplates() map[string]*template.Template {
	pages, err := fs.Glob(web.FS, "templates/pages/*.html")
	if err != nil {
		log.Fatal(err)
	}
	out := map[string]*template.Template{}
	for _, file := range pages {
		name := strings.TrimSuffix(path.Base(file), ".html")
		t := template.New(name).Funcs(templateFuncs)
		out[name] = template.Must(t.ParseFS(web.FS, "templates/layout.html", "templates/partials.html", file))
	}
	return out
}

// render fills in the shared Page fields and writes the page.
func (s *Server) render(w http.ResponseWriter, r *http.Request, status int, name, title string, data map[string]any) {
	t, ok := s.templates[name]
	if !ok {
		http.Error(w, "unknown template "+name, http.StatusInternalServerError)
		return
	}
	user := currentUser(r)
	page := Page{Title: title, User: user, Path: r.URL.Path, Data: data, Flash: takeFlash(w, r)}
	if user != nil {
		page.Unread = s.app.UnreadCount(user.ID)
	}

	// Render into a buffer first so a template error doesn't leave a half page.
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "layout", page); err != nil {
		log.Printf("render %s: %v", name, err)
		http.Error(w, "Something went wrong", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	buf.WriteTo(w)
}

// ---------- flash messages (stored in a short-lived cookie) ----------

func setFlash(w http.ResponseWriter, kind, msg string) {
	http.SetCookie(w, &http.Cookie{
		Name: "flash", Value: url.QueryEscape(kind + "|" + msg), Path: "/", MaxAge: 60, HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func takeFlash(w http.ResponseWriter, r *http.Request) *Flash {
	c, err := r.Cookie("flash")
	if err != nil {
		return nil
	}
	http.SetCookie(w, &http.Cookie{Name: "flash", Path: "/", MaxAge: -1})
	value, _ := url.QueryUnescape(c.Value)
	kind, msg, _ := strings.Cut(value, "|")
	return &Flash{Kind: kind, Msg: msg}
}

// ---------- helpers available inside templates ----------

var templateFuncs = template.FuncMap{
	// date formats a stored date as "Oct 20, 2026".
	"date": func(s string) string { return formatTime(s, "Jan 2, 2006") },
	// datetime formats a stored date as "Oct 20, 2026, 10:00 AM".
	"datetime": func(s string) string { return formatTime(s, "Jan 2, 2006, 3:04 PM") },
	// fee shows "Free" or a rupee amount.
	"fee": func(f float64) string {
		if f <= 0 {
			return "Free"
		}
		return fmt.Sprintf("₹%.0f", f)
	},
	// label turns "round_robin" into "Round robin".
	"label": func(s string) string {
		s = strings.ReplaceAll(s, "_", " ")
		if s == "" {
			return s
		}
		return strings.ToUpper(s[:1]) + s[1:]
	},
	"lower": strings.ToLower,
	// initial is the first letter of a name, for avatar circles.
	"initial": func(s string) string {
		for _, r := range s {
			return strings.ToUpper(string(r))
		}
		return "?"
	},
	// plural picks "team" or "teams".
	"plural": func(n int64, one, many string) string {
		if n == 1 {
			return one
		}
		return many
	},
	// dict builds a map so a partial can receive several values:
	// {{template "team-card" dict "Team" . "User" $.User}}
	"dict": func(pairs ...any) map[string]any {
		m := map[string]any{}
		for i := 0; i+1 < len(pairs); i += 2 {
			m[fmt.Sprint(pairs[i])] = pairs[i+1]
		}
		return m
	},
	// rounds groups matches by round, in order, for the bracket view.
	"rounds": func(matches []app.Match) []matchRound {
		var out []matchRound
		for _, m := range matches {
			if len(out) == 0 || out[len(out)-1].Name != m.Round {
				out = append(out, matchRound{Name: m.Round})
			}
			out[len(out)-1].Matches = append(out[len(out)-1].Matches, m)
		}
		return out
	},
	// statusColor picks the pill colour for a status word.
	"statusColor": func(status string) string {
		switch status {
		case "published", "approved", "accepted", "open", "active":
			return "green"
		case "ongoing", "live":
			return "blue"
		case "cancelled", "rejected", "suspended":
			return "red"
		case "pending", "postponed":
			return "amber"
		}
		return ""
	},
	"canManage":  app.CanManage,
	"categories": func() []string { return app.Categories },
	"formats":    func() []string { return app.CompetitionFormats },
	"roles":      func() []string { return app.Roles },
	"join":       strings.Join,
}

type matchRound struct {
	Name    string
	Matches []app.Match
}

// formatTime understands both formats we store: form dates
// ("2026-10-20T10:00", local time) and SQLite timestamps
// ("2026-10-20 10:00:00", UTC).
func formatTime(s, layout string) string {
	if s == "" {
		return ""
	}
	if t, err := time.ParseInLocation(app.DateTimeLayout, s, time.Local); err == nil {
		return t.Format(layout)
	}
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return t.Local().Format(layout)
	}
	if t, err := time.ParseInLocation("2006-01-02", s, time.Local); err == nil {
		return t.Format(layout)
	}
	return s
}
