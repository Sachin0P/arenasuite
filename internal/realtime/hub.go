// Package realtime pushes live updates to browsers using Server-Sent Events
// (SSE). A browser opens one long-lived GET /events request, and the server
// writes events to it as they happen. The browser side is the built-in
// EventSource API, so no WebSocket library is needed.
//
// Clients are grouped into "rooms":
//   - "user:<id>"        every tab of a signed-in user (personal notifications)
//   - "competition:<id>" everyone viewing that competition's page (live scores)
//
// The server never broadcasts to everyone, only to one room at a time.
package realtime

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Event is one message sent to the browser.
type Event struct {
	Name string // e.g. "scoreUpdated"; the browser listens by this name
	Data string // JSON payload
}

type client struct {
	events chan Event
}

type Hub struct {
	mu    sync.Mutex
	rooms map[string]map[*client]bool
}

func NewHub() *Hub {
	return &Hub{rooms: map[string]map[*client]bool{}}
}

func UserRoom(userID int64) string        { return fmt.Sprintf("user:%d", userID) }
func CompetitionRoom(compID int64) string { return fmt.Sprintf("competition:%d", compID) }

// Send delivers an event to everyone in a room. The payload is turned into JSON.
func (h *Hub) Send(room, name string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		slog.Error("realtime: cannot encode event", "event", name, "error", err)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.rooms[room] {
		select {
		case c.events <- Event{Name: name, Data: string(data)}:
		default:
			// This client's buffer is full (very slow connection); skip it
			// rather than blocking every other request.
		}
	}
}

func (h *Hub) join(c *client, rooms []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, room := range rooms {
		if h.rooms[room] == nil {
			h.rooms[room] = map[*client]bool{}
		}
		h.rooms[room][c] = true
	}
}

func (h *Hub) leave(c *client, rooms []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, room := range rooms {
		delete(h.rooms[room], c)
		if len(h.rooms[room]) == 0 {
			delete(h.rooms, room)
		}
	}
}

// Serve keeps the HTTP response open and streams events from the given rooms
// until the browser disconnects.
func (h *Hub) Serve(w http.ResponseWriter, r *http.Request, rooms []string) {
	// Use a ResponseController rather than asserting w.(http.Flusher): it
	// finds the Flush method even when middleware (like the session manager)
	// has wrapped the ResponseWriter.
	rc := http.NewResponseController(w)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	c := &client{events: make(chan Event, 32)}
	h.join(c, rooms)
	defer h.leave(c, rooms)

	// A comment line every 25s keeps proxies from closing an idle connection.
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()

	fmt.Fprint(w, ": connected\n\n")
	rc.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			rc.Flush()
		case ev := <-c.events:
			// SSE wire format: "event: <name>\ndata: <json>\n\n"
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Name, ev.Data)
			rc.Flush()
		}
	}
}
