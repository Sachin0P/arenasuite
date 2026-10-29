package main

import (
	"errors"
	"net/http"

	"arenasuite/internal/models"
	"arenasuite/internal/validator"
)

// Every /admin route is wrapped in the requireAdmin middleware (see routes),
// so these handlers can assume the user is an admin.

func (app *application) adminOverview(w http.ResponseWriter, r *http.Request) {
	stats, err := app.stats.Get()
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	data := app.newTemplateData(r)
	data.PlatformStats = stats
	data.Section = "overview"

	app.render(w, r, http.StatusOK, "admin-overview.tmpl", data)
}

func (app *application) adminUserList(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()

	data := app.newTemplateData(r)
	data.Role = qs.Get("role")
	data.Status = qs.Get("status")
	data.Q = qs.Get("q")
	data.Section = "users"

	users, err := app.users.Search(data.Role, data.Status, data.Q)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	data.Users = users

	app.render(w, r, http.StatusOK, "admin-users.tmpl", data)
}

func (app *application) adminOrganizationList(w http.ResponseWriter, r *http.Request) {
	orgs, err := app.organizations.All()
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	data := app.newTemplateData(r)
	data.Orgs = orgs
	data.Section = "organizations"

	app.render(w, r, http.StatusOK, "admin-organizations.tmpl", data)
}

func (app *application) adminCompetitionList(w http.ResponseWriter, r *http.Request) {
	competitions, err := app.competitions.All()
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	data := app.newTemplateData(r)
	data.Competitions = competitions
	data.Section = "competitions"

	app.render(w, r, http.StatusOK, "admin-competitions.tmpl", data)
}

func (app *application) adminReportList(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")

	reports, err := app.reports.All(status)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	data := app.newTemplateData(r)
	data.Reports = reports
	data.Status = status
	data.Section = "reports"

	app.render(w, r, http.StatusOK, "admin-reports.tmpl", data)
}

func (app *application) adminUserStatusPost(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r, "id")
	if err != nil {
		app.notFound(w, r)
		return
	}

	status := r.PostFormValue("status")
	if !validator.PermittedValue(status, models.UserStatuses...) {
		app.clientError(w, r, http.StatusBadRequest)
		return
	}

	if id == app.contextGetUser(r).ID {
		app.redirectBackWithError(w, r, "You cannot change your own status")
		return
	}

	err = app.users.SetStatus(id, status)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return
	}

	app.redirectBack(w, r, "User status updated")
}

func (app *application) adminUserRolePost(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r, "id")
	if err != nil {
		app.notFound(w, r)
		return
	}

	role := r.PostFormValue("role")
	if !validator.PermittedValue(role, models.Roles...) {
		app.clientError(w, r, http.StatusBadRequest)
		return
	}

	err = app.users.SetRole(id, role)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return
	}

	app.redirectBack(w, r, "User role updated")
}

func (app *application) adminOrganizationVerifyPost(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r, "id")
	if err != nil {
		app.notFound(w, r)
		return
	}

	err = app.organizations.Verify(id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return
	}

	app.redirectBack(w, r, "Organization verified")
}

// adminCompetitionHidePost force-cancels a competition.
func (app *application) adminCompetitionHidePost(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r, "id")
	if err != nil {
		app.notFound(w, r)
		return
	}

	err = app.competitions.SetStatus(id, "cancelled")
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return
	}

	app.redirectBack(w, r, "Competition hidden (cancelled)")
}

func (app *application) adminReportResolvePost(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r, "id")
	if err != nil {
		app.notFound(w, r)
		return
	}

	status := r.PostFormValue("status")
	if !validator.PermittedValue(status, models.ReportResolutions...) {
		app.clientError(w, r, http.StatusBadRequest)
		return
	}

	err = app.reports.Resolve(id, status)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return
	}

	app.redirectBack(w, r, "Report updated")
}
