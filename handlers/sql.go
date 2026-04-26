package handlers

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func sqlOperation(project, targetLink, opType string) map[string]any {
	return map[string]any{
		"kind":          "sql#operation",
		"status":        "DONE",
		"operationType": opType,
		"targetProject": project,
		"targetLink":    targetLink,
		"name":          uuid.NewString(),
	}
}

func (app *Application) CreateSQLInstance(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")

	body, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}

	name, _ := body["name"].(string)
	if name == "" {
		writeGCPError(w, http.StatusBadRequest, "Missing required field: name", "required")
		return
	}

	region, _ := body["region"].(string)
	if region == "" {
		region = "us-central1"
	}

	body["kind"] = "sql#instance"
	body["state"] = "RUNNABLE"
	body["connectionName"] = project + ":" + region + ":" + name
	body["ipAddresses"] = []map[string]any{{
		"type":      "PRIMARY",
		"ipAddress": fmt.Sprintf("10.%d.%d.%d", randomIPv4Octet(), randomIPv4Octet(), randomIPv4Octet()),
	}}
	body["selfLink"] = selfLink(r, "sql", "v1beta4", "projects", project, "instances", name)

	created, err := app.repo.CreateSQLInstance(project, body)
	if err != nil {
		writeCreateError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, sqlOperation(project, getString(created, "selfLink"), "CREATE"))
}

func (app *Application) GetSQLInstance(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	name := chi.URLParam(r, "name")

	item, err := app.repo.GetSQLInstance(project, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (app *Application) ListSQLInstances(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")

	items, err := app.repo.ListSQLInstances(project)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if items == nil {
		items = []map[string]any{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"kind":  "sql#instancesList",
		"items": items,
	})
}

func (app *Application) UpdateSQLInstance(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	name := chi.URLParam(r, "name")

	patch, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}

	updated, err := app.repo.UpdateSQLInstance(project, name, patch)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, sqlOperation(project, getString(updated, "selfLink"), "UPDATE"))
}

func (app *Application) DeleteSQLInstance(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	name := chi.URLParam(r, "name")

	item, err := app.repo.GetSQLInstance(project, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if err := app.repo.DeleteSQLInstance(project, name); err != nil {
		writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, sqlOperation(project, getString(item, "selfLink"), "DELETE"))
}

func (app *Application) CreateSQLDatabase(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	instance := chi.URLParam(r, "instance")

	body, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}

	name, _ := body["name"].(string)
	if name == "" {
		writeGCPError(w, http.StatusBadRequest, "Missing required field: name", "required")
		return
	}

	body["kind"] = "sql#database"
	body["instance"] = instance
	body["selfLink"] = selfLink(r, "sql", "v1beta4", "projects", project, "instances", instance, "databases", name)

	created, err := app.repo.CreateSQLDatabase(project, instance, body)
	if err != nil {
		writeCreateError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, sqlOperation(project, getString(created, "selfLink"), "CREATE_DATABASE"))
}

func (app *Application) GetSQLDatabase(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	instance := chi.URLParam(r, "instance")
	name := chi.URLParam(r, "name")

	item, err := app.repo.GetSQLDatabase(project, instance, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (app *Application) ListSQLDatabases(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	instance := chi.URLParam(r, "instance")

	items, err := app.repo.ListSQLDatabases(project, instance)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if items == nil {
		items = []map[string]any{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"kind":  "sql#databasesList",
		"items": items,
	})
}

func (app *Application) DeleteSQLDatabase(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	instance := chi.URLParam(r, "instance")
	name := chi.URLParam(r, "name")

	item, err := app.repo.GetSQLDatabase(project, instance, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if err := app.repo.DeleteSQLDatabase(project, instance, name); err != nil {
		writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, sqlOperation(project, getString(item, "selfLink"), "DELETE_DATABASE"))
}

func (app *Application) CreateSQLUser(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	instance := chi.URLParam(r, "instance")

	body, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}

	name, _ := body["name"].(string)
	if name == "" {
		writeGCPError(w, http.StatusBadRequest, "Missing required field: name", "required")
		return
	}

	body["kind"] = "sql#user"
	body["instance"] = instance

	created, err := app.repo.CreateSQLUser(project, instance, body)
	if err != nil {
		writeCreateError(w, err)
		return
	}

	targetLink := selfLink(r, "sql", "v1beta4", "projects", project, "instances", instance, "users", getString(created, "name"))
	writeJSON(w, http.StatusOK, sqlOperation(project, targetLink, "CREATE_USER"))
}

func (app *Application) ListSQLUsers(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	instance := chi.URLParam(r, "instance")

	items, err := app.repo.ListSQLUsers(project, instance)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if items == nil {
		items = []map[string]any{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"kind":  "sql#usersList",
		"items": items,
	})
}

func (app *Application) UpdateSQLUser(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	instance := chi.URLParam(r, "instance")

	patch, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		name, _ = patch["name"].(string)
	}
	if name == "" {
		writeGCPError(w, http.StatusBadRequest, "Missing required field: name", "required")
		return
	}

	updated, err := app.repo.UpdateSQLUser(project, instance, name, patch)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	targetLink := selfLink(r, "sql", "v1beta4", "projects", project, "instances", instance, "users", getString(updated, "name"))
	writeJSON(w, http.StatusOK, sqlOperation(project, targetLink, "UPDATE_USER"))
}

func (app *Application) DeleteSQLUser(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	instance := chi.URLParam(r, "instance")
	name := r.URL.Query().Get("name")
	if name == "" {
		writeGCPError(w, http.StatusBadRequest, "Missing required query parameter: name", "required")
		return
	}

	targetLink := selfLink(r, "sql", "v1beta4", "projects", project, "instances", instance, "users", name)
	if err := app.repo.DeleteSQLUser(project, instance, name); err != nil {
		writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, sqlOperation(project, targetLink, "DELETE_USER"))
}
