package main

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"arenasuite/internal/models"
	"arenasuite/internal/realtime"

	"github.com/go-playground/form/v4"
)

// The serverError helper writes a log entry at Error level (including the
// request method and URI as attributes), then sends a generic 500 Internal
// Server Error response to the user.
func (app *application) serverError(w http.ResponseWriter, r *http.Request, err error) {
	var (
		method = r.Method
		uri    = r.URL.RequestURI()
	)

	app.logger.Error(err.Error(), "method", method, "uri", uri)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

// The clientError helper renders the error page with a message for the given
// status code. We'll use this to send responses like 403 "Forbidden" when
// there's a problem with the request that the user sent.
func (app *application) clientError(w http.ResponseWriter, r *http.Request, status int) {
	data := app.newTemplateData(r)
	data.Heading = http.StatusText(status)

	switch status {
	case http.StatusNotFound:
		data.Heading = "Page not found"
		data.Message = "The page you're looking for doesn't exist or isn't public."
	case http.StatusForbidden:
		data.Heading = "Not allowed"
		data.Message = "You don't have permission to do that."
	}

	app.render(w, r, status, "error.tmpl", data)
}

// For consistency, we'll also implement a notFound helper. This is simply a
// convenience wrapper around clientError which sends a 404 Not Found response.
func (app *application) notFound(w http.ResponseWriter, r *http.Request) {
	app.clientError(w, r, http.StatusNotFound)
}

func (app *application) render(w http.ResponseWriter, r *http.Request, status int, page string, data templateData) {
	// Retrieve the appropriate template set from the cache based on the page
	// name (like 'home.tmpl'). If no entry exists in the cache with the
	// provided name, then create a new error and call the serverError() helper
	// method that we made earlier and return.
	ts, ok := app.templateCache[page]
	if !ok {
		err := fmt.Errorf("the template %s does not exist", page)
		app.serverError(w, r, err)
		return
	}

	// Initialize a new buffer.
	buf := new(bytes.Buffer)

	// Write the template to the buffer, instead of straight to the
	// http.ResponseWriter. If there's an error, call our serverError() helper
	// and then return.
	err := ts.ExecuteTemplate(buf, "base", data)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// If the template is written to the buffer without any errors, we are
	// safe to go ahead and write the HTTP status code to http.ResponseWriter.
	w.WriteHeader(status)

	// Write the contents of the buffer to the http.ResponseWriter.
	buf.WriteTo(w)
}

// Create a newTemplateData() helper, which returns a templateData struct
// initialized with the data that every page needs.
func (app *application) newTemplateData(r *http.Request) templateData {
	user := app.contextGetUser(r)

	data := templateData{
		CurrentYear: time.Now().Year(),
		// Use the PopString() method to retrieve the value for the "flash"
		// key. PopString() also deletes the key and value from the session
		// data, so it acts like a one-time fetch.
		Flash:           app.sessionManager.PopString(r.Context(), "flash"),
		FlashError:      app.sessionManager.PopString(r.Context(), "flashError"),
		IsAuthenticated: user != nil,
		User:            user,
		Path:            r.URL.Path,
	}

	if user != nil {
		unread, err := app.notifications.UnreadCount(user.ID)
		if err != nil {
			// The badge isn't worth failing the whole page over, so just log it.
			app.logger.Error(err.Error(), "uri", r.URL.RequestURI())
		}
		data.Unread = unread
	}

	return data
}

// Create a new decodePostForm() helper method. The second parameter here,
// dst, is the target destination that we want to decode the form data into.
func (app *application) decodePostForm(r *http.Request, dst any) error {
	// Call ParseForm() on the request, which fills r.PostForm with the
	// values from the request body.
	err := r.ParseForm()
	if err != nil {
		return err
	}

	// Call Decode() on our decoder instance, passing the target destination as
	// the first parameter.
	err = app.formDecoder.Decode(dst, r.PostForm)
	if err != nil {
		// If we try to use an invalid target destination, the Decode() method
		// will return an error with the type *form.InvalidDecoderError. We use
		// errors.As() to check for this and raise a panic rather than returning
		// the error.
		var invalidDecoderError *form.InvalidDecoderError

		if errors.As(err, &invalidDecoderError) {
			panic(err)
		}

		// For all other errors, we return them as normal.
		return err
	}

	return nil
}

// readIDParam reads a numeric wildcard like {id} from the URL path. Any value
// that isn't a positive integer is an error.
func (app *application) readIDParam(r *http.Request, name string) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id < 1 {
		return 0, errors.New("invalid id parameter")
	}

	return id, nil
}

// backURL is where a form action redirects to: the form's hidden "next" field
// if it's a local path, otherwise the page the form was on.
func backURL(r *http.Request) string {
	next := r.PostFormValue("next")
	if strings.HasPrefix(next, "/") && !strings.HasPrefix(next, "//") {
		return next
	}

	if ref, err := url.Parse(r.Referer()); err == nil && ref.Path != "" {
		return ref.RequestURI()
	}

	return "/"
}

// redirectBack finishes a successful form action: it stores a flash message
// (if there is one) and sends the user back to the page the form was on
// (Post/Redirect/Get, so refreshing never re-submits the form).
func (app *application) redirectBack(w http.ResponseWriter, r *http.Request, flash string) {
	if flash != "" {
		app.sessionManager.Put(r.Context(), "flash", flash)
	}
	http.Redirect(w, r, backURL(r), http.StatusSeeOther)
}

// redirectBackWithError is redirectBack for an action that couldn't be done,
// like joining a team that's already full.
func (app *application) redirectBackWithError(w http.ResponseWriter, r *http.Request, message string) {
	app.sessionManager.Put(r.Context(), "flashError", message)
	http.Redirect(w, r, backURL(r), http.StatusSeeOther)
}

// getCompetitionToManage loads the competition with the given id and checks
// that the signed-in user may manage it. If not, it sends the error response
// itself and returns ok == false, so the caller just needs to return.
func (app *application) getCompetitionToManage(w http.ResponseWriter, r *http.Request, id int64) (*models.Competition, bool) {
	competition, err := app.competitions.Get(id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return nil, false
	}

	if !competition.ManagedBy(app.contextGetUser(r)) {
		app.clientError(w, r, http.StatusForbidden)
		return nil, false
	}

	return competition, true
}

// getTeam loads the team with the {id} in the URL, sending a 404 or 500
// response itself if that fails.
func (app *application) getTeam(w http.ResponseWriter, r *http.Request) (*models.Team, bool) {
	id, err := app.readIDParam(r, "id")
	if err != nil {
		app.notFound(w, r)
		return nil, false
	}

	team, err := app.teams.Get(id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return nil, false
	}

	return team, true
}

// notify saves a notification for a user and pushes it live to any open tab,
// so it shows up now and is still there if they weren't online.
func (app *application) notify(userID int64, kind, message string, referenceID int64) {
	n, err := app.notifications.Insert(userID, kind, message, referenceID)
	if err != nil {
		app.logger.Error(err.Error(), "notification", kind)
		return
	}

	app.hub.Send(realtime.UserRoom(userID), "notification", n)
}

// broadcast pushes a live event to everyone viewing a competition's page.
func (app *application) broadcast(competitionID int64, event string, payload any) {
	app.hub.Send(realtime.CompetitionRoom(competitionID), event, payload)
}

// parseDateTime turns a datetime-local form value like "2026-10-20T10:00"
// into a time in the site's timezone.
func parseDateTime(value string) (time.Time, error) {
	return time.ParseInLocation(dateTimeLayout, strings.TrimSpace(value), time.Local)
}
